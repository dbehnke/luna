package models

import (
	"time"

	"gorm.io/gorm"
)

// Role constants
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// CurrentDBSchemaVersion is the current database schema version
const CurrentDBSchemaVersion = 1

// MediaType constants
const (
	MediaTypeVideo = "video"
	MediaTypePhoto = "photo"
	MediaTypeAudio = "audio"
)

// JobType constants
const (
	JobTypeProbe      = "probe"
	JobTypeTranscode  = "transcode"
	JobTypeThumbs     = "thumbs"
	JobTypeClip       = "clip"
	JobTypePhotoThumb = "photo_thumb"
	JobTypeCleanup    = "cleanup"
	JobTypeWriteMeta  = "write_meta"
	JobTypeHLS        = "hls"
)

// HLSStatus constants
const (
	HLSStatusPending = "pending"
	HLSStatusReady   = "ready"
	HLSStatusFailed  = "failed"
)

// JobStatus constants
const (
	JobStatusQueued  = "queued"
	JobStatusRunning = "running"
	JobStatusFailed  = "failed"
	JobStatusDone    = "done"
)

// ClipStatus constants
const (
	ClipStatusPending = "pending"
	ClipStatusReady   = "ready"
	ClipStatusFailed  = "failed"
)

// User represents a user account
type User struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	Username      string         `gorm:"uniqueIndex;size:255;not null" json:"username"`
	PasswordHash  string         `gorm:"size:255;not null" json:"-"`
	Role          string         `gorm:"size:50;default:user" json:"role"`
	IsActive      bool           `gorm:"not null;default:true;index" json:"is_active"`
	DeactivatedAt *time.Time     `json:"deactivated_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	Personas      []Persona      `gorm:"foreignKey:UserID" json:"personas,omitempty"`
}

// Session represents an authenticated browser session.
type Session struct {
	ID        string    `gorm:"primaryKey;size:128" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	ExpiresAt time.Time `gorm:"index;not null" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	User      User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// Persona represents a character that a user can upload as
type Persona struct {
	ID          string         `gorm:"primaryKey;size:26" json:"id"`
	UserID      uint           `gorm:"index;not null" json:"user_id"`
	DisplayName string         `gorm:"size:255;not null" json:"display_name"`
	Slug        string         `gorm:"size:255;not null;uniqueIndex:idx_persona_user_slug" json:"slug"`
	Description string         `gorm:"type:text" json:"description"`
	AvatarPath  string         `gorm:"size:512" json:"avatar_path"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	User        User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// MediaItem represents a video or photo
type MediaItem struct {
	ID             string         `gorm:"primaryKey;size:26" json:"id"` // ULID string
	UserID         uint           `gorm:"index;not null" json:"user_id"`
	PersonaID      *string        `gorm:"index" json:"persona_id"`
	Type           string         `gorm:"size:50;not null;index" json:"type"` // video or photo
	Title          string         `gorm:"size:500;not null" json:"title"`
	Description    string         `gorm:"type:text" json:"description"`
	IsHighlighted  bool           `gorm:"default:false;index" json:"is_highlighted"`
	HLSStatus      string         `gorm:"size:50;default:pending" json:"hls_status"`
	HLSDerivedPath string         `gorm:"size:512" json:"hls_derived_path"`
	CreatedAt      time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	User           User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Persona        *Persona       `gorm:"foreignKey:PersonaID" json:"persona,omitempty"`
}

// Job represents a background processing job
type Job struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Type         string     `gorm:"size:50;not null;index" json:"type"`   // probe, transcode, thumbs, clip, etc.
	Status       string     `gorm:"size:50;not null;index" json:"status"` // queued, running, failed, done
	Priority     int        `gorm:"default:0;index" json:"priority"`
	PayloadJSON  string     `gorm:"type:text" json:"payload_json"`
	Attempts     int        `gorm:"default:0" json:"attempts"`
	MaxAttempts  int        `gorm:"default:3" json:"max_attempts"`
	LockedAt     *time.Time `json:"locked_at"`
	LockedBy     string     `gorm:"size:255" json:"locked_by"`
	RunAfter     time.Time  `gorm:"index" json:"run_after"`
	ErrorMessage string     `gorm:"type:text" json:"error_message"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// ClipAsset represents a short clip derived from a video
type ClipAsset struct {
	ID          string    `gorm:"primaryKey;size:26" json:"id"` // ULID string
	ItemID      string    `gorm:"index;not null" json:"item_id"`
	PersonaID   *string   `gorm:"index" json:"persona_id,omitempty"`
	StoragePath string    `gorm:"size:512" json:"storage_path"`
	Description string    `gorm:"type:text" json:"description"`
	Status      string    `gorm:"size:50;default:pending" json:"status"`
	StartMs     int64     `json:"start_ms"`
	EndMs       int64     `json:"end_ms"`
	DurationMs  int64     `json:"duration_ms"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	CropMode    string    `gorm:"size:50" json:"crop_mode"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Reaction represents a user's reaction to a media item
type Reaction struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	ItemID    string    `gorm:"index;not null" json:"item_id"`
	Value     int       `gorm:"not null" json:"value"` // -1 (down), 0 (none), 1 (up)
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Favorite struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"uniqueIndex:idx_favorite_user_item;not null" json:"user_id"`
	ItemID    string    `gorm:"uniqueIndex:idx_favorite_user_item;index;not null" json:"item_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Playlist represents a user-owned ordered media collection.
type Playlist struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	UserID    uint           `gorm:"index;not null" json:"user_id"`
	Name      string         `gorm:"size:255;not null" json:"name"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// PlaylistItem represents one media item inside a playlist at a stable position.
type PlaylistItem struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	PlaylistID uint      `gorm:"uniqueIndex:idx_playlist_item;index;not null" json:"playlist_id"`
	ItemID     string    `gorm:"uniqueIndex:idx_playlist_item;index;not null;size:26" json:"item_id"`
	Position   int       `gorm:"index;not null" json:"position"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SchemaVersion stores the current schema version for migrations
type SchemaVersion struct {
	Key   string `gorm:"primaryKey;size:100" json:"key"`
	Value int    `gorm:"not null" json:"value"`
}
