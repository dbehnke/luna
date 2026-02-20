package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"luna/internal/auth"
	"luna/internal/id"
	"luna/internal/jobs"
	"luna/internal/logging"
	"luna/internal/meta"
	"luna/internal/models"
	"luna/internal/storage"

	"gorm.io/gorm"
)

type Handler struct {
	db        *gorm.DB
	mediaRoot string
	jobQueue  *jobs.Queue
}

func New(db *gorm.DB, mediaRoot string) *Handler {
	return &Handler{db: db, mediaRoot: mediaRoot, jobQueue: jobs.New(db)}
}

type CreateItemRequest struct {
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	PersonaID   *string `json:"persona_id"`
}

type CreateItemResponse struct {
	ItemID    string `json:"item_id"`
	UploadURL string `json:"upload_url"`
}

type MediaItemResponse struct {
	ID               string   `json:"id"`
	Type             string   `json:"type"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	CreatedAt        string   `json:"created_at"`
	MediaURL         string   `json:"media_url,omitempty"`
	ProcessingStatus string   `json:"processing_status,omitempty"`
	MasterURL        string   `json:"master_url,omitempty"`
	ThumbURLs        []string `json:"thumb_urls,omitempty"`
	ErrorMessage     string   `json:"error_message,omitempty"`
	DeletedAt        *string  `json:"deleted_at,omitempty"`
}

type ListItemsResponse struct {
	Items   []MediaItemResponse `json:"items"`
	Cursor  string              `json:"cursor,omitempty"`
	HasMore bool                `json:"has_more"`
}

func (h *Handler) CreateItem(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Type != "video" && req.Type != "photo" {
		http.Error(w, "Invalid type: must be 'video' or 'photo'", http.StatusBadRequest)
		return
	}

	itemID := id.NewULID()
	now := time.Now().UTC()

	var personaDisplayName *string
	var personaID *uint

	if req.PersonaID != nil && *req.PersonaID != "" {
		pid, err := strconv.ParseUint(*req.PersonaID, 10, 32)
		if err == nil {
			var persona models.Persona
			if err := h.db.First(&persona, pid).Error; err == nil && persona.UserID == user.ID {
				personaID = &persona.ID
				personaDisplayName = &persona.DisplayName
			}
		}
	}

	mediaItem := models.MediaItem{
		ID:          itemID,
		UserID:      user.ID,
		PersonaID:   personaID,
		Type:        req.Type,
		Title:       req.Title,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.db.Create(&mediaItem).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to create item: %v", err), http.StatusInternalServerError)
		return
	}

	if err := storage.EnsureItemDirs(h.mediaRoot, itemID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create item dirs: %v", err), http.StatusInternalServerError)
		return
	}

	itemMeta := &meta.ItemMeta{
		Schema:             meta.SchemaVersion,
		ItemID:             itemID,
		Type:               req.Type,
		OwnerUsername:      user.Username,
		PersonaDisplayName: personaDisplayName,
		Title:              req.Title,
		Description:        req.Description,
		CreatedAt:          now.Format(time.RFC3339),
		State: meta.ItemState{
			Highlighted: false,
		},
		Original: meta.OriginalFile{
			Filename: "",
			Path:     "",
		},
	}

	if err := meta.WriteItemMetaAtomic(h.mediaRoot, itemID, itemMeta); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write meta: %v", err), http.StatusInternalServerError)
		return
	}

	assetsMeta := &meta.AssetsMeta{
		Schema:     meta.SchemaVersion,
		Assets:     []meta.Asset{},
		Thumbnails: []meta.Thumbnail{},
		Photos:     []meta.Photo{},
	}

	if err := meta.WriteAssetsMetaAtomic(h.mediaRoot, itemID, assetsMeta); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write assets meta: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CreateItemResponse{
		ItemID:    itemID,
		UploadURL: fmt.Sprintf("/api/items/%s/upload", itemID),
	})
}

func (h *Handler) UploadItem(w http.ResponseWriter, r *http.Request, itemID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if itemID == "" {
		http.Error(w, "Item ID required", http.StatusBadRequest)
		return
	}

	var mediaItem models.MediaItem
	if err := h.db.First(&mediaItem, "id = ?", itemID).Error; err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	if mediaItem.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	originalFilename := header.Filename
	ext := strings.ToLower(filepath.Ext(originalFilename))
	if ext == "" {
		ext = ".bin"
	}

	tmpDir := storage.TmpUploadsDir(h.mediaRoot)
	if err := storage.EnsureRootLayout(h.mediaRoot); err != nil {
		http.Error(w, fmt.Sprintf("Failed to ensure dirs: %v", err), http.StatusInternalServerError)
		return
	}

	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("%s-%d%s", itemID, time.Now().UnixNano(), ext))
	destPath := filepath.Join(h.mediaRoot, "items", itemID, "original", "upload"+ext)

	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create temp file: %v", err), http.StatusInternalServerError)
		return
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, file); err != nil {
		os.Remove(tmpPath)
		http.Error(w, fmt.Sprintf("Failed to write file: %v", err), http.StatusInternalServerError)
		return
	}
	tmpFile.Close()

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		http.Error(w, fmt.Sprintf("Failed to save file: %v", err), http.StatusInternalServerError)
		return
	}

	itemMeta, err := meta.ReadItemMetaByID(h.mediaRoot, itemID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read meta: %v", err), http.StatusInternalServerError)
		return
	}

	itemMeta.Original.Filename = originalFilename
	itemMeta.Original.Path = "original/upload" + ext

	if err := meta.WriteItemMetaAtomic(h.mediaRoot, itemID, itemMeta); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update meta: %v", err), http.StatusInternalServerError)
		return
	}

	if mediaItem.Title == "" {
		baseName := filepath.Base(originalFilename)
		if ext := filepath.Ext(baseName); ext != "" {
			baseName = strings.TrimSuffix(baseName, ext)
		}
		mediaItem.Title = baseName
		h.db.Model(&mediaItem).Update("title", baseName)
	}

	if mediaItem.Type == models.MediaTypeVideo {
		if err := h.jobQueue.EnqueueVideoProcessing(itemID); err != nil {
			logging.Info.Printf("Failed to enqueue processing jobs for %s: %v", itemID, err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"item_id":   itemID,
		"media_url": fmt.Sprintf("/media/%s/original/upload%s", itemID, ext),
	})
}

func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	query := h.db.Where("deleted_at IS NULL")

	itemType := r.URL.Query().Get("type")
	if itemType != "" {
		query = query.Where("type = ?", itemType)
	}

	userParam := r.URL.Query().Get("user")
	if userParam == "me" {
		user := auth.GetUser(r.Context())
		if user != nil {
			query = query.Where("user_id = ?", user.ID)
		}
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var items []models.MediaItem
	query = query.Order("created_at DESC").Limit(limit + 1)
	if err := query.Find(&items).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to list items: %v", err), http.StatusInternalServerError)
		return
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	responses := make([]MediaItemResponse, len(items))
	for i, item := range items {
		resp := MediaItemResponse{
			ID:          item.ID,
			Type:        item.Type,
			Title:       item.Title,
			Description: item.Description,
			CreatedAt:   item.CreatedAt.Format(time.RFC3339),
		}

		itemMeta, err := meta.ReadItemMetaByID(h.mediaRoot, item.ID)
		if err == nil && itemMeta.Original.Path != "" {
			resp.MediaURL = "/media/" + item.ID + "/" + itemMeta.Original.Path
		}

		if item.Type == models.MediaTypeVideo {
			status, _ := h.jobQueue.GetProcessingStatus(item.ID)
			resp.ProcessingStatus = status

			assetsMeta, _ := meta.ReadAssetsMetaByID(h.mediaRoot, item.ID)
			if assetsMeta != nil {
				for _, asset := range assetsMeta.Assets {
					if asset.Kind == "master_mp4" {
						resp.MasterURL = "/media/" + item.ID + "/" + asset.StoragePath
						break
					}
				}
				if len(assetsMeta.Thumbnails) > 0 {
					resp.ThumbURLs = []string{"/media/" + item.ID + "/" + assetsMeta.Thumbnails[0].StoragePath}
				}
			}
		}

		responses[i] = resp
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ListItemsResponse{
		Items:   responses,
		HasMore: hasMore,
	})
}

func (h *Handler) GetItem(w http.ResponseWriter, r *http.Request, itemID string) {
	if itemID == "" {
		http.Error(w, "Item ID required", http.StatusBadRequest)
		return
	}

	var item models.MediaItem
	if err := h.db.First(&item, "id = ?", itemID).Error; err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	itemMeta, _ := meta.ReadItemMetaByID(h.mediaRoot, itemID)
	assetsMeta, _ := meta.ReadAssetsMetaByID(h.mediaRoot, itemID)

	resp := MediaItemResponse{
		ID:          item.ID,
		Type:        item.Type,
		Title:       item.Title,
		Description: item.Description,
		CreatedAt:   item.CreatedAt.Format(time.RFC3339),
	}

	if itemMeta != nil && itemMeta.Original.Path != "" {
		resp.MediaURL = "/media/" + item.ID + "/" + itemMeta.Original.Path
	}

	if item.Type == models.MediaTypeVideo {
		status, _ := h.jobQueue.GetProcessingStatus(itemID)
		resp.ProcessingStatus = status

		if status == "failed" {
			if errMsg, _ := h.jobQueue.GetJobError(itemID); errMsg != "" {
				resp.ErrorMessage = errMsg
			}
		}

		if assetsMeta != nil {
			for _, asset := range assetsMeta.Assets {
				if asset.Kind == "master_mp4" {
					resp.MasterURL = "/media/" + item.ID + "/" + asset.StoragePath
					break
				}
			}

			for _, thumb := range assetsMeta.Thumbnails {
				resp.ThumbURLs = append(resp.ThumbURLs, "/media/"+item.ID+"/"+thumb.StoragePath)
			}
		}
	}

	if item.DeletedAt.Valid {
		deletedAt := item.DeletedAt.Time.Format(time.RFC3339)
		resp.DeletedAt = &deletedAt
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request, itemID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if itemID == "" {
		http.Error(w, "Item ID required", http.StatusBadRequest)
		return
	}

	var item models.MediaItem
	if err := h.db.First(&item, "id = ?", itemID).Error; err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	if item.UserID != user.ID {
		http.Error(w, "Forbidden - only owner can delete", http.StatusForbidden)
		return
	}

	now := time.Now()
	if err := h.db.Model(&item).Update("deleted_at", now).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete: %v", err), http.StatusInternalServerError)
		return
	}

	itemMeta, err := meta.ReadItemMetaByID(h.mediaRoot, itemID)
	if err == nil {
		deletedAt := now.Format(time.RFC3339)
		itemMeta.State.DeletedAt = &deletedAt
		meta.WriteItemMetaAtomic(h.mediaRoot, itemID, itemMeta)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "deleted",
	})
}

type CreateClipRequest struct {
	StartMs int64 `json:"start_ms"`
	EndMs   int64 `json:"end_ms"`
}

type CreateClipResponse struct {
	ClipID  string `json:"clip_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func (h *Handler) CreateClip(w http.ResponseWriter, r *http.Request, itemID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if itemID == "" {
		http.Error(w, "Item ID required", http.StatusBadRequest)
		return
	}

	var item models.MediaItem
	if err := h.db.First(&item, "id = ?", itemID).Error; err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	if item.Type != models.MediaTypeVideo {
		http.Error(w, "Clips can only be created from video items", http.StatusBadRequest)
		return
	}

	status, _ := h.jobQueue.GetProcessingStatus(itemID)
	if status != "ready" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(CreateClipResponse{
			ClipID:  "",
			Status:  "not_ready",
			Message: fmt.Sprintf("Video processing status: %s", status),
		})
		return
	}

	var req CreateClipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.StartMs < 0 || req.EndMs <= req.StartMs {
		http.Error(w, "Invalid start/end times", http.StatusBadRequest)
		return
	}

	durationMs := req.EndMs - req.StartMs
	if durationMs > 90000 {
		http.Error(w, "Clip duration cannot exceed 90 seconds", http.StatusBadRequest)
		return
	}

	clipID := id.NewULID()

	clipAsset := models.ClipAsset{
		ID:          clipID,
		ItemID:      itemID,
		StoragePath: fmt.Sprintf("derived/short_%s.mp4", clipID),
		Status:      models.ClipStatusPending,
		StartMs:     req.StartMs,
		EndMs:       req.EndMs,
		DurationMs:  durationMs,
		Width:       1080,
		Height:      1920,
		CropMode:    "9:16_center",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := h.db.Create(&clipAsset).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to create clip: %v", err), http.StatusInternalServerError)
		return
	}

	assetsMeta, _ := meta.ReadAssetsMetaByID(h.mediaRoot, itemID)
	if assetsMeta == nil {
		assetsMeta = &meta.AssetsMeta{Schema: meta.SchemaVersion}
	}

	assetsMeta.Assets = append(assetsMeta.Assets, meta.Asset{
		Kind:        "short_clip",
		StoragePath: fmt.Sprintf("derived/short_%s.mp4", clipID),
		ClipID:      clipID,
		StartMs:     req.StartMs,
		EndMs:       req.EndMs,
		CropMode:    "9:16_center",
		Width:       1080,
		Height:      1920,
	})

	if err := meta.WriteAssetsMetaAtomic(h.mediaRoot, itemID, assetsMeta); err != nil {
		logging.Error.Printf("Failed to write assets meta for clip: %v", err)
	}

	_, enqueueErr := h.jobQueue.Enqueue(models.JobTypeClip, models.JobStatusQueued, 75, jobs.ClipPayload{
		ItemID:  itemID,
		ClipID:  clipID,
		StartMs: req.StartMs,
		EndMs:   req.EndMs,
	})
	if enqueueErr != nil {
		logging.Error.Printf("Failed to enqueue clip job: %v", enqueueErr)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CreateClipResponse{
		ClipID: clipID,
		Status: "queued",
	})
}

type ShortsClipResponse struct {
	ClipID       string `json:"clip_id"`
	ItemID       string `json:"item_id"`
	VideoURL     string `json:"video_url,omitempty"`
	ThumbURL     string `json:"thumb_url,omitempty"`
	Title        string `json:"title"`
	OwnerName    string `json:"owner_name"`
	UpCount      int    `json:"up_count"`
	DownCount    int    `json:"down_count"`
	UserReaction *int   `json:"user_reaction,omitempty"`
	CreatedAt    string `json:"created_at"`
	Status       string `json:"status"`
}

type ListShortsResponse struct {
	Clips   []ShortsClipResponse `json:"clips"`
	Cursor  string               `json:"cursor,omitempty"`
	HasMore bool                 `json:"has_more"`
}

func (h *Handler) ListShorts(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 25 {
			limit = parsed
		}
	}

	query := h.db.Model(&models.ClipAsset{}).
		Select("clip_assets.*, media_items.title, users.username as owner_name").
		Joins("JOIN media_items ON media_items.id = clip_assets.item_id").
		Joins("JOIN users ON users.id = media_items.user_id").
		Where("media_items.deleted_at IS NULL").
		Order("clip_assets.created_at DESC, clip_assets.id DESC")

	cursor := r.URL.Query().Get("cursor")
	if cursor != "" {
		decoded := ""
		if decodedBytes, err := base64.StdEncoding.DecodeString(cursor); err == nil {
			decoded = string(decodedBytes)
		}
		parts := strings.Split(decoded, "|")
		if len(parts) == 2 {
			ts, _ := strconv.ParseInt(parts[0], 10, 64)
			clipID := parts[1]
			query = query.Where("(clip_assets.created_at < ? OR (clip_assets.created_at = ? AND clip_assets.id < ?))",
				time.Unix(ts, 0), time.Unix(ts, 0), clipID)
		}
	}

	var clips []struct {
		models.ClipAsset
		Title     string
		OwnerName string
	}

	query = query.Limit(limit + 1)
	if err := query.Find(&clips).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to list shorts: %v", err), http.StatusInternalServerError)
		return
	}

	hasMore := len(clips) > limit
	if hasMore {
		clips = clips[:limit]
	}

	user := auth.GetUser(r.Context())

	var responses []ShortsClipResponse
	for _, clip := range clips {
		thumbURL := ""
		assetsMeta, _ := meta.ReadAssetsMetaByID(h.mediaRoot, clip.ItemID)
		if assetsMeta != nil && len(assetsMeta.Thumbnails) > 0 {
			thumbURL = "/media/" + clip.ItemID + "/" + assetsMeta.Thumbnails[0].StoragePath
		}

		var upCount, downCount int64
		h.db.Model(&models.Reaction{}).Where("item_id = ? AND value = 1", clip.ItemID).Count(&upCount)
		h.db.Model(&models.Reaction{}).Where("item_id = ? AND value = -1", clip.ItemID).Count(&downCount)

		var userReaction *int
		if user != nil {
			var reaction models.Reaction
			if err := h.db.Where("user_id = ? AND item_id = ?", user.ID, clip.ItemID).First(&reaction).Error; err == nil {
				userReaction = &reaction.Value
			}
		}

		status := clip.Status
		videoURL := ""
		if status == models.ClipStatusReady {
			videoURL = "/media/" + clip.ItemID + "/derived/short_" + clip.ID + ".mp4"
		}

		responses = append(responses, ShortsClipResponse{
			ClipID:       clip.ID,
			ItemID:       clip.ItemID,
			VideoURL:     videoURL,
			ThumbURL:     thumbURL,
			Title:        clip.Title,
			OwnerName:    clip.OwnerName,
			UpCount:      int(upCount),
			DownCount:    int(downCount),
			UserReaction: userReaction,
			CreatedAt:    clip.CreatedAt.Format(time.RFC3339),
			Status:       status,
		})
	}

	var nextCursor string
	if hasMore && len(clips) > 0 {
		lastClip := clips[len(clips)-1]
		nextCursor = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d|%s", lastClip.CreatedAt.Unix(), lastClip.ID)))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ListShortsResponse{
		Clips:   responses,
		Cursor:  nextCursor,
		HasMore: hasMore,
	})
}

type SetReactionRequest struct {
	Value int `json:"value"`
}

func (h *Handler) SetReaction(w http.ResponseWriter, r *http.Request, itemID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if itemID == "" {
		http.Error(w, "Item ID required", http.StatusBadRequest)
		return
	}

	var item models.MediaItem
	if err := h.db.First(&item, "id = ?", itemID).Error; err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	var req SetReactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Value != -1 && req.Value != 0 && req.Value != 1 {
		http.Error(w, "Invalid reaction value: must be -1, 0, or 1", http.StatusBadRequest)
		return
	}

	if req.Value == 0 {
		h.db.Where("user_id = ? AND item_id = ?", user.ID, itemID).Delete(&models.Reaction{})
	} else {
		reaction := models.Reaction{
			UserID: user.ID,
			ItemID: itemID,
			Value:  req.Value,
		}

		var existing models.Reaction
		if err := h.db.Where("user_id = ? AND item_id = ?", user.ID, itemID).First(&existing).Error; err == nil {
			existing.Value = req.Value
			h.db.Save(&existing)
		} else {
			h.db.Create(&reaction)
		}
	}

	var upCount, downCount int64
	h.db.Model(&models.Reaction{}).Where("item_id = ? AND value = 1", itemID).Count(&upCount)
	h.db.Model(&models.Reaction{}).Where("item_id = ? AND value = -1", itemID).Count(&downCount)

	var userReaction *int
	var reaction models.Reaction
	if err := h.db.Where("user_id = ? AND item_id = ?", user.ID, itemID).First(&reaction).Error; err == nil {
		userReaction = &reaction.Value
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"up_count":      upCount,
		"down_count":    downCount,
		"user_reaction": userReaction,
	})
}

func (h *Handler) GetItemClips(w http.ResponseWriter, r *http.Request, itemID string) {
	if itemID == "" {
		http.Error(w, "Item ID required", http.StatusBadRequest)
		return
	}

	var clips []models.ClipAsset
	if err := h.db.Where("item_id = ?", itemID).Order("created_at DESC").Find(&clips).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to get clips: %v", err), http.StatusInternalServerError)
		return
	}

	type ClipInfo struct {
		ClipID     string `json:"clip_id"`
		Status     string `json:"status"`
		StartMs    int64  `json:"start_ms"`
		EndMs      int64  `json:"end_ms"`
		DurationMs int64  `json:"duration_ms"`
		VideoURL   string `json:"video_url,omitempty"`
		CreatedAt  string `json:"created_at"`
	}

	responses := make([]ClipInfo, len(clips))
	for i, clip := range clips {
		videoURL := ""
		if clip.Status == models.ClipStatusReady {
			videoURL = "/media/" + itemID + "/derived/short_" + clip.ID + ".mp4"
		}
		responses[i] = ClipInfo{
			ClipID:     clip.ID,
			Status:     clip.Status,
			StartMs:    clip.StartMs,
			EndMs:      clip.EndMs,
			DurationMs: clip.DurationMs,
			VideoURL:   videoURL,
			CreatedAt:  clip.CreatedAt.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"clips": responses,
	})
}

var _ = io.Discard
