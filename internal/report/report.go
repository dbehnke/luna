package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type StorageReport struct {
	TotalBytes    int64             `json:"total_bytes"`
	Breakdown     CategoryBreakdown `json:"breakdown"`
	TopItems      []ItemSize        `json:"top_items"`
	Counts        TypeCounts        `json:"counts"`
	PersonaCounts map[string]int    `json:"persona_counts,omitempty"`
}

type CategoryBreakdown struct {
	Originals int64 `json:"originals"`
	Derived   int64 `json:"derived"`
	Thumbs    int64 `json:"thumbs"`
	Avatars   int64 `json:"avatars"`
	DB        int64 `json:"db"`
	Meta      int64 `json:"meta"`
	Other     int64 `json:"other"`
}

type ItemSize struct {
	ItemID   string `json:"item_id"`
	Title    string `json:"title"`
	Size     int64  `json:"size"`
	Type     string `json:"type"`
	Username string `json:"username,omitempty"`
}

type TypeCounts struct {
	Videos int `json:"videos"`
	Photos int `json:"photos"`
	Clips  int `json:"clips"`
	Total  int `json:"total"`
}

func GenerateStorageReport(mediaRoot, sqlitePath string) (*StorageReport, error) {
	report := &StorageReport{
		PersonaCounts: make(map[string]int),
	}

	itemSizes := []ItemSize{}

	itemsDir := filepath.Join(mediaRoot, "items")
	entries, err := os.ReadDir(itemsDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read items dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		itemID := entry.Name()
		itemPath := filepath.Join(itemsDir, itemID)

		itemSize, title, itemType, username, personaName := calculateItemSize(itemPath)
		report.TotalBytes += itemSize

		itemSizes = append(itemSizes, ItemSize{
			ItemID:   itemID,
			Title:    title,
			Size:     itemSize,
			Type:     itemType,
			Username: username,
		})

		if personaName != "" {
			report.PersonaCounts[personaName]++
		}

		origSize := dirSize(filepath.Join(itemPath, "original"))
		derivedSize := dirSize(filepath.Join(itemPath, "derived"))
		thumbsSize := dirSize(filepath.Join(itemPath, "thumbs"))
		metaSize := dirSize(filepath.Join(itemPath, "meta"))

		report.Breakdown.Originals += origSize
		report.Breakdown.Derived += derivedSize
		report.Breakdown.Thumbs += thumbsSize
		report.Breakdown.Meta += metaSize

		switch itemType {
		case "video":
			report.Counts.Videos++
		case "photo":
			report.Counts.Photos++
		}

		clips := countClips(filepath.Join(itemPath, "derived"))
		report.Counts.Clips += clips
	}

	report.Counts.Total = report.Counts.Videos + report.Counts.Photos

	avatarsSize := dirSize(filepath.Join(mediaRoot, "avatars"))
	report.Breakdown.Avatars = avatarsSize
	report.TotalBytes += avatarsSize

	dbSize := dirSize(filepath.Join(mediaRoot, "db"))
	report.Breakdown.DB = dbSize
	report.TotalBytes += dbSize

	sort.Slice(itemSizes, func(i, j int) bool {
		return itemSizes[i].Size > itemSizes[j].Size
	})

	if len(itemSizes) > 10 {
		itemSizes = itemSizes[:10]
	}
	report.TopItems = itemSizes

	return report, nil
}

func calculateItemSize(itemPath string) (int64, string, string, string, string) {
	var totalSize int64
	var title string
	var itemType string
	var username string
	var personaName string

	metaPath := filepath.Join(itemPath, "meta", "item.json")
	data, err := os.ReadFile(metaPath)
	if err == nil {
		var meta struct {
			Title              string  `json:"title"`
			Type               string  `json:"type"`
			OwnerUsername      string  `json:"owner_username"`
			PersonaDisplayName *string `json:"persona_display_name"`
		}
		if json.Unmarshal(data, &meta) == nil {
			title = meta.Title
			itemType = meta.Type
			username = meta.OwnerUsername
			if meta.PersonaDisplayName != nil {
				personaName = *meta.PersonaDisplayName
			}
		}
	}

	subdirs := []string{"original", "derived", "thumbs", "photos", "meta"}
	for _, subdir := range subdirs {
		totalSize += dirSize(filepath.Join(itemPath, subdir))
	}

	return totalSize, title, itemType, username, personaName
}

func dirSize(path string) int64 {
	var size int64

	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		if entry.IsDir() {
			size += dirSize(filepath.Join(path, entry.Name()))
		} else {
			info, err := entry.Info()
			if err == nil {
				size += info.Size()
			}
		}
	}

	return size
}

func countClips(derivedPath string) int {
	entries, err := os.ReadDir(derivedPath)
	if err != nil {
		return 0
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "short_") {
			count++
		}
	}

	return count
}

func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}
