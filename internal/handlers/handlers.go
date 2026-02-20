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
	"luna/internal/useradmin"
	"luna/internal/video"

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

type UpdateItemRequest struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
}

type MediaItemResponse struct {
	ID               string   `json:"id"`
	UserID           uint     `json:"user_id"`
	Type             string   `json:"type"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	CreatedAt        string   `json:"created_at"`
	MediaURL         string   `json:"media_url,omitempty"`
	ProcessingStatus string   `json:"processing_status,omitempty"`
	MasterURL        string   `json:"master_url,omitempty"`
	HLSURL           string   `json:"hls_url,omitempty"`
	ThumbURLs        []string `json:"thumb_urls,omitempty"`
	ErrorMessage     string   `json:"error_message,omitempty"`
	DeletedAt        *string  `json:"deleted_at,omitempty"`
	PersonaID        *string  `json:"persona_id,omitempty"`
	PersonaName      *string  `json:"persona_name,omitempty"`
	PersonaSlug      *string  `json:"persona_slug,omitempty"`
	PersonaAvatarURL *string  `json:"persona_avatar_url,omitempty"`
	IsFavorited      bool     `json:"is_favorited"`
	IsHighlighted    bool     `json:"is_highlighted"`
	CanManage        bool     `json:"can_manage,omitempty"`
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

	if req.Type != models.MediaTypeVideo && req.Type != models.MediaTypePhoto && req.Type != models.MediaTypeAudio {
		http.Error(w, "Invalid type: must be 'video', 'photo', or 'audio'", http.StatusBadRequest)
		return
	}

	itemID := id.NewULID()
	now := time.Now().UTC()

	var personaDisplayName *string
	var personaID *string

	if req.PersonaID != nil && *req.PersonaID != "" {
		var persona models.Persona
		if err := h.db.First(&persona, "id = ?", *req.PersonaID).Error; err != nil {
			http.Error(w, "Invalid persona_id", http.StatusBadRequest)
			return
		}
		if persona.UserID != user.ID {
			http.Error(w, "Forbidden - persona is not owned by current user", http.StatusForbidden)
			return
		}
		personaID = &persona.ID
		personaDisplayName = &persona.DisplayName
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
		PersonaID:          personaID,
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
	if err := json.NewEncoder(w).Encode(CreateItemResponse{
		ItemID:    itemID,
		UploadURL: fmt.Sprintf("/api/items/%s/upload", itemID),
	}); err != nil {
		logging.Error.Printf("Failed to encode create item response: %v", err)
	}
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
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logging.Error.Printf("Failed to close upload file for item %s: %v", itemID, closeErr)
		}
	}()

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
	defer func() {
		if closeErr := tmpFile.Close(); closeErr != nil {
			logging.Error.Printf("Failed to close temp upload file for item %s: %v", itemID, closeErr)
		}
	}()

	if _, err := io.Copy(tmpFile, file); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			logging.Error.Printf("Failed to remove temp file %s: %v", tmpPath, removeErr)
		}
		http.Error(w, fmt.Sprintf("Failed to write file: %v", err), http.StatusInternalServerError)
		return
	}
	if err := tmpFile.Close(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to finalize temp file: %v", err), http.StatusInternalServerError)
		return
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			logging.Error.Printf("Failed to remove temp file %s: %v", tmpPath, removeErr)
		}
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

	if mediaItem.Type == models.MediaTypeAudio {
		if err := h.jobQueue.EnqueueAudioProcessing(itemID); err != nil {
			logging.Info.Printf("Failed to enqueue audio processing jobs for %s: %v", itemID, err)
		}
	}

	if mediaItem.Type == models.MediaTypePhoto {
		if err := h.jobQueue.EnqueuePhotoProcessing(itemID); err != nil {
			logging.Info.Printf("Failed to enqueue photo processing jobs for %s: %v", itemID, err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"item_id":   itemID,
		"media_url": fmt.Sprintf("/media/%s/original/upload%s", itemID, ext),
	}); err != nil {
		logging.Error.Printf("Failed to encode upload response for item %s: %v", itemID, err)
	}
}

func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	query := h.db.Where("deleted_at IS NULL")

	user := auth.GetUser(r.Context())

	searchQuery := r.URL.Query().Get("q")
	if searchQuery != "" {
		query = query.Where("LOWER(title) LIKE LOWER(?)", "%"+searchQuery+"%")
	}

	itemType := r.URL.Query().Get("type")
	if itemType != "" {
		query = query.Where("type = ?", itemType)
	}

	personaID := r.URL.Query().Get("persona_id")
	if personaID != "" {
		query = query.Where("persona_id = ?", personaID)
	}

	highlighted := r.URL.Query().Get("highlighted")
	if highlighted == "1" {
		query = query.Where("is_highlighted = ?", true)
	}

	favoritesOnly := r.URL.Query().Get("favorites")
	if favoritesOnly == "1" && user != nil {
		var favoriteItemIDs []string
		h.db.Model(&models.Favorite{}).Where("user_id = ?", user.ID).Pluck("item_id", &favoriteItemIDs)
		if len(favoriteItemIDs) > 0 {
			query = query.Where("id IN ?", favoriteItemIDs)
		} else {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(ListItemsResponse{
				Items:   []MediaItemResponse{},
				HasMore: false,
			}); err != nil {
				logging.Error.Printf("Failed to encode empty list items response: %v", err)
			}
			return
		}
	}

	userParam := r.URL.Query().Get("user")
	if userParam == "me" && user != nil {
		query = query.Where("user_id = ?", user.ID)
	}

	limit := 24
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	sortOrder := "created_at DESC, id DESC"
	sortParam := r.URL.Query().Get("sort")
	sortDescending := true
	if sortParam == "old" {
		sortOrder = "created_at ASC, id ASC"
		sortDescending = false
	}

	cursor := r.URL.Query().Get("cursor")
	if cursor != "" {
		if decodedBytes, err := base64.StdEncoding.DecodeString(cursor); err == nil {
			parts := strings.Split(string(decodedBytes), "|")
			if len(parts) == 2 {
				if tsNanos, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
					cursorTime := time.Unix(0, tsNanos).UTC()
					cursorID := parts[1]
					if sortDescending {
						query = query.Where("(created_at < ? OR (created_at = ? AND id < ?))", cursorTime, cursorTime, cursorID)
					} else {
						query = query.Where("(created_at > ? OR (created_at = ? AND id > ?))", cursorTime, cursorTime, cursorID)
					}
				}
			}
		}
	}

	var items []models.MediaItem
	query = query.Preload("Persona").Order(sortOrder).Limit(limit + 1)
	if err := query.Find(&items).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to list items: %v", err), http.StatusInternalServerError)
		return
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	var nextCursor string
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d|%s", last.CreatedAt.UnixNano(), last.ID)))
	}

	userFavoritedIDs := map[string]bool{}
	if user != nil {
		if len(items) > 0 {
			var itemIDs []string
			for _, item := range items {
				itemIDs = append(itemIDs, item.ID)
			}
			var favorites []models.Favorite
			h.db.Where("user_id = ? AND item_id IN ?", user.ID, itemIDs).Find(&favorites)
			for _, f := range favorites {
				userFavoritedIDs[f.ItemID] = true
			}
		}
	}

	responses := make([]MediaItemResponse, len(items))
	for i, item := range items {
		resp := MediaItemResponse{
			ID:            item.ID,
			UserID:        item.UserID,
			Type:          item.Type,
			Title:         item.Title,
			Description:   item.Description,
			CreatedAt:     item.CreatedAt.Format(time.RFC3339),
			IsFavorited:   userFavoritedIDs[item.ID],
			IsHighlighted: item.IsHighlighted,
		}
		resp.CanManage = h.canManageItem(user, &item)

		if item.PersonaID != nil {
			resp.PersonaID = item.PersonaID
			if item.Persona != nil {
				resp.PersonaName = &item.Persona.DisplayName
				resp.PersonaSlug = &item.Persona.Slug
				if item.Persona.AvatarPath != "" {
					avatarURL := avatarURL(item.Persona.AvatarPath)
					resp.PersonaAvatarURL = &avatarURL
				}
			}
		}

		itemMeta, err := meta.ReadItemMetaByID(h.mediaRoot, item.ID)
		if err == nil && itemMeta.Original.Path != "" {
			resp.MediaURL = "/media/" + item.ID + "/" + itemMeta.Original.Path
		}

		assetsMeta, _ := meta.ReadAssetsMetaByID(h.mediaRoot, item.ID)
		if item.Type == models.MediaTypePhoto && assetsMeta != nil {
			for _, photo := range assetsMeta.Photos {
				if photo.Kind == "display" {
					resp.MediaURL = "/media/" + item.ID + "/" + photo.StoragePath
					break
				}
			}
		}

		if item.Type == models.MediaTypeVideo {
			status, _ := h.jobQueue.GetProcessingStatus(item.ID)
			resp.ProcessingStatus = status

			if assetsMeta != nil {
				for _, asset := range assetsMeta.Assets {
					if asset.Kind == "master_mp4" {
						resp.MasterURL = "/media/" + item.ID + "/" + asset.StoragePath
					}
					if asset.Kind == "hls" {
						resp.HLSURL = "/media/" + item.ID + "/" + asset.StoragePath
					}
				}
				if len(assetsMeta.Thumbnails) > 0 {
					resp.ThumbURLs = []string{"/media/" + item.ID + "/" + assetsMeta.Thumbnails[0].StoragePath}
				}
			}
		}

		if item.Type == models.MediaTypeAudio {
			status, _ := h.jobQueue.GetProcessingStatus(item.ID)
			resp.ProcessingStatus = status

			if assetsMeta != nil {
				for _, asset := range assetsMeta.Assets {
					if asset.Kind == "master_m4a" {
						resp.MasterURL = "/media/" + item.ID + "/" + asset.StoragePath
					}
				}
			}
		}

		responses[i] = resp
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ListItemsResponse{
		Items:   responses,
		Cursor:  nextCursor,
		HasMore: hasMore,
	}); err != nil {
		logging.Error.Printf("Failed to encode list items response: %v", err)
	}
}

func (h *Handler) GetItem(w http.ResponseWriter, r *http.Request, itemID string) {
	if itemID == "" {
		http.Error(w, "Item ID required", http.StatusBadRequest)
		return
	}

	var item models.MediaItem
	if err := h.db.Preload("Persona").First(&item, "id = ?", itemID).Error; err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	user := auth.GetUser(r.Context())
	isFavorited := false
	if user != nil {
		var favorite models.Favorite
		if err := h.db.Where("user_id = ? AND item_id = ?", user.ID, itemID).First(&favorite).Error; err == nil {
			isFavorited = true
		}
	}

	itemMeta, _ := meta.ReadItemMetaByID(h.mediaRoot, itemID)
	assetsMeta, _ := meta.ReadAssetsMetaByID(h.mediaRoot, itemID)

	resp := MediaItemResponse{
		ID:            item.ID,
		UserID:        item.UserID,
		Type:          item.Type,
		Title:         item.Title,
		Description:   item.Description,
		CreatedAt:     item.CreatedAt.Format(time.RFC3339),
		IsFavorited:   isFavorited,
		IsHighlighted: item.IsHighlighted,
	}
	resp.CanManage = h.canManageItem(user, &item)

	if item.PersonaID != nil {
		resp.PersonaID = item.PersonaID
		if item.Persona != nil {
			resp.PersonaName = &item.Persona.DisplayName
			resp.PersonaSlug = &item.Persona.Slug
			if item.Persona.AvatarPath != "" {
				avatarURL := avatarURL(item.Persona.AvatarPath)
				resp.PersonaAvatarURL = &avatarURL
			}
		}
	}

	if itemMeta != nil && itemMeta.Original.Path != "" {
		resp.MediaURL = "/media/" + item.ID + "/" + itemMeta.Original.Path
	}

	if item.Type == models.MediaTypePhoto && assetsMeta != nil {
		for _, photo := range assetsMeta.Photos {
			if photo.Kind == "display" {
				resp.MediaURL = "/media/" + item.ID + "/" + photo.StoragePath
				break
			}
		}
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
				}
				if asset.Kind == "hls" {
					resp.HLSURL = "/media/" + item.ID + "/" + asset.StoragePath
				}
			}

			for _, thumb := range assetsMeta.Thumbnails {
				resp.ThumbURLs = append(resp.ThumbURLs, "/media/"+item.ID+"/"+thumb.StoragePath)
			}
		}
	}

	if item.Type == models.MediaTypeAudio {
		status, _ := h.jobQueue.GetProcessingStatus(itemID)
		resp.ProcessingStatus = status

		if status == "failed" {
			if errMsg, _ := h.jobQueue.GetJobError(itemID); errMsg != "" {
				resp.ErrorMessage = errMsg
			}
		}

		if assetsMeta != nil {
			for _, asset := range assetsMeta.Assets {
				if asset.Kind == "master_m4a" {
					resp.MasterURL = "/media/" + item.ID + "/" + asset.StoragePath
				}
			}
		}
	}

	if item.DeletedAt.Valid {
		deletedAt := item.DeletedAt.Time.Format(time.RFC3339)
		resp.DeletedAt = &deletedAt
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logging.Error.Printf("Failed to encode item response for %s: %v", itemID, err)
	}
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

	if !h.canManageItem(user, &item) {
		http.Error(w, "Forbidden - only owner or admin can delete", http.StatusForbidden)
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
		if writeErr := meta.WriteItemMetaAtomic(h.mediaRoot, itemID, itemMeta); writeErr != nil {
			logging.Error.Printf("Failed to update item meta delete state for %s: %v", itemID, writeErr)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "deleted",
	}); err != nil {
		logging.Error.Printf("Failed to encode delete response for %s: %v", itemID, err)
	}
}

func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request, itemID string) {
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
	if !h.canManageItem(user, &item) {
		http.Error(w, "Forbidden - only owner or admin can edit", http.StatusForbidden)
		return
	}

	var req UpdateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	updates := map[string]interface{}{}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			http.Error(w, "title cannot be empty", http.StatusBadRequest)
			return
		}
		updates["title"] = title
		item.Title = title
	}
	if req.Description != nil {
		desc := strings.TrimSpace(*req.Description)
		updates["description"] = desc
		item.Description = desc
	}
	if len(updates) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	if err := h.db.Model(&item).Updates(updates).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to update item: %v", err), http.StatusInternalServerError)
		return
	}

	itemMeta, err := meta.ReadItemMetaByID(h.mediaRoot, itemID)
	if err == nil && itemMeta != nil {
		itemMeta.Title = item.Title
		itemMeta.Description = item.Description
		if writeErr := meta.WriteItemMetaAtomic(h.mediaRoot, itemID, itemMeta); writeErr != nil {
			logging.Error.Printf("Failed to write item meta for %s: %v", itemID, writeErr)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "updated"}); err != nil {
		logging.Error.Printf("Failed to encode update item response for %s: %v", itemID, err)
	}
}

func (h *Handler) ReprocessItem(w http.ResponseWriter, r *http.Request, itemID string) {
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
	if !h.canManageItem(user, &item) {
		http.Error(w, "Forbidden - only owner or admin can reprocess", http.StatusForbidden)
		return
	}

	// Clear failed jobs for this item to avoid stale noise in status/error reporting.
	_ = h.db.Where("payload_json LIKE ? AND status = ?", "%"+itemID+"%", models.JobStatusFailed).
		Delete(&models.Job{}).Error

	var err error
	switch item.Type {
	case models.MediaTypeVideo:
		err = h.jobQueue.EnqueueVideoProcessing(itemID)
	case models.MediaTypeAudio:
		err = h.jobQueue.EnqueueAudioProcessing(itemID)
	case models.MediaTypePhoto:
		err = h.jobQueue.EnqueuePhotoProcessing(itemID)
	default:
		http.Error(w, "Unsupported media type for reprocess", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to enqueue reprocess: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "queued"}); err != nil {
		logging.Error.Printf("Failed to encode reprocess response for %s: %v", itemID, err)
	}
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

type SetThumbnailRequest struct {
	TimestampMs int64 `json:"timestamp_ms"`
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
	if !h.canManageItem(user, &item) {
		http.Error(w, "Forbidden - only owner or admin can create clips", http.StatusForbidden)
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
		if err := json.NewEncoder(w).Encode(CreateClipResponse{
			ClipID:  "",
			Status:  "not_ready",
			Message: fmt.Sprintf("Video processing status: %s", status),
		}); err != nil {
			logging.Error.Printf("Failed to encode not-ready clip response for %s: %v", itemID, err)
		}
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
	if err := json.NewEncoder(w).Encode(CreateClipResponse{
		ClipID: clipID,
		Status: "queued",
	}); err != nil {
		logging.Error.Printf("Failed to encode create clip response for %s: %v", itemID, err)
	}
}

func (h *Handler) SetItemThumbnail(w http.ResponseWriter, r *http.Request, itemID string) {
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
		http.Error(w, "Thumbnails can only be set for videos", http.StatusBadRequest)
		return
	}
	if !h.canManageItem(user, &item) {
		http.Error(w, "Forbidden - only owner or admin can set thumbnail", http.StatusForbidden)
		return
	}

	var req SetThumbnailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.TimestampMs < 0 {
		http.Error(w, "timestamp_ms must be >= 0", http.StatusBadRequest)
		return
	}

	processor := video.NewProcessor(h.mediaRoot)
	thumb, err := processor.SetThumbnailAt(itemID, float64(req.TimestampMs)/1000.0)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate thumbnail: %v", err), http.StatusInternalServerError)
		return
	}

	assetsMeta, err := meta.ReadAssetsMetaByID(h.mediaRoot, itemID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read assets meta: %v", err), http.StatusInternalServerError)
		return
	}
	if assetsMeta == nil {
		assetsMeta = &meta.AssetsMeta{Schema: meta.SchemaVersion}
	}

	newThumbs := []meta.Thumbnail{thumb}
	for _, existing := range assetsMeta.Thumbnails {
		if existing.StoragePath == thumb.StoragePath {
			continue
		}
		newThumbs = append(newThumbs, existing)
	}
	assetsMeta.Thumbnails = newThumbs

	if err := meta.WriteAssetsMetaAtomic(h.mediaRoot, itemID, assetsMeta); err != nil {
		http.Error(w, fmt.Sprintf("Failed to write assets meta: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":    "updated",
		"thumb_url": "/media/" + itemID + "/" + thumb.StoragePath,
	}); err != nil {
		logging.Error.Printf("Failed to encode set thumbnail response for %s: %v", itemID, err)
	}
}

func (h *Handler) DeleteClip(w http.ResponseWriter, r *http.Request, itemID string, clipID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if itemID == "" || clipID == "" {
		http.Error(w, "Item ID and clip ID are required", http.StatusBadRequest)
		return
	}

	var item models.MediaItem
	if err := h.db.First(&item, "id = ?", itemID).Error; err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	if !h.canManageItem(user, &item) {
		http.Error(w, "Forbidden - only owner or admin can delete clips", http.StatusForbidden)
		return
	}

	var clip models.ClipAsset
	if err := h.db.Where("id = ? AND item_id = ?", clipID, itemID).First(&clip).Error; err != nil {
		http.Error(w, "Clip not found", http.StatusNotFound)
		return
	}

	// Best-effort cleanup of queued/failed clip jobs for this clip.
	_ = h.db.Where(
		"type = ? AND (status = ? OR status = ?) AND payload_json LIKE ? AND payload_json LIKE ?",
		models.JobTypeClip,
		models.JobStatusQueued,
		models.JobStatusFailed,
		"%\"item_id\":\""+itemID+"\"%",
		"%\"clip_id\":\""+clipID+"\"%",
	).Delete(&models.Job{}).Error

	if err := h.db.Delete(&clip).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete clip: %v", err), http.StatusInternalServerError)
		return
	}

	clipPath := filepath.Join(h.mediaRoot, "items", itemID, "derived", fmt.Sprintf("short_%s.mp4", clipID))
	_ = os.Remove(clipPath)

	assetsMeta, err := meta.ReadAssetsMetaByID(h.mediaRoot, itemID)
	if err == nil && assetsMeta != nil {
		filtered := assetsMeta.Assets[:0]
		for _, asset := range assetsMeta.Assets {
			if asset.Kind == "short_clip" && asset.ClipID == clipID {
				continue
			}
			filtered = append(filtered, asset)
		}
		assetsMeta.Assets = filtered
		if writeErr := meta.WriteAssetsMetaAtomic(h.mediaRoot, itemID, assetsMeta); writeErr != nil {
			logging.Error.Printf("Failed to write assets meta after clip delete for %s/%s: %v", itemID, clipID, writeErr)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		logging.Error.Printf("Failed to encode delete clip response for %s/%s: %v", itemID, clipID, err)
	}
}

type ShortsClipResponse struct {
	ClipID       string  `json:"clip_id"`
	ItemID       string  `json:"item_id"`
	VideoURL     string  `json:"video_url,omitempty"`
	ThumbURL     string  `json:"thumb_url,omitempty"`
	Title        string  `json:"title"`
	OwnerName    string  `json:"owner_name"`
	PersonaSlug  *string `json:"persona_slug,omitempty"`
	UpCount      int     `json:"up_count"`
	DownCount    int     `json:"down_count"`
	UserReaction *int    `json:"user_reaction,omitempty"`
	CreatedAt    string  `json:"created_at"`
	Status       string  `json:"status"`
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
		Select("clip_assets.*, media_items.title, users.username as owner_name, personas.slug as persona_slug").
		Joins("JOIN media_items ON media_items.id = clip_assets.item_id").
		Joins("JOIN users ON users.id = media_items.user_id").
		Joins("LEFT JOIN personas ON personas.id = media_items.persona_id").
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
		Title       string
		OwnerName   string
		PersonaSlug *string
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
			PersonaSlug:  clip.PersonaSlug,
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
	if err := json.NewEncoder(w).Encode(ListShortsResponse{
		Clips:   responses,
		Cursor:  nextCursor,
		HasMore: hasMore,
	}); err != nil {
		logging.Error.Printf("Failed to encode list shorts response: %v", err)
	}
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
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"up_count":      upCount,
		"down_count":    downCount,
		"user_reaction": userReaction,
	}); err != nil {
		logging.Error.Printf("Failed to encode reaction response for %s: %v", itemID, err)
	}
}

type SetFavoriteRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *Handler) SetFavorite(w http.ResponseWriter, r *http.Request, itemID string) {
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

	var req SetFavoriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Enabled {
		favorite := models.Favorite{
			UserID:    user.ID,
			ItemID:    itemID,
			CreatedAt: time.Now(),
		}
		h.db.Where("user_id = ? AND item_id = ?", user.ID, itemID).Delete(&models.Favorite{})
		if err := h.db.Create(&favorite).Error; err != nil {
			http.Error(w, fmt.Sprintf("Failed to favorite item: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		h.db.Where("user_id = ? AND item_id = ?", user.ID, itemID).Delete(&models.Favorite{})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]bool{
		"is_favorited": req.Enabled,
	}); err != nil {
		logging.Error.Printf("Failed to encode favorite response for %s: %v", itemID, err)
	}
}

type SetHighlightRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *Handler) SetHighlight(w http.ResponseWriter, r *http.Request, itemID string) {
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

	if !h.canManageItem(user, &item) {
		http.Error(w, "Forbidden - only owner or admin can toggle highlight", http.StatusForbidden)
		return
	}

	var req SetHighlightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := h.db.Model(&item).Update("is_highlighted", req.Enabled).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to update highlight: %v", err), http.StatusInternalServerError)
		return
	}

	itemMeta, err := meta.ReadItemMetaByID(h.mediaRoot, itemID)
	if err == nil {
		itemMeta.State.Highlighted = req.Enabled
		if writeErr := meta.WriteItemMetaAtomic(h.mediaRoot, itemID, itemMeta); writeErr != nil {
			logging.Error.Printf("Failed to update item meta highlight state for %s: %v", itemID, writeErr)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]bool{
		"is_highlighted": req.Enabled,
	}); err != nil {
		logging.Error.Printf("Failed to encode highlight response for %s: %v", itemID, err)
	}
}

func (h *Handler) canManageItem(user *models.User, item *models.MediaItem) bool {
	if user == nil || item == nil {
		return false
	}
	if user.Role == models.RoleAdmin {
		return true
	}
	if item.UserID == user.ID {
		return true
	}
	if item.PersonaID == nil || *item.PersonaID == "" {
		return false
	}

	var persona models.Persona
	if err := h.db.Select("id", "user_id").First(&persona, "id = ?", *item.PersonaID).Error; err != nil {
		return false
	}

	return persona.UserID == user.ID
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
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"clips": responses,
	}); err != nil {
		logging.Error.Printf("Failed to encode clips response for %s: %v", itemID, err)
	}
}

type CurrentUserResponse struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	IsActive bool   `json:"is_active"`
}

type AdminUserResponse struct {
	ID            uint    `json:"id"`
	Username      string  `json:"username"`
	Role          string  `json:"role"`
	IsActive      bool    `json:"is_active"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
	DeactivatedAt *string `json:"deactivated_at,omitempty"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	if !auth.VerifyPassword(req.Password, user.PasswordHash) {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}
	if !user.IsActive {
		http.Error(w, "Account is deactivated", http.StatusForbidden)
		return
	}

	token, err := auth.NewSessionToken()
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().UTC().Add(auth.SessionDuration)
	session := models.Session{
		ID:        token,
		UserID:    user.ID,
		ExpiresAt: expiresAt,
	}

	if err := h.db.Create(&session).Error; err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	auth.SetSessionCookie(w, token, expiresAt)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(CurrentUserResponse{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
		IsActive: user.IsActive,
	}); err != nil {
		logging.Error.Printf("Failed to encode login response for %s: %v", user.Username, err)
	}
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil && cookie.Value != "" {
		h.db.Delete(&models.Session{}, "id = ?", cookie.Value)
	}

	auth.ClearSessionCookie(w)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	}); err != nil {
		logging.Error.Printf("Failed to encode logout response: %v", err)
	}
}

func (h *Handler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(CurrentUserResponse{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
		IsActive: user.IsActive,
	}); err != nil {
		logging.Error.Printf("Failed to encode current user response for %s: %v", user.Username, err)
	}
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (*models.User, bool) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	if !user.IsActive {
		http.Error(w, "Account is deactivated", http.StatusForbidden)
		return nil, false
	}
	if user.Role != models.RoleAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return nil, false
	}
	return user, true
}

func userToAdminResponse(user models.User) AdminUserResponse {
	var deactivatedAt *string
	if user.DeactivatedAt != nil {
		ts := user.DeactivatedAt.UTC().Format(time.RFC3339)
		deactivatedAt = &ts
	}
	return AdminUserResponse{
		ID:            user.ID,
		Username:      user.Username,
		Role:          user.Role,
		IsActive:      user.IsActive,
		CreatedAt:     user.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     user.UpdatedAt.UTC().Format(time.RFC3339),
		DeactivatedAt: deactivatedAt,
	}
}

type CreateAdminUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (h *Handler) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}

	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	query := h.db.Model(&models.User{})

	switch status {
	case "", "all":
	case "active":
		query = query.Where("is_active = ?", true)
	case "inactive":
		query = query.Where("is_active = ?", false)
	default:
		http.Error(w, "Invalid status filter", http.StatusBadRequest)
		return
	}

	var users []models.User
	if err := query.Order("id ASC").Find(&users).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to list users: %v", err), http.StatusInternalServerError)
		return
	}

	resp := make([]AdminUserResponse, 0, len(users))
	for _, user := range users {
		resp = append(resp, userToAdminResponse(user))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"users": resp,
	}); err != nil {
		logging.Error.Printf("Failed to encode admin user list response: %v", err)
	}
}

func (h *Handler) AdminCreateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}

	var req CreateAdminUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || strings.TrimSpace(req.Password) == "" {
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	role := req.Role
	if strings.TrimSpace(role) == "" {
		role = models.RoleUser
	}
	role, err := useradmin.NormalizeRole(role)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var existing models.User
	if err := h.db.Where("username = ?", req.Username).First(&existing).Error; err == nil {
		http.Error(w, "Username already exists", http.StatusConflict)
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	user := models.User{
		Username:     req.Username,
		PasswordHash: passwordHash,
		Role:         role,
		IsActive:     true,
	}
	if err := h.db.Create(&user).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to create user: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(userToAdminResponse(user)); err != nil {
		logging.Error.Printf("Failed to encode admin create user response for %s: %v", user.Username, err)
	}
}

type SetAdminUserRoleRequest struct {
	Role string `json:"role"`
}

func (h *Handler) AdminSetUserRole(w http.ResponseWriter, r *http.Request, username string) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}

	var req SetAdminUserRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	role, err := useradmin.NormalizeRole(req.Role)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.Where("username = ?", username).First(&user).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	if user.Role == role {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(userToAdminResponse(user)); err != nil {
			logging.Error.Printf("Failed to encode admin set-role noop response for %s: %v", user.Username, err)
		}
		return
	}
	if err := useradmin.EnsureCanChangeRole(h.db, user, role); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	if err := h.db.Model(&user).Update("role", role).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to update role: %v", err), http.StatusInternalServerError)
		return
	}
	user.Role = role

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(userToAdminResponse(user)); err != nil {
		logging.Error.Printf("Failed to encode admin set-role response for %s: %v", user.Username, err)
	}
}

func (h *Handler) AdminDeactivateUser(w http.ResponseWriter, r *http.Request, username string) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}

	var user models.User
	if err := h.db.Where("username = ?", username).First(&user).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	if !user.IsActive {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(userToAdminResponse(user)); err != nil {
			logging.Error.Printf("Failed to encode admin deactivate noop response for %s: %v", user.Username, err)
		}
		return
	}
	if err := useradmin.EnsureCanDeactivate(h.db, user); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	now := time.Now().UTC()
	if err := h.db.Model(&user).Updates(map[string]interface{}{
		"is_active":      false,
		"deactivated_at": now,
	}).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to deactivate user: %v", err), http.StatusInternalServerError)
		return
	}

	h.db.Where("user_id = ?", user.ID).Delete(&models.Session{})

	user.IsActive = false
	user.DeactivatedAt = &now
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(userToAdminResponse(user)); err != nil {
		logging.Error.Printf("Failed to encode admin deactivate response for %s: %v", user.Username, err)
	}
}

func (h *Handler) AdminActivateUser(w http.ResponseWriter, r *http.Request, username string) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}

	var user models.User
	if err := h.db.Where("username = ?", username).First(&user).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	if user.IsActive {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(userToAdminResponse(user)); err != nil {
			logging.Error.Printf("Failed to encode admin activate noop response for %s: %v", user.Username, err)
		}
		return
	}

	if err := h.db.Model(&user).Updates(map[string]interface{}{
		"is_active":      true,
		"deactivated_at": nil,
	}).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to activate user: %v", err), http.StatusInternalServerError)
		return
	}

	user.IsActive = true
	user.DeactivatedAt = nil
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(userToAdminResponse(user)); err != nil {
		logging.Error.Printf("Failed to encode admin activate response for %s: %v", user.Username, err)
	}
}

type ResetAdminUserPasswordRequest struct {
	Password string `json:"password"`
}

func (h *Handler) AdminSetUserPassword(w http.ResponseWriter, r *http.Request, username string) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}

	var req ResetAdminUserPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Password) == "" {
		http.Error(w, "password is required", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.Where("username = ?", username).First(&user).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}
	if err := h.db.Model(&user).Update("password_hash", passwordHash).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to update password: %v", err), http.StatusInternalServerError)
		return
	}

	h.db.Where("user_id = ?", user.ID).Delete(&models.Session{})

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		logging.Error.Printf("Failed to encode admin password response for %s: %v", user.Username, err)
	}
}

type CreatePersonaRequest struct {
	DisplayName string `json:"display_name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

type PersonaResponse struct {
	ID          string `json:"id"`
	UserID      uint   `json:"user_id"`
	DisplayName string `json:"display_name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	AvatarURL   string `json:"avatar_url"`
	CreatedAt   string `json:"created_at"`
}

func (h *Handler) CreatePersona(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreatePersonaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.DisplayName == "" {
		http.Error(w, "display_name is required", http.StatusBadRequest)
		return
	}

	req.Description = strings.TrimSpace(req.Description)

	baseSlug := normalizeSlug(req.DisplayName)
	if req.Slug != "" {
		baseSlug = normalizeSlug(req.Slug)
	}
	if baseSlug == "" {
		http.Error(w, "invalid slug/display_name", http.StatusBadRequest)
		return
	}

	// If a matching slug exists but is soft-deleted for this user, restore it in place.
	var deletedMatch models.Persona
	if err := h.db.Unscoped().Where("user_id = ? AND slug = ?", user.ID, baseSlug).First(&deletedMatch).Error; err == nil && deletedMatch.DeletedAt.Valid {
		deletedMatch.DisplayName = req.DisplayName
		deletedMatch.Description = req.Description
		deletedMatch.DeletedAt = gorm.DeletedAt{}
		deletedMatch.UpdatedAt = time.Now()
		if saveErr := h.db.Unscoped().Save(&deletedMatch).Error; saveErr != nil {
			http.Error(w, fmt.Sprintf("Failed to restore persona: %v", saveErr), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(personaToResponse(deletedMatch)); err != nil {
			logging.Error.Printf("Failed to encode restored persona response for %s: %v", deletedMatch.ID, err)
		}
		return
	}

	slug, err := h.ensureUniqueSlug(baseSlug)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create slug: %v", err), http.StatusInternalServerError)
		return
	}

	persona := models.Persona{
		ID:          id.NewULID(),
		UserID:      user.ID,
		DisplayName: req.DisplayName,
		Slug:        slug,
		Description: req.Description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := h.db.Create(&persona).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to create persona: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(personaToResponse(persona)); err != nil {
		logging.Error.Printf("Failed to encode create persona response for %s: %v", persona.ID, err)
	}
}

func (h *Handler) ListPersonas(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var personas []models.Persona
	if err := h.db.Where("user_id = ?", user.ID).Order("created_at DESC").Find(&personas).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to list personas: %v", err), http.StatusInternalServerError)
		return
	}

	responses := make([]PersonaResponse, len(personas))
	for i, p := range personas {
		responses[i] = personaToResponse(p)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"personas": responses,
	}); err != nil {
		logging.Error.Printf("Failed to encode list personas response: %v", err)
	}
}

type UpdatePersonaRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (h *Handler) UpdatePersona(w http.ResponseWriter, r *http.Request, personaID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if personaID == "" {
		http.Error(w, "Persona ID required", http.StatusBadRequest)
		return
	}

	var persona models.Persona
	if err := h.db.First(&persona, "id = ?", personaID).Error; err != nil {
		http.Error(w, "Persona not found", http.StatusNotFound)
		return
	}

	if persona.UserID != user.ID {
		http.Error(w, "Forbidden - you can only update your own personas", http.StatusForbidden)
		return
	}

	var req UpdatePersonaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.DisplayName != nil {
		persona.DisplayName = *req.DisplayName
	}

	if req.Slug != nil {
		newSlug := normalizeSlug(*req.Slug)
		if newSlug == "" {
			http.Error(w, "Invalid slug", http.StatusBadRequest)
			return
		}
		existingPersona := models.Persona{}
		if err := h.db.Unscoped().Where("slug = ? AND id != ?", newSlug, personaID).First(&existingPersona).Error; err == nil {
			http.Error(w, "A persona with this slug already exists", http.StatusConflict)
			return
		}
		persona.Slug = newSlug
	}

	if req.Description != nil {
		persona.Description = strings.TrimSpace(*req.Description)
	}

	persona.UpdatedAt = time.Now()

	if err := h.db.Save(&persona).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to update persona: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(personaToResponse(persona)); err != nil {
		logging.Error.Printf("Failed to encode update persona response for %s: %v", persona.ID, err)
	}
}

func (h *Handler) DeletePersona(w http.ResponseWriter, r *http.Request, personaID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if personaID == "" {
		http.Error(w, "Persona ID required", http.StatusBadRequest)
		return
	}

	var persona models.Persona
	if err := h.db.First(&persona, "id = ?", personaID).Error; err != nil {
		http.Error(w, "Persona not found", http.StatusNotFound)
		return
	}

	if persona.UserID != user.ID {
		http.Error(w, "Forbidden - you can only delete your own personas", http.StatusForbidden)
		return
	}

	if err := h.db.Delete(&persona).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete persona: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "deleted",
	}); err != nil {
		logging.Error.Printf("Failed to encode delete persona response for %s: %v", personaID, err)
	}
}

func (h *Handler) GetPersona(w http.ResponseWriter, r *http.Request, personaID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if personaID == "" {
		http.Error(w, "Persona ID required", http.StatusBadRequest)
		return
	}

	var persona models.Persona
	if err := h.db.First(&persona, "id = ?", personaID).Error; err != nil {
		http.Error(w, "Persona not found", http.StatusNotFound)
		return
	}

	if persona.UserID != user.ID {
		http.Error(w, "Forbidden - you can only view your own personas", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(personaToResponse(persona)); err != nil {
		logging.Error.Printf("Failed to encode get persona response for %s: %v", persona.ID, err)
	}
}

func avatarURL(avatarPath string) string {
	if avatarPath == "" {
		return ""
	}
	return "/media/" + avatarPath
}

// normalizeSlug converts a string to a URL-friendly slug
func normalizeSlug(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)
	// Replace spaces with hyphens
	s = strings.ReplaceAll(s, " ", "-")
	// Remove any characters that are not lowercase letters, numbers, or hyphens
	var result strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result.WriteRune(c)
		}
	}
	// Remove leading/trailing hyphens
	s = strings.Trim(result.String(), "-")
	// Replace multiple hyphens with single
	s = strings.ReplaceAll(s, "--", "-")
	return s
}

// ensureUniqueSlug ensures the slug is globally unique, appending -2, -3, etc. if needed.
func (h *Handler) ensureUniqueSlug(baseSlug string) (string, error) {
	slug := baseSlug
	counter := 1

	for {
		var existing models.Persona
		err := h.db.Unscoped().Where("slug = ?", slug).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			return slug, nil
		}
		if err != nil {
			return "", err
		}

		counter++
		slug = fmt.Sprintf("%s-%d", baseSlug, counter)
	}
}

func personaToResponse(p models.Persona) PersonaResponse {
	return PersonaResponse{
		ID:          p.ID,
		UserID:      p.UserID,
		DisplayName: p.DisplayName,
		Slug:        p.Slug,
		Description: p.Description,
		AvatarURL:   avatarURL(p.AvatarPath),
		CreatedAt:   p.CreatedAt.Format(time.RFC3339),
	}
}

type UploadAvatarResponse struct {
	AvatarURL string `json:"avatar_url"`
}

func (h *Handler) UploadAvatar(w http.ResponseWriter, r *http.Request, personaID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if personaID == "" {
		http.Error(w, "Persona ID required", http.StatusBadRequest)
		return
	}

	var persona models.Persona
	if err := h.db.First(&persona, "id = ?", personaID).Error; err != nil {
		http.Error(w, "Persona not found", http.StatusNotFound)
		return
	}

	if persona.UserID != user.ID {
		http.Error(w, "Forbidden - you can only upload avatars for your own personas", http.StatusForbidden)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logging.Error.Printf("Failed to close avatar upload file for persona %s: %v", personaID, closeErr)
		}
	}()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		http.Error(w, "Invalid file type: must be jpeg, png, or webp", http.StatusBadRequest)
		return
	}
	if ext == ".jpeg" {
		ext = ".jpg"
	}

	if header.Size > 5*1024*1024 {
		http.Error(w, "File too large: maximum 5MB", http.StatusBadRequest)
		return
	}

	if err := storage.EnsureAvatarDir(h.mediaRoot, user.ID, personaID); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create avatar directory: %v", err), http.StatusInternalServerError)
		return
	}

	tmpDir := storage.TmpUploadsDir(h.mediaRoot)
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("avatar-%s-%d%s", personaID, time.Now().UnixNano(), ext))

	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create temp file: %v", err), http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(tmpFile, file); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			logging.Error.Printf("Failed to close temp avatar file %s: %v", tmpPath, closeErr)
		}
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			logging.Error.Printf("Failed to remove temp avatar file %s: %v", tmpPath, removeErr)
		}
		http.Error(w, fmt.Sprintf("Failed to write file: %v", err), http.StatusInternalServerError)
		return
	}
	if err := tmpFile.Close(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to finalize avatar upload: %v", err), http.StatusInternalServerError)
		return
	}

	avatarPath := storage.AvatarPathWithExt(h.mediaRoot, user.ID, personaID, ext)

	if err := os.Rename(tmpPath, avatarPath); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			logging.Error.Printf("Failed to remove temp avatar file %s: %v", tmpPath, removeErr)
		}
		http.Error(w, fmt.Sprintf("Failed to save avatar: %v", err), http.StatusInternalServerError)
		return
	}

	// Keep only one active avatar file to make updates deterministic.
	avatarDir := storage.AvatarDir(h.mediaRoot, user.ID, personaID)
	oldAvatarCandidates, globErr := filepath.Glob(filepath.Join(avatarDir, "avatar.*"))
	if globErr == nil {
		for _, candidate := range oldAvatarCandidates {
			if candidate == avatarPath {
				continue
			}
			if removeErr := os.Remove(candidate); removeErr != nil && !os.IsNotExist(removeErr) {
				logging.Error.Printf("Failed to remove old avatar %s: %v", candidate, removeErr)
			}
		}
	}

	relPath := storage.AvatarRelativePathWithExt(user.ID, personaID, ext)
	persona.AvatarPath = relPath
	persona.UpdatedAt = time.Now()

	if err := h.db.Save(&persona).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to update persona: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(UploadAvatarResponse{
		AvatarURL: avatarURL(relPath),
	}); err != nil {
		logging.Error.Printf("Failed to encode upload avatar response for %s: %v", personaID, err)
	}
}

type ProfileResponse struct {
	PersonaID   string `json:"persona_id"`
	DisplayName string `json:"display_name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	AvatarURL   string `json:"avatar_url"`
	CreatedAt   string `json:"created_at"`
	Counts      struct {
		Videos int64 `json:"videos"`
		Shorts int64 `json:"shorts"`
	} `json:"counts"`
}

func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request, slug string) {
	if slug == "" {
		http.Error(w, "Slug required", http.StatusBadRequest)
		return
	}

	var persona models.Persona
	if err := h.db.First(&persona, "slug = ?", slug).Error; err != nil {
		http.Error(w, "Profile not found", http.StatusNotFound)
		return
	}

	var videoCount, shortCount int64

	h.db.Model(&models.MediaItem{}).Where("persona_id = ? AND deleted_at IS NULL AND type = ?", persona.ID, models.MediaTypeVideo).Count(&videoCount)
	h.db.Model(&models.ClipAsset{}).Joins("JOIN media_items ON media_items.id = clip_assets.item_id").Where("media_items.persona_id = ? AND media_items.deleted_at IS NULL AND clip_assets.status = ?", persona.ID, models.ClipStatusReady).Count(&shortCount)

	resp := ProfileResponse{
		PersonaID:   persona.ID,
		DisplayName: persona.DisplayName,
		Slug:        persona.Slug,
		Description: persona.Description,
		AvatarURL:   avatarURL(persona.AvatarPath),
		CreatedAt:   persona.CreatedAt.Format(time.RFC3339),
	}
	resp.Counts.Videos = videoCount
	resp.Counts.Shorts = shortCount

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logging.Error.Printf("Failed to encode profile response for %s: %v", slug, err)
	}
}

type ProfileItemResponse struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Title         string `json:"title"`
	CreatedAt     string `json:"created_at"`
	ThumbURL      string `json:"thumb_url,omitempty"`
	MasterURL     string `json:"master_url,omitempty"`
	IsFavorited   bool   `json:"is_favorited"`
	IsHighlighted bool   `json:"is_highlighted"`
}

type ProfileItemsResponse struct {
	Items      []ProfileItemResponse `json:"items"`
	NextCursor string                `json:"next_cursor,omitempty"`
	HasMore    bool                  `json:"has_more"`
}

func (h *Handler) GetProfileItems(w http.ResponseWriter, r *http.Request, slug string) {
	if slug == "" {
		http.Error(w, "Slug required", http.StatusBadRequest)
		return
	}

	var persona models.Persona
	if err := h.db.First(&persona, "slug = ?", slug).Error; err != nil {
		http.Error(w, "Profile not found", http.StatusNotFound)
		return
	}

	user := auth.GetUser(r.Context())

	itemType := r.URL.Query().Get("type")

	limit := 24
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	sortOrder := "created_at DESC"
	sortParam := r.URL.Query().Get("sort")
	if sortParam == "old" {
		sortOrder = "created_at ASC"
	}

	query := h.db.Model(&models.MediaItem{}).Where("persona_id = ? AND deleted_at IS NULL", persona.ID)
	if itemType != "" {
		query = query.Where("type = ?", itemType)
	}

	searchQuery := r.URL.Query().Get("q")
	if searchQuery != "" {
		query = query.Where("LOWER(title) LIKE LOWER(?)", "%"+searchQuery+"%")
	}

	highlighted := r.URL.Query().Get("highlighted")
	if highlighted == "1" {
		query = query.Where("is_highlighted = ?", true)
	}

	cursor := r.URL.Query().Get("cursor")
	if cursor != "" {
		decodedBytes, err := base64.StdEncoding.DecodeString(cursor)
		if err == nil {
			cursorTime, _ := time.Parse(time.RFC3339, string(decodedBytes))
			query = query.Where("created_at < ?", cursorTime)
		}
	}

	var items []models.MediaItem
	query = query.Order(sortOrder).Limit(limit + 1)
	if err := query.Find(&items).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to list items: %v", err), http.StatusInternalServerError)
		return
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	userFavoritedIDs := map[string]bool{}
	if user != nil {
		if len(items) > 0 {
			var itemIDs []string
			for _, item := range items {
				itemIDs = append(itemIDs, item.ID)
			}
			var favorites []models.Favorite
			h.db.Where("user_id = ? AND item_id IN ?", user.ID, itemIDs).Find(&favorites)
			for _, f := range favorites {
				userFavoritedIDs[f.ItemID] = true
			}
		}
	}

	responses := make([]ProfileItemResponse, len(items))
	for i, item := range items {
		resp := ProfileItemResponse{
			ID:            item.ID,
			Type:          item.Type,
			Title:         item.Title,
			CreatedAt:     item.CreatedAt.Format(time.RFC3339),
			IsFavorited:   userFavoritedIDs[item.ID],
			IsHighlighted: item.IsHighlighted,
		}

		if item.Type == models.MediaTypeVideo {
			assetsMeta, _ := meta.ReadAssetsMetaByID(h.mediaRoot, item.ID)
			if assetsMeta != nil {
				for _, asset := range assetsMeta.Assets {
					if asset.Kind == "master_mp4" {
						resp.MasterURL = "/media/" + item.ID + "/" + asset.StoragePath
						break
					}
				}
				if len(assetsMeta.Thumbnails) > 0 {
					resp.ThumbURL = "/media/" + item.ID + "/" + assetsMeta.Thumbnails[0].StoragePath
				}
			}
		}

		responses[i] = resp
	}

	var nextCursor string
	if hasMore && len(items) > 0 {
		lastItem := items[len(items)-1]
		nextCursor = base64.StdEncoding.EncodeToString([]byte(lastItem.CreatedAt.Format(time.RFC3339)))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ProfileItemsResponse{
		Items:      responses,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}); err != nil {
		logging.Error.Printf("Failed to encode profile items response for %s: %v", slug, err)
	}
}

type ProfileShortResponse struct {
	ClipID    string `json:"clip_id"`
	ItemID    string `json:"item_id"`
	VideoURL  string `json:"video_url,omitempty"`
	ThumbURL  string `json:"thumb_url,omitempty"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
}

type ProfileShortsResponse struct {
	Shorts     []ProfileShortResponse `json:"shorts"`
	NextCursor string                 `json:"next_cursor,omitempty"`
	HasMore    bool                   `json:"has_more"`
}

func (h *Handler) GetProfileShorts(w http.ResponseWriter, r *http.Request, slug string) {
	if slug == "" {
		http.Error(w, "Slug required", http.StatusBadRequest)
		return
	}

	var persona models.Persona
	if err := h.db.First(&persona, "slug = ?", slug).Error; err != nil {
		http.Error(w, "Profile not found", http.StatusNotFound)
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 25 {
			limit = parsed
		}
	}

	query := h.db.Model(&models.ClipAsset{}).
		Select("clip_assets.*, media_items.title").
		Joins("JOIN media_items ON media_items.id = clip_assets.item_id").
		Where("media_items.persona_id = ? AND media_items.deleted_at IS NULL AND clip_assets.status = ?", persona.ID, models.ClipStatusReady).
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
		Title string
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

	responses := make([]ProfileShortResponse, len(clips))
	for i, clip := range clips {
		thumbURL := ""
		assetsMeta, _ := meta.ReadAssetsMetaByID(h.mediaRoot, clip.ItemID)
		if assetsMeta != nil && len(assetsMeta.Thumbnails) > 0 {
			thumbURL = "/media/" + clip.ItemID + "/" + assetsMeta.Thumbnails[0].StoragePath
		}

		videoURL := ""
		if clip.Status == models.ClipStatusReady {
			videoURL = "/media/" + clip.ItemID + "/derived/short_" + clip.ID + ".mp4"
		}

		responses[i] = ProfileShortResponse{
			ClipID:    clip.ID,
			ItemID:    clip.ItemID,
			VideoURL:  videoURL,
			ThumbURL:  thumbURL,
			Title:     clip.Title,
			CreatedAt: clip.CreatedAt.Format(time.RFC3339),
		}
	}

	var nextCursor string
	if hasMore && len(clips) > 0 {
		lastClip := clips[len(clips)-1]
		nextCursor = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d|%s", lastClip.CreatedAt.Unix(), lastClip.ID)))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ProfileShortsResponse{
		Shorts:     responses,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}); err != nil {
		logging.Error.Printf("Failed to encode profile shorts response for %s: %v", slug, err)
	}
}
