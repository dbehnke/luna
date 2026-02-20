package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"luna/internal/auth"
	"luna/internal/logging"
	"luna/internal/meta"
	"luna/internal/models"

	"gorm.io/gorm"
)

type playlistResponse struct {
	ID        uint                       `json:"id"`
	Name      string                     `json:"name"`
	CreatedAt string                     `json:"created_at"`
	UpdatedAt string                     `json:"updated_at"`
	Items     []playlistMediaItemSummary `json:"items,omitempty"`
	ItemCount int                        `json:"item_count"`
}

type playlistMediaItemSummary struct {
	ItemID    string `json:"item_id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Position  int    `json:"position"`
	ThumbURL  string `json:"thumb_url,omitempty"`
	MasterURL string `json:"master_url,omitempty"`
}

type createPlaylistRequest struct {
	Name string `json:"name"`
}

type updatePlaylistRequest struct {
	Name string `json:"name"`
}

type addPlaylistItemRequest struct {
	ItemID   string `json:"item_id"`
	Position *int   `json:"position,omitempty"`
}

type reorderPlaylistRequest struct {
	ItemIDs []string `json:"item_ids"`
}

func (h *Handler) ListPlaylists(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	includeItems := r.URL.Query().Get("include_items") == "1"

	var playlists []models.Playlist
	if err := h.db.Where("user_id = ?", user.ID).Order("created_at DESC").Find(&playlists).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to list playlists: %v", err), http.StatusInternalServerError)
		return
	}

	resp := make([]playlistResponse, 0, len(playlists))
	for _, p := range playlists {
		out := playlistResponse{
			ID:        p.ID,
			Name:      p.Name,
			CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt: p.UpdatedAt.UTC().Format(time.RFC3339),
		}

		var count int64
		if err := h.db.Model(&models.PlaylistItem{}).Where("playlist_id = ?", p.ID).Count(&count).Error; err == nil {
			out.ItemCount = int(count)
		}

		if includeItems {
			items, err := h.playlistItemsForResponse(p.ID)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to load playlist items: %v", err), http.StatusInternalServerError)
				return
			}
			out.Items = items
		}

		resp = append(resp, out)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"playlists": resp}); err != nil {
		logging.Error.Printf("Failed to encode playlist list response: %v", err)
	}
}

func (h *Handler) CreatePlaylist(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req createPlaylistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	playlist := models.Playlist{
		UserID: user.ID,
		Name:   req.Name,
	}
	if err := h.db.Create(&playlist).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to create playlist: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(playlistResponse{
		ID:        playlist.ID,
		Name:      playlist.Name,
		CreatedAt: playlist.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: playlist.UpdatedAt.UTC().Format(time.RFC3339),
		ItemCount: 0,
	}); err != nil {
		logging.Error.Printf("Failed to encode create playlist response: %v", err)
	}
}

func (h *Handler) GetPlaylist(w http.ResponseWriter, r *http.Request, playlistID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pid, err := strconv.ParseUint(playlistID, 10, 64)
	if err != nil {
		http.Error(w, "Invalid playlist id", http.StatusBadRequest)
		return
	}

	var playlist models.Playlist
	if err := h.db.First(&playlist, "id = ?", uint(pid)).Error; err != nil {
		http.Error(w, "Playlist not found", http.StatusNotFound)
		return
	}
	if playlist.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	items, err := h.playlistItemsForResponse(playlist.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load playlist items: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(playlistResponse{
		ID:        playlist.ID,
		Name:      playlist.Name,
		CreatedAt: playlist.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: playlist.UpdatedAt.UTC().Format(time.RFC3339),
		Items:     items,
		ItemCount: len(items),
	}); err != nil {
		logging.Error.Printf("Failed to encode get playlist response: %v", err)
	}
}

func (h *Handler) UpdatePlaylist(w http.ResponseWriter, r *http.Request, playlistID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pid, err := strconv.ParseUint(playlistID, 10, 64)
	if err != nil {
		http.Error(w, "Invalid playlist id", http.StatusBadRequest)
		return
	}

	var playlist models.Playlist
	if err := h.db.First(&playlist, "id = ?", uint(pid)).Error; err != nil {
		http.Error(w, "Playlist not found", http.StatusNotFound)
		return
	}
	if playlist.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var req updatePlaylistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	if err := h.db.Model(&playlist).Update("name", req.Name).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to update playlist: %v", err), http.StatusInternalServerError)
		return
	}
	playlist.Name = req.Name

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(playlistResponse{
		ID:        playlist.ID,
		Name:      playlist.Name,
		CreatedAt: playlist.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: playlist.UpdatedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		logging.Error.Printf("Failed to encode update playlist response: %v", err)
	}
}

func (h *Handler) DeletePlaylist(w http.ResponseWriter, r *http.Request, playlistID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pid, err := strconv.ParseUint(playlistID, 10, 64)
	if err != nil {
		http.Error(w, "Invalid playlist id", http.StatusBadRequest)
		return
	}

	var playlist models.Playlist
	if err := h.db.First(&playlist, "id = ?", uint(pid)).Error; err != nil {
		http.Error(w, "Playlist not found", http.StatusNotFound)
		return
	}
	if playlist.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("playlist_id = ?", playlist.ID).Delete(&models.PlaylistItem{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&playlist).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete playlist: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		logging.Error.Printf("Failed to encode delete playlist response: %v", err)
	}
}

func (h *Handler) AddPlaylistItem(w http.ResponseWriter, r *http.Request, playlistID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pid, err := strconv.ParseUint(playlistID, 10, 64)
	if err != nil {
		http.Error(w, "Invalid playlist id", http.StatusBadRequest)
		return
	}

	var playlist models.Playlist
	if err := h.db.First(&playlist, "id = ?", uint(pid)).Error; err != nil {
		http.Error(w, "Playlist not found", http.StatusNotFound)
		return
	}
	if playlist.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var req addPlaylistItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ItemID) == "" {
		http.Error(w, "item_id is required", http.StatusBadRequest)
		return
	}

	var mediaItem models.MediaItem
	if err := h.db.Where("id = ? AND deleted_at IS NULL", req.ItemID).First(&mediaItem).Error; err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var existing models.PlaylistItem
		if err := tx.Where("playlist_id = ? AND item_id = ?", playlist.ID, req.ItemID).First(&existing).Error; err == nil {
			return nil
		}

		var count int64
		if err := tx.Model(&models.PlaylistItem{}).Where("playlist_id = ?", playlist.ID).Count(&count).Error; err != nil {
			return err
		}
		position := int(count)
		if req.Position != nil && *req.Position >= 0 && *req.Position < int(count) {
			position = *req.Position
			if err := tx.Model(&models.PlaylistItem{}).
				Where("playlist_id = ? AND position >= ?", playlist.ID, position).
				Update("position", gorm.Expr("position + 1")).Error; err != nil {
				return err
			}
		}

		item := models.PlaylistItem{
			PlaylistID: playlist.ID,
			ItemID:     req.ItemID,
			Position:   position,
		}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add playlist item: %v", err), http.StatusInternalServerError)
		return
	}

	items, err := h.playlistItemsForResponse(playlist.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load playlist items: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"items":  items,
	}); err != nil {
		logging.Error.Printf("Failed to encode add playlist item response: %v", err)
	}
}

func (h *Handler) RemovePlaylistItem(w http.ResponseWriter, r *http.Request, playlistID, itemID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pid, err := strconv.ParseUint(playlistID, 10, 64)
	if err != nil {
		http.Error(w, "Invalid playlist id", http.StatusBadRequest)
		return
	}

	var playlist models.Playlist
	if err := h.db.First(&playlist, "id = ?", uint(pid)).Error; err != nil {
		http.Error(w, "Playlist not found", http.StatusNotFound)
		return
	}
	if playlist.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var current models.PlaylistItem
		if err := tx.Where("playlist_id = ? AND item_id = ?", playlist.ID, itemID).First(&current).Error; err != nil {
			return err
		}
		if err := tx.Delete(&current).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.PlaylistItem{}).
			Where("playlist_id = ? AND position > ?", playlist.ID, current.Position).
			Update("position", gorm.Expr("position - 1")).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to remove playlist item: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		logging.Error.Printf("Failed to encode remove playlist item response: %v", err)
	}
}

func (h *Handler) ReorderPlaylistItems(w http.ResponseWriter, r *http.Request, playlistID string) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pid, err := strconv.ParseUint(playlistID, 10, 64)
	if err != nil {
		http.Error(w, "Invalid playlist id", http.StatusBadRequest)
		return
	}

	var playlist models.Playlist
	if err := h.db.First(&playlist, "id = ?", uint(pid)).Error; err != nil {
		http.Error(w, "Playlist not found", http.StatusNotFound)
		return
	}
	if playlist.UserID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var req reorderPlaylistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if len(req.ItemIDs) == 0 {
		http.Error(w, "item_ids cannot be empty", http.StatusBadRequest)
		return
	}

	var existing []models.PlaylistItem
	if err := h.db.Where("playlist_id = ?", playlist.ID).Order("position ASC").Find(&existing).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to load current order: %v", err), http.StatusInternalServerError)
		return
	}

	if len(existing) != len(req.ItemIDs) {
		http.Error(w, "item_ids must include every playlist item exactly once", http.StatusBadRequest)
		return
	}

	existingMap := map[string]bool{}
	for _, e := range existing {
		existingMap[e.ItemID] = true
	}
	seen := map[string]bool{}
	for _, id := range req.ItemIDs {
		if !existingMap[id] || seen[id] {
			http.Error(w, "item_ids must include every playlist item exactly once", http.StatusBadRequest)
			return
		}
		seen[id] = true
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		for i, itemID := range req.ItemIDs {
			if err := tx.Model(&models.PlaylistItem{}).
				Where("playlist_id = ? AND item_id = ?", playlist.ID, itemID).
				Update("position", i).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to reorder playlist: %v", err), http.StatusInternalServerError)
		return
	}

	items, err := h.playlistItemsForResponse(playlist.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load playlist items: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"items":  items,
	}); err != nil {
		logging.Error.Printf("Failed to encode reorder playlist response: %v", err)
	}
}

func (h *Handler) playlistItemsForResponse(playlistID uint) ([]playlistMediaItemSummary, error) {
	var rows []struct {
		ItemID   string
		Position int
		Title    string
		Type     string
	}

	err := h.db.Table("playlist_items").
		Select("playlist_items.item_id, playlist_items.position, media_items.title, media_items.type").
		Joins("JOIN media_items ON media_items.id = playlist_items.item_id").
		Where("playlist_items.playlist_id = ? AND media_items.deleted_at IS NULL", playlistID).
		Order("playlist_items.position ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	items := make([]playlistMediaItemSummary, 0, len(rows))
	for _, row := range rows {
		summary := playlistMediaItemSummary{
			ItemID:   row.ItemID,
			Title:    row.Title,
			Type:     row.Type,
			Position: row.Position,
		}

		assetsMeta, _ := meta.ReadAssetsMetaByID(h.mediaRoot, row.ItemID)
		if assetsMeta != nil {
			if len(assetsMeta.Thumbnails) > 0 {
				summary.ThumbURL = "/media/" + row.ItemID + "/" + assetsMeta.Thumbnails[0].StoragePath
			}
			for _, asset := range assetsMeta.Assets {
				if asset.Kind == "master_mp4" || asset.Kind == "master_m4a" {
					summary.MasterURL = "/media/" + row.ItemID + "/" + asset.StoragePath
					break
				}
			}
		}

		items = append(items, summary)
	}
	return items, nil
}
