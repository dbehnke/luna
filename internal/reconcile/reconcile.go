package reconcile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"luna/internal/id"
	"luna/internal/jobs"
	"luna/internal/meta"
	"luna/internal/models"
	"luna/internal/storage"

	"gorm.io/gorm"
)

const JobLockTimeout = 15 * time.Minute

type ReconcileReport struct {
	Timestamp      time.Time `json:"timestamp"`
	MediaRoot      string    `json:"media_root"`
	TotalDBItems   int       `json:"total_db_items"`
	TotalMetaFiles int       `json:"total_meta_files"`

	DBToFS DBToFSReport `json:"db_to_fs"`
	FSToDB FSToDBReport `json:"fs_to_db"`
	Jobs   JobsReport   `json:"jobs"`

	MissingMetaCount    int            `json:"missing_meta_count"`
	MissingFilesCount   int            `json:"missing_files_count"`
	MissingFilesByType  map[string]int `json:"missing_files_by_type"`
	OrphanMetaCount     int            `json:"orphan_meta_count"`
	OrphanFilesCount    int            `json:"orphan_files_count"`
	SchemaMismatchCount int            `json:"schema_mismatch_count"`
	StuckJobsCount      int            `json:"stuck_jobs_count"`

	Examples ReconcileExamples `json:"examples"`
}

type DBToFSReport struct {
	Total              int `json:"total"`
	ItemDirMissing     int `json:"item_dir_missing"`
	ItemMetaMissing    int `json:"item_meta_missing"`
	SchemaMismatch     int `json:"schema_mismatch"`
	MasterMissing      int `json:"master_missing"`
	ThumbsMissing      int `json:"thumbs_missing"`
	ClipsMissing       int `json:"clips_missing"`
	HLSDirMissing      int `json:"hls_dir_missing"`
	AudioMasterMissing int `json:"audio_master_missing"`
}

type FSToDBReport struct {
	Total          int `json:"total"`
	OrphanDirs     int `json:"orphan_dirs"`
	OrphanMeta     int `json:"orphan_meta"`
	UserMissing    int `json:"user_missing"`
	PersonaMissing int `json:"persona_missing"`
}

type JobsReport struct {
	Total        int        `json:"total"`
	Queued       int        `json:"queued"`
	Running      int        `json:"running"`
	Failed       int        `json:"failed"`
	Done         int        `json:"done"`
	StuckRunning int        `json:"stuck_running"`
	OldestQueued *time.Time `json:"oldest_queued,omitempty"`
}

type ReconcileExamples struct {
	MissingMeta    []string `json:"missing_meta"`
	MissingMaster  []string `json:"missing_master"`
	OrphanMeta     []string `json:"orphan_meta"`
	SchemaMismatch []string `json:"schema_mismatch"`
	StuckJobs      []string `json:"stuck_jobs"`
}

type FixResult struct {
	MetaRegenerated int      `json:"meta_regenerated"`
	DBRowsRecreated int      `json:"db_rows_recreated"`
	JobsEnqueued    int      `json:"jobs_enqueued"`
	JobsRequeued    int      `json:"jobs_requeued"`
	Errors          []string `json:"errors"`
}

func Reconcile(db *gorm.DB, mediaRoot string, limit int) (*ReconcileReport, error) {
	report := &ReconcileReport{
		Timestamp:          time.Now(),
		MediaRoot:          mediaRoot,
		MissingFilesByType: make(map[string]int),
		Examples:           ReconcileExamples{},
	}

	itemsDir := storage.ItemsDir(mediaRoot)

	dbToFS, examples, err := reconcileDBToFS(db, mediaRoot, itemsDir, limit)
	if err != nil {
		return nil, fmt.Errorf("db to fs reconciliation: %w", err)
	}
	report.DBToFS = *dbToFS
	report.Examples.MissingMeta = examples.MissingMeta
	report.Examples.MissingMaster = examples.MissingMaster
	report.Examples.SchemaMismatch = examples.SchemaMismatch

	fsToDB, fsExamples, err := reconcileFSToDB(db, mediaRoot, itemsDir, limit)
	if err != nil {
		return nil, fmt.Errorf("fs to db reconciliation: %w", err)
	}
	report.FSToDB = *fsToDB
	report.Examples.OrphanMeta = fsExamples.OrphanMeta

	jobsReport, jobsExamples, err := reconcileJobs(db)
	if err != nil {
		return nil, fmt.Errorf("jobs reconciliation: %w", err)
	}
	report.Jobs = *jobsReport
	report.Examples.StuckJobs = jobsExamples.StuckJobs

	var totalDBItems int64
	db.Model(&models.MediaItem{}).Count(&totalDBItems)
	report.TotalDBItems = int(totalDBItems)

	entries, _ := os.ReadDir(itemsDir)
	report.TotalMetaFiles = countMetaFiles(entries, itemsDir)

	report.MissingMetaCount = dbToFS.ItemMetaMissing
	report.MissingFilesCount = dbToFS.MasterMissing + dbToFS.ThumbsMissing + dbToFS.ClipsMissing + dbToFS.HLSDirMissing + dbToFS.AudioMasterMissing
	report.MissingFilesByType["master"] = dbToFS.MasterMissing
	report.MissingFilesByType["thumbs"] = dbToFS.ThumbsMissing
	report.MissingFilesByType["clips"] = dbToFS.ClipsMissing
	report.MissingFilesByType["hls"] = dbToFS.HLSDirMissing
	report.MissingFilesByType["audio_master"] = dbToFS.AudioMasterMissing
	report.OrphanMetaCount = fsToDB.OrphanMeta
	report.SchemaMismatchCount = dbToFS.SchemaMismatch
	report.StuckJobsCount = jobsReport.StuckRunning

	return report, nil
}

type dbToFSExamples struct {
	MissingMeta    []string
	MissingMaster  []string
	SchemaMismatch []string
}

type fsToDBExamples struct {
	OrphanMeta []string
}

type jobsExamples struct {
	StuckJobs []string
}

func reconcileDBToFS(db *gorm.DB, mediaRoot, itemsDir string, limit int) (*DBToFSReport, *dbToFSExamples, error) {
	report := &DBToFSReport{}
	examples := &dbToFSExamples{}

	var items []models.MediaItem
	if err := db.Find(&items).Error; err != nil {
		return nil, nil, err
	}
	report.Total = len(items)

	maxExamples := limit
	if maxExamples <= 0 {
		maxExamples = 5
	}

	for _, item := range items {
		itemDir := filepath.Join(itemsDir, item.ID)

		if _, err := os.Stat(itemDir); os.IsNotExist(err) {
			continue
		}

		itemMetaPath := filepath.Join(itemDir, "meta", meta.ItemMetaFile)
		if _, err := os.Stat(itemMetaPath); os.IsNotExist(err) {
			report.ItemMetaMissing++
			if len(examples.MissingMeta) < maxExamples {
				examples.MissingMeta = append(examples.MissingMeta, item.ID)
			}
			continue
		}

		itemMeta, err := meta.ReadItemMeta(itemMetaPath)
		if err != nil {
			report.SchemaMismatch++
			if len(examples.SchemaMismatch) < maxExamples {
				examples.SchemaMismatch = append(examples.SchemaMismatch, fmt.Sprintf("%s: %v", item.ID, err))
			}
			continue
		}

		if itemMeta.Schema != meta.CurrentItemSchema {
			report.SchemaMismatch++
			if len(examples.SchemaMismatch) < maxExamples {
				examples.SchemaMismatch = append(examples.SchemaMismatch, fmt.Sprintf("%s: got %d, want %d", item.ID, itemMeta.Schema, meta.CurrentItemSchema))
			}
		}

		if item.Type == models.MediaTypeVideo {
			masterPath := filepath.Join(itemDir, "derived", "master.mp4")
			if _, err := os.Stat(masterPath); os.IsNotExist(err) {
				report.MasterMissing++
				if len(examples.MissingMaster) < maxExamples {
					examples.MissingMaster = append(examples.MissingMaster, item.ID)
				}
			}

			assetsMeta, err := meta.ReadAssetsMetaByID(mediaRoot, item.ID)
			if err == nil && assetsMeta != nil {
				if len(assetsMeta.Thumbnails) == 0 {
					report.ThumbsMissing++
				}
			} else {
				report.ThumbsMissing++
			}
		}

		if item.Type == models.MediaTypeAudio {
			masterPath := filepath.Join(itemDir, "derived", "master.m4a")
			if _, err := os.Stat(masterPath); os.IsNotExist(err) {
				report.AudioMasterMissing++
			}
		}

		if item.Type == models.MediaTypePhoto {
			photosDir := filepath.Join(itemDir, "photos")
			if _, err := os.Stat(photosDir); os.IsNotExist(err) {
				report.ThumbsMissing++
			}
		}

		if item.HLSStatus == models.HLSStatusReady {
			hlsDir := filepath.Join(itemDir, "derived", "hls")
			if _, err := os.Stat(hlsDir); os.IsNotExist(err) {
				report.HLSDirMissing++
			}
		}
	}

	return report, examples, nil
}

func reconcileFSToDB(db *gorm.DB, mediaRoot, itemsDir string, limit int) (*FSToDBReport, *fsToDBExamples, error) {
	report := &FSToDBReport{}
	examples := &fsToDBExamples{}

	entries, err := os.ReadDir(itemsDir)
	if err != nil {
		return nil, nil, err
	}

	maxExamples := limit
	if maxExamples <= 0 {
		maxExamples = 5
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		itemID := entry.Name()
		itemDir := filepath.Join(itemsDir, itemID)
		itemMetaPath := filepath.Join(itemDir, "meta", meta.ItemMetaFile)

		_, err := os.Stat(itemMetaPath)
		if os.IsNotExist(err) {
			continue
		}

		var count int64
		db.Model(&models.MediaItem{}).Where("id = ?", itemID).Count(&count)
		if count == 0 {
			report.OrphanDirs++
			report.OrphanMeta++

			if len(examples.OrphanMeta) < maxExamples {
				examples.OrphanMeta = append(examples.OrphanMeta, itemID)
			}
		}
		report.Total++
	}

	return report, examples, nil
}

func reconcileJobs(db *gorm.DB) (*JobsReport, *jobsExamples, error) {
	report := &JobsReport{}
	examples := &jobsExamples{}

	var allJobs []models.Job
	if err := db.Find(&allJobs).Error; err != nil {
		return nil, nil, err
	}
	report.Total = len(allJobs)

	for _, job := range allJobs {
		switch job.Status {
		case models.JobStatusQueued:
			report.Queued++
			if report.OldestQueued == nil || job.CreatedAt.Before(*report.OldestQueued) {
				report.OldestQueued = &job.CreatedAt
			}
		case models.JobStatusRunning:
			report.Running++
			if job.LockedAt != nil && time.Since(*job.LockedAt) > JobLockTimeout {
				report.StuckRunning++
				examples.StuckJobs = append(examples.StuckJobs, fmt.Sprintf("job %d (%s)", job.ID, job.Type))
			}
		case models.JobStatusFailed:
			report.Failed++
		case models.JobStatusDone:
			report.Done++
		}
	}

	return report, examples, nil
}

func countMetaFiles(entries []os.DirEntry, itemsDir string) int {
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		itemDir := filepath.Join(itemsDir, entry.Name())
		metaPath := filepath.Join(itemDir, "meta", meta.ItemMetaFile)
		if _, err := os.Stat(metaPath); err == nil {
			count++
		}
	}
	return count
}

func Fix(db *gorm.DB, mediaRoot string, report *ReconcileReport) (*FixResult, error) {
	result := &FixResult{}

	itemsDir := storage.ItemsDir(mediaRoot)
	entries, err := os.ReadDir(itemsDir)
	if err != nil {
		return nil, err
	}

	var items []models.MediaItem
	if err := db.Find(&items).Error; err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		itemID := entry.Name()
		itemDir := filepath.Join(itemsDir, itemID)
		itemMetaPath := filepath.Join(itemDir, "meta", meta.ItemMetaFile)

		var count int64
		db.Model(&models.MediaItem{}).Where("id = ?", itemID).Count(&count)

		if count == 0 {
			if _, err := os.Stat(itemMetaPath); err == nil {
				if err := recreateDBRowFromMeta(db, mediaRoot, itemID); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("failed to recreate DB row for %s: %v", itemID, err))
				} else {
					result.DBRowsRecreated++
				}
			}
		}
	}

	for _, item := range items {
		itemDir := filepath.Join(itemsDir, item.ID)
		itemMetaPath := filepath.Join(itemDir, "meta", meta.ItemMetaFile)

		if _, err := os.Stat(itemMetaPath); os.IsNotExist(err) {
			if err := regenerateMetaFromDB(db, mediaRoot, item.ID); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to regenerate meta for %s: %v", item.ID, err))
			} else {
				result.MetaRegenerated++
			}
		}

		if item.Type == models.MediaTypeVideo {
			masterPath := filepath.Join(itemDir, "derived", "master.mp4")
			if _, err := os.Stat(masterPath); os.IsNotExist(err) {
				if err := enqueueProcessingJobs(db, item.ID); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("failed to enqueue jobs for %s: %v", item.ID, err))
				} else {
					result.JobsEnqueued++
				}
			}
		}

		if item.Type == models.MediaTypeAudio {
			masterPath := filepath.Join(itemDir, "derived", "master.m4a")
			if _, err := os.Stat(masterPath); os.IsNotExist(err) {
				if err := enqueueAudioJobs(db, item.ID); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("failed to enqueue audio jobs for %s: %v", item.ID, err))
				} else {
					result.JobsEnqueued++
				}
			}
		}

		if item.Type == models.MediaTypePhoto {
			photosDir := filepath.Join(itemDir, "photos")
			if _, err := os.Stat(photosDir); os.IsNotExist(err) {
				if err := enqueuePhotoJobs(db, item.ID); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("failed to enqueue photo jobs for %s: %v", item.ID, err))
				} else {
					result.JobsEnqueued++
				}
			}
		}
	}

	if err := requeueStuckJobs(db, result); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to requeue stuck jobs: %v", err))
	}

	return result, nil
}

func recreateDBRowFromMeta(db *gorm.DB, mediaRoot, itemID string) error {
	itemMetaPath := filepath.Join(mediaRoot, "items", itemID, "meta", meta.ItemMetaFile)
	itemMeta, err := meta.ReadItemMeta(itemMetaPath)
	if err != nil {
		return fmt.Errorf("read meta: %w", err)
	}

	var user models.User
	result := db.Where("username = ?", itemMeta.OwnerUsername).First(&user)
	if result.Error != nil {
		user = models.User{
			Username: itemMeta.OwnerUsername,
			Role:     models.RoleUser,
		}
		if err := db.Create(&user).Error; err != nil {
			return fmt.Errorf("create user: %w", err)
		}
	}

	var personaID *string
	if itemMeta.PersonaDisplayName != nil && *itemMeta.PersonaDisplayName != "" {
		var persona models.Persona
		result := db.Where("user_id = ? AND display_name = ?", user.ID, *itemMeta.PersonaDisplayName).First(&persona)
		if result.Error != nil {
			persona = models.Persona{
				ID:          id.NewULID(),
				UserID:      user.ID,
				DisplayName: *itemMeta.PersonaDisplayName,
				Slug:        *itemMeta.PersonaDisplayName,
			}
			if err := db.Create(&persona).Error; err != nil {
				return fmt.Errorf("create persona: %w", err)
			}
		}
		personaID = &persona.ID
	}

	createdAt, _ := time.Parse(time.RFC3339, itemMeta.CreatedAt)
	mediaItem := models.MediaItem{
		ID:            itemMeta.ItemID,
		UserID:        user.ID,
		PersonaID:     personaID,
		Type:          itemMeta.Type,
		Title:         itemMeta.Title,
		Description:   itemMeta.Description,
		IsHighlighted: itemMeta.State.Highlighted,
		HLSStatus:     models.HLSStatusPending,
		CreatedAt:     createdAt,
		UpdatedAt:     time.Now(),
	}

	if err := db.Create(&mediaItem).Error; err != nil {
		return fmt.Errorf("create media item: %w", err)
	}

	return nil
}

func regenerateMetaFromDB(db *gorm.DB, mediaRoot, itemID string) error {
	var item models.MediaItem
	if err := db.First(&item, "id = ?", itemID).Error; err != nil {
		return fmt.Errorf("find item: %w", err)
	}

	var user models.User
	if err := db.First(&user, item.UserID).Error; err != nil {
		return fmt.Errorf("find user: %w", err)
	}

	var persona *models.Persona
	if item.PersonaID != nil {
		var p models.Persona
		if err := db.First(&p, "id = ?", *item.PersonaID).Error; err == nil {
			persona = &p
		}
	}

	itemMeta := &meta.ItemMeta{
		Schema:        meta.CurrentItemSchema,
		ItemID:        item.ID,
		Type:          item.Type,
		OwnerUsername: user.Username,
		Title:         item.Title,
		Description:   item.Description,
		CreatedAt:     item.CreatedAt.Format(time.RFC3339),
		State: meta.ItemState{
			Highlighted: item.IsHighlighted,
		},
	}

	if persona != nil {
		itemMeta.PersonaID = &persona.ID
		itemMeta.PersonaDisplayName = &persona.DisplayName
	}

	originalPath := filepath.Join(mediaRoot, "items", itemID, "original")
	entries, _ := os.ReadDir(originalPath)
	if len(entries) > 0 {
		itemMeta.Original = meta.OriginalFile{
			Filename: entries[0].Name(),
			Path:     filepath.Join("original", entries[0].Name()),
		}
	}

	return meta.WriteItemMetaAtomic(mediaRoot, itemID, itemMeta)
}

func enqueueProcessingJobs(db *gorm.DB, itemID string) error {
	jobQueue := jobs.New(db)
	return jobQueue.EnqueueVideoProcessing(itemID)
}

func enqueueAudioJobs(db *gorm.DB, itemID string) error {
	jobQueue := jobs.New(db)
	return jobQueue.EnqueueAudioProcessing(itemID)
}

func enqueuePhotoJobs(db *gorm.DB, itemID string) error {
	jobQueue := jobs.New(db)
	_, err := jobQueue.Enqueue(models.JobTypePhotoThumb, models.JobStatusQueued, 50, jobs.ThumbsPayload{ItemID: itemID})
	return err
}

func requeueStuckJobs(db *gorm.DB, result *FixResult) error {
	staleThreshold := time.Now().Add(-JobLockTimeout)
	err := db.Model(&models.Job{}).
		Where("status = ? AND locked_at < ?", models.JobStatusRunning, staleThreshold).
		Updates(map[string]interface{}{
			"status":    models.JobStatusQueued,
			"locked_at": nil,
			"locked_by": nil,
		}).Error

	if err != nil {
		return err
	}

	var count int64
	db.Model(&models.Job{}).
		Where("status = ? AND locked_at IS NULL", models.JobStatusQueued).
		Count(&count)

	if count > 0 {
		result.JobsRequeued = int(count)
	}

	return nil
}
