package meta

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	SchemaVersion       = 1
	CurrentItemSchema   = 1
	CurrentAssetsSchema = 1
	ItemMetaFile        = "item.json"
	AssetsMetaFile      = "assets.json"
)

var (
	ErrInvalidSchema = errors.New("invalid or missing schema version")
)

const DBSchemaVersionKey = "db_schema_version"

func ValidateSchema(schema int) error {
	if schema != SchemaVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrInvalidSchema, schema, SchemaVersion)
	}
	return nil
}

func ValidateItemSchema(schema int) error {
	if schema != CurrentItemSchema {
		return fmt.Errorf("%w: got %d, want %d", ErrInvalidSchema, schema, CurrentItemSchema)
	}
	return nil
}

func ValidateAssetsSchema(schema int) error {
	if schema != CurrentAssetsSchema {
		return fmt.Errorf("%w: got %d, want %d", ErrInvalidSchema, schema, CurrentAssetsSchema)
	}
	return nil
}

type ItemMeta struct {
	Schema             int          `json:"schema"`
	ItemID             string       `json:"item_id"`
	Type               string       `json:"type"` // "video" or "photo"
	OwnerUsername      string       `json:"owner_username"`
	PersonaID          *string      `json:"persona_id,omitempty"`
	PersonaDisplayName *string      `json:"persona_display_name,omitempty"`
	Title              string       `json:"title"`
	Description        string       `json:"description,omitempty"`
	CreatedAt          string       `json:"created_at"`
	State              ItemState    `json:"state"`
	Original           OriginalFile `json:"original"`
}

type ItemState struct {
	DeletedAt   *string `json:"deleted_at,omitempty"`
	Highlighted bool    `json:"highlighted"`
}

type OriginalFile struct {
	Filename string `json:"filename"`
	Path     string `json:"path"` // relative, e.g. "original/upload.mov"
}

type AssetsMeta struct {
	Schema     int         `json:"schema"`
	Assets     []Asset     `json:"assets"`
	Thumbnails []Thumbnail `json:"thumbnails"`
	Photos     []Photo     `json:"photos"`
	SourceInfo *SourceInfo `json:"source_info,omitempty"`
}

type SourceInfo struct {
	DurationMs      int64  `json:"duration_ms"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	Rotation        int    `json:"rotation"`
	VideoCodec      string `json:"video_codec"`
	AudioCodec      string `json:"audio_codec"`
	Bitrate         string `json:"bitrate"`
	AudioSampleRate int    `json:"audio_sample_rate"`
	AudioChannels   int    `json:"audio_channels"`
}

type Asset struct {
	Kind        string       `json:"kind"` // master_mp4, short_clip, preview_mp4, hls
	StoragePath string       `json:"storage_path"`
	Width       int          `json:"width,omitempty"`
	Height      int          `json:"height,omitempty"`
	Bitrate     string       `json:"bitrate,omitempty"`
	Codecs      string       `json:"codecs,omitempty"`
	ClipID      string       `json:"clip_id,omitempty"`
	StartMs     int64        `json:"start_ms,omitempty"`
	EndMs       int64        `json:"end_ms,omitempty"`
	CropMode    string       `json:"crop_mode,omitempty"`
	Variants    []HLSVariant `json:"variants,omitempty"`
}

type Thumbnail struct {
	StoragePath string `json:"storage_path"`
	Timestamp   string `json:"timestamp,omitempty"` // percentage or timestamp
}

type Photo struct {
	Kind        string `json:"kind"` // original, display, thumb
	StoragePath string `json:"storage_path"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

type HLSVariant struct {
	Height    int `json:"height"`
	Bandwidth int `json:"bandwidth"`
}

func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmpFile := filepath.Join(dir, ".tmp-"+filepath.Base(path))

	f, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	_, err = f.Write(data)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("write temp file: %w", err)
	}

	err = f.Sync()
	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("sync temp file: %w", err)
	}

	err = f.Close()
	if err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("close temp file: %w", err)
	}

	err = os.Rename(tmpFile, path)
	if err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

func WriteJSONAtomic(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}

	err = WriteFileAtomic(path, data)
	if err != nil {
		return fmt.Errorf("write atomic: %w", err)
	}

	return nil
}

func ReadItemMeta(path string) (*ItemMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var meta ItemMeta
	err = json.Unmarshal(data, &meta)
	if err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}

	if meta.Schema != SchemaVersion {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrInvalidSchema, meta.Schema, SchemaVersion)
	}

	return &meta, nil
}

func ReadAssetsMeta(path string) (*AssetsMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read file: %w", err)
	}

	var meta AssetsMeta
	err = json.Unmarshal(data, &meta)
	if err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}

	if meta.Schema != SchemaVersion {
		return nil, fmt.Errorf("%w: got %d, want %d", ErrInvalidSchema, meta.Schema, SchemaVersion)
	}

	return &meta, nil
}

func WriteItemMetaAtomic(mediaRoot, itemID string, meta *ItemMeta) error {
	metaPath := filepath.Join(mediaRoot, "items", itemID, "meta", ItemMetaFile)
	return WriteJSONAtomic(metaPath, meta)
}

func WriteAssetsMetaAtomic(mediaRoot, itemID string, meta *AssetsMeta) error {
	metaPath := filepath.Join(mediaRoot, "items", itemID, "meta", AssetsMetaFile)
	return WriteJSONAtomic(metaPath, meta)
}

func ReadItemMetaByID(mediaRoot, itemID string) (*ItemMeta, error) {
	metaPath := filepath.Join(mediaRoot, "items", itemID, "meta", ItemMetaFile)
	return ReadItemMeta(metaPath)
}

func ReadAssetsMetaByID(mediaRoot, itemID string) (*AssetsMeta, error) {
	metaPath := filepath.Join(mediaRoot, "items", itemID, "meta", AssetsMetaFile)
	return ReadAssetsMeta(metaPath)
}

func ItemMetaExists(mediaRoot, itemID string) bool {
	metaPath := filepath.Join(mediaRoot, "items", itemID, "meta", ItemMetaFile)
	_, err := os.Stat(metaPath)
	return err == nil
}

func CopyItemMeta(src *ItemMeta) *ItemMeta {
	副本 := *src
	if src.PersonaDisplayName != nil {
		name := *src.PersonaDisplayName
		副本.PersonaDisplayName = &name
	}
	return &副本
}

var _ = io.Discard
