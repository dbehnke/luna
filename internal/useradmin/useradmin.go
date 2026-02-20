package useradmin

import (
	"fmt"
	"strings"

	"luna/internal/models"

	"gorm.io/gorm"
)

func NormalizeRole(role string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(role))
	switch normalized {
	case models.RoleAdmin, models.RoleUser:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid role %q: must be %q or %q", role, models.RoleUser, models.RoleAdmin)
	}
}

func CountActiveAdmins(db *gorm.DB) (int64, error) {
	var count int64
	if err := db.Model(&models.User{}).
		Where("role = ? AND is_active = ?", models.RoleAdmin, true).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func EnsureCanDeactivate(db *gorm.DB, user models.User) error {
	if user.Role != models.RoleAdmin || !user.IsActive {
		return nil
	}

	adminCount, err := CountActiveAdmins(db)
	if err != nil {
		return fmt.Errorf("count active admins: %w", err)
	}
	if adminCount <= 1 {
		return fmt.Errorf("cannot deactivate the last active admin")
	}
	return nil
}

func EnsureCanChangeRole(db *gorm.DB, user models.User, newRole string) error {
	if user.Role != models.RoleAdmin || !user.IsActive || newRole == models.RoleAdmin {
		return nil
	}

	adminCount, err := CountActiveAdmins(db)
	if err != nil {
		return fmt.Errorf("count active admins: %w", err)
	}
	if adminCount <= 1 {
		return fmt.Errorf("cannot demote the last active admin")
	}
	return nil
}
