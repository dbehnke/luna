package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrPathTraversal = errors.New("path traversal attempt detected")
var ErrInvalidPath = errors.New("invalid path: absolute paths not allowed")
var ErrEscapesItemDir = errors.New("path escapes item directory")

func EnsureRootLayout(mediaRoot string) error {
	dirs := []string{
		filepath.Join(mediaRoot, "items"),
		filepath.Join(mediaRoot, "avatars"),
		filepath.Join(mediaRoot, "tmp", "uploads"),
		filepath.Join(mediaRoot, "tmp", "jobs"),
		filepath.Join(mediaRoot, "db"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

func EnsureItemDirs(mediaRoot, itemID string) error {
	dirs := []string{
		ItemDir(mediaRoot, itemID, "original"),
		ItemDir(mediaRoot, itemID, "derived"),
		ItemDir(mediaRoot, itemID, "thumbs"),
		ItemDir(mediaRoot, itemID, "photos"),
		ItemDir(mediaRoot, itemID, "meta"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

func ItemDir(mediaRoot, itemID, subdir string) string {
	return filepath.Join(mediaRoot, "items", itemID, subdir)
}

func MetaPath(mediaRoot, itemID, filename string) string {
	return filepath.Join(mediaRoot, "items", itemID, "meta", filename)
}

func SafeJoinItem(root, itemID, relPath string) (string, error) {
	if filepath.IsAbs(relPath) {
		return "", ErrInvalidPath
	}

	if relPath == ".." || containsDotDot(relPath) {
		return "", ErrPathTraversal
	}

	itemDir := filepath.Join(root, "items", itemID)
	cleaned := filepath.Clean(filepath.Join(itemDir, relPath))

	if !filepath.HasPrefix(cleaned, itemDir+string(filepath.Separator)) && cleaned != itemDir {
		return "", ErrEscapesItemDir
	}

	return cleaned, nil
}

func containsDotDot(s string) bool {
	for i := 0; i < len(s); i++ {
		if i+1 < len(s) && s[i] == '.' && s[i+1] == '.' {
			return true
		}
	}
	return false
}

func ItemsDir(mediaRoot string) string {
	return filepath.Join(mediaRoot, "items")
}

func TmpUploadsDir(mediaRoot string) string {
	return filepath.Join(mediaRoot, "tmp", "uploads")
}

func TmpJobsDir(mediaRoot string) string {
	return filepath.Join(mediaRoot, "tmp", "jobs")
}

// AvatarDir returns the directory path for a persona's avatar
func AvatarDir(mediaRoot string, userID uint, personaID string) string {
	return filepath.Join(mediaRoot, "avatars", "users", fmt.Sprintf("%d", userID), "personas", personaID)
}

// AvatarPath returns the absolute path for a persona's avatar
func AvatarPath(mediaRoot string, userID uint, personaID string) string {
	return filepath.Join(AvatarDir(mediaRoot, userID, personaID), "avatar.webp")
}

// AvatarRelativePath returns the relative path for serving via /media/avatars/
func AvatarRelativePath(userID uint, personaID string) string {
	return fmt.Sprintf("avatars/users/%d/personas/%s/avatar.webp", userID, personaID)
}

// EnsureAvatarDir creates the directory structure for a persona's avatar
func EnsureAvatarDir(mediaRoot string, userID uint, personaID string) error {
	dir := AvatarDir(mediaRoot, userID, personaID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return nil
}

// SafeJoinAvatar provides path traversal protection for avatar operations
func SafeJoinAvatar(mediaRoot string, userID uint, personaID string, filename string) (string, error) {
	if filepath.IsAbs(filename) {
		return "", ErrInvalidPath
	}

	if filename == ".." || containsDotDot(filename) {
		return "", ErrPathTraversal
	}

	avatarDir := AvatarDir(mediaRoot, userID, personaID)
	cleaned := filepath.Clean(filepath.Join(avatarDir, filename))

	if !filepath.HasPrefix(cleaned, avatarDir+string(filepath.Separator)) && cleaned != avatarDir {
		return "", ErrEscapesItemDir
	}

	return cleaned, nil
}
