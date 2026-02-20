package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"luna/internal/auth"
	"luna/internal/logging"
	"luna/internal/meta"
	"luna/internal/models"

	"gorm.io/gorm"
)

// ListTrashItems returns soft-deleted items for the current user.
// Admins can pass scope=all to view all deleted items.
func (h *Handler) ListTrashItems(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	query := h.db.Unscoped().
		Preload("Persona").
		Where("deleted_at IS NOT NULL")

	if user.Role != models.RoleAdmin || scope != "all" {
		query = query.Where("user_id = ?", user.ID)
	}

	var items []models.MediaItem
	if err := query.Order("deleted_at DESC").Limit(200).Find(&items).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to list trash items: %v", err), http.StatusInternalServerError)
		return
	}

	resp := make([]MediaItemResponse, 0, len(items))
	for _, item := range items {
		out := MediaItemResponse{
			ID:            item.ID,
			UserID:        item.UserID,
			Type:          item.Type,
			Title:         item.Title,
			Description:   item.Description,
			CreatedAt:     item.CreatedAt.Format(time.RFC3339),
			IsHighlighted: item.IsHighlighted,
		}
		if item.DeletedAt.Valid {
			deletedAt := item.DeletedAt.Time.Format(time.RFC3339)
			out.DeletedAt = &deletedAt
		}
		if item.PersonaID != nil {
			out.PersonaID = item.PersonaID
			if item.Persona != nil {
				out.PersonaName = &item.Persona.DisplayName
				out.PersonaSlug = &item.Persona.Slug
				if item.Persona.AvatarPath != "" {
					url := avatarURL(item.Persona.AvatarPath)
					out.PersonaAvatarURL = &url
				}
			}
		}
		resp = append(resp, out)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"items": resp}); err != nil {
		logging.Error.Printf("Failed to encode trash response: %v", err)
	}
}

func (h *Handler) RestoreItem(w http.ResponseWriter, r *http.Request, itemID string) {
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
	if err := h.db.Unscoped().First(&item, "id = ?", itemID).Error; err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	if item.UserID != user.ID && user.Role != models.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if !item.DeletedAt.Valid {
		http.Error(w, "Item is not deleted", http.StatusConflict)
		return
	}

	if err := h.db.Unscoped().Model(&item).Update("deleted_at", nil).Error; err != nil {
		http.Error(w, fmt.Sprintf("Failed to restore item: %v", err), http.StatusInternalServerError)
		return
	}

	itemMeta, err := meta.ReadItemMetaByID(h.mediaRoot, itemID)
	if err == nil && itemMeta != nil {
		itemMeta.State.DeletedAt = nil
		if writeErr := meta.WriteItemMetaAtomic(h.mediaRoot, itemID, itemMeta); writeErr != nil {
			logging.Error.Printf("Failed to update item meta on restore for %s: %v", itemID, writeErr)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "restored"}); err != nil {
		logging.Error.Printf("Failed to encode restore response for %s: %v", itemID, err)
	}
}

func (h *Handler) PurgeItemAdmin(w http.ResponseWriter, r *http.Request, itemID string) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if itemID == "" {
		http.Error(w, "Item ID required", http.StatusBadRequest)
		return
	}

	var item models.MediaItem
	if err := h.db.Unscoped().First(&item, "id = ?", itemID).Error; err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	if !item.DeletedAt.Valid {
		http.Error(w, "Item must be soft-deleted before purge", http.StatusConflict)
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("item_id = ?", itemID).Delete(&models.Reaction{}).Error; err != nil {
			return err
		}
		if err := tx.Where("item_id = ?", itemID).Delete(&models.Favorite{}).Error; err != nil {
			return err
		}
		if err := tx.Where("item_id = ?", itemID).Delete(&models.PlaylistItem{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("item_id = ?", itemID).Delete(&models.ClipAsset{}).Error; err != nil {
			return err
		}
		if err := tx.Where("payload_json LIKE ?", "%"+itemID+"%").Delete(&models.Job{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Delete(&models.MediaItem{}, "id = ?", itemID).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to purge item: %v", err), http.StatusInternalServerError)
		return
	}

	itemDir := filepath.Join(h.mediaRoot, "items", itemID)
	if err := os.RemoveAll(itemDir); err != nil {
		http.Error(w, fmt.Sprintf("Item DB records purged, but media dir removal failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "purged"}); err != nil {
		logging.Error.Printf("Failed to encode purge response for %s: %v", itemID, err)
	}
}
