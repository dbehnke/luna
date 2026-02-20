package jobs

import (
	"encoding/json"
	"fmt"
	"time"

	"luna/internal/models"

	"gorm.io/gorm"
)

// ProbePayload for probe jobs
type ProbePayload struct {
	ItemID string `json:"item_id"`
}

// TranscodePayload for transcode jobs
type TranscodePayload struct {
	ItemID string `json:"item_id"`
}

// ThumbsPayload for thumbnail jobs
type ThumbsPayload struct {
	ItemID string `json:"item_id"`
}

// ClipPayload for clip jobs
type ClipPayload struct {
	ItemID  string `json:"item_id"`
	ClipID  string `json:"clip_id"`
	StartMs int64  `json:"start_ms"`
	EndMs   int64  `json:"end_ms"`
}

// HLSPayload for HLS jobs
type HLSPayload struct {
	ItemID string `json:"item_id"`
}

// Backoff schedule for retries
var BackoffSchedule = []time.Duration{
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
}

const LockTimeout = 15 * time.Minute

// Queue for job queue operations
type Queue struct {
	db *gorm.DB
}

// New creates a new job queue
func New(db *gorm.DB) *Queue {
	return &Queue{db: db}
}

// Enqueue creates a new job
func (q *Queue) Enqueue(jobType, status string, priority int, payload interface{}) (*models.Job, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	job := &models.Job{
		Type:        jobType,
		Status:      status,
		Priority:    priority,
		PayloadJSON: string(payloadJSON),
		Attempts:    0,
		MaxAttempts: 3,
		RunAfter:    time.Now(),
	}

	if err := q.db.Create(job).Error; err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}

	return job, nil
}

// EnqueueVideoProcessing enqueues all jobs for video processing
func (q *Queue) EnqueueVideoProcessing(itemID string) error {
	// Probe - high priority, runs first
	_, err := q.Enqueue(models.JobTypeProbe, models.JobStatusQueued, 100, ProbePayload{ItemID: itemID})
	if err != nil {
		return fmt.Errorf("enqueue probe: %w", err)
	}

	// Transcode - normal priority
	_, err = q.Enqueue(models.JobTypeTranscode, models.JobStatusQueued, 50, TranscodePayload{ItemID: itemID})
	if err != nil {
		return fmt.Errorf("enqueue transcode: %w", err)
	}

	// Thumbs - normal priority
	_, err = q.Enqueue(models.JobTypeThumbs, models.JobStatusQueued, 50, ThumbsPayload{ItemID: itemID})
	if err != nil {
		return fmt.Errorf("enqueue thumbs: %w", err)
	}

	return nil
}

// EnqueueAudioProcessing enqueues jobs for audio processing (probe + transcode)
func (q *Queue) EnqueueAudioProcessing(itemID string) error {
	// Probe - high priority, runs first
	_, err := q.Enqueue(models.JobTypeProbe, models.JobStatusQueued, 100, ProbePayload{ItemID: itemID})
	if err != nil {
		return fmt.Errorf("enqueue probe: %w", err)
	}

	// Transcode - normal priority
	_, err = q.Enqueue(models.JobTypeTranscode, models.JobStatusQueued, 50, TranscodePayload{ItemID: itemID})
	if err != nil {
		return fmt.Errorf("enqueue transcode: %w", err)
	}

	return nil
}

// Claim selects and locks a job for processing
func (q *Queue) Claim(workerID string, maxConcurrency int, jobTypes []string) (*models.Job, error) {
	var job models.Job

	// Start transaction for atomic claim
	err := q.db.Transaction(func(tx *gorm.DB) error {
		// First, recover any stale locks
		staleThreshold := time.Now().Add(-LockTimeout)
		if err := tx.Model(&models.Job{}).
			Where("status = ? AND locked_at < ?", models.JobStatusRunning, staleThreshold).
			Updates(map[string]interface{}{
				"status":    models.JobStatusQueued,
				"locked_at": nil,
				"locked_by": nil,
			}).Error; err != nil {
			return err
		}

		// For transcode jobs, check concurrency limit
		if len(jobTypes) > 0 && containsString(jobTypes, models.JobTypeTranscode) {
			var runningCount int64
			if err := tx.Model(&models.Job{}).
				Where("type = ? AND status = ? AND locked_by = ?", models.JobTypeTranscode, models.JobStatusRunning, workerID).
				Count(&runningCount).Error; err != nil {
				return err
			}
			if runningCount >= int64(maxConcurrency) {
				return gorm.ErrRecordNotFound
			}
		}

		// Select job to claim
		query := tx.Where("status = ? AND run_after <= ? AND locked_at IS NULL", models.JobStatusQueued, time.Now())
		if len(jobTypes) > 0 {
			query = query.Where("type IN ?", jobTypes)
		}
		query = query.Order("priority DESC, created_at ASC").Limit(1)

		if err := query.First(&job).Error; err != nil {
			return err
		}

		// Lock the job
		now := time.Now()
		updates := map[string]interface{}{
			"status":    models.JobStatusRunning,
			"locked_at": now,
			"locked_by": workerID,
		}

		if err := tx.Model(&job).Updates(updates).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &job, nil
}

// Complete marks a job as done
func (q *Queue) Complete(jobID uint) error {
	return q.db.Model(&models.Job{}).
		Where("id = ?", jobID).
		Updates(map[string]interface{}{
			"status":    models.JobStatusDone,
			"locked_at": nil,
			"locked_by": nil,
		}).Error
}

// Fail marks a job as failed or schedules retry
func (q *Queue) Fail(jobID uint, errorMessage string) error {
	var job models.Job
	if err := q.db.First(&job, jobID).Error; err != nil {
		return err
	}

	job.Attempts++
	job.ErrorMessage = errorMessage

	if job.Attempts >= job.MaxAttempts {
		job.Status = models.JobStatusFailed
		job.LockedAt = nil
		job.LockedBy = ""
	} else {
		backoff := BackoffSchedule[0]
		if job.Attempts-1 < len(BackoffSchedule) {
			backoff = BackoffSchedule[job.Attempts-1]
		}
		job.Status = models.JobStatusQueued
		job.RunAfter = time.Now().Add(backoff)
		job.LockedAt = nil
		job.LockedBy = ""
	}

	return q.db.Save(&job).Error
}

// GetJobPayload parses the payload JSON into the appropriate struct
func GetJobPayload(job *models.Job) (interface{}, error) {
	switch job.Type {
	case models.JobTypeProbe:
		var payload ProbePayload
		if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
			return nil, err
		}
		return payload, nil
	case models.JobTypeTranscode:
		var payload TranscodePayload
		if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
			return nil, err
		}
		return payload, nil
	case models.JobTypeThumbs:
		var payload ThumbsPayload
		if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
			return nil, err
		}
		return payload, nil
	case models.JobTypeClip:
		var payload ClipPayload
		if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
			return nil, err
		}
		return payload, nil
	case models.JobTypeHLS:
		var payload HLSPayload
		if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
			return nil, err
		}
		return payload, nil
	default:
		return nil, fmt.Errorf("unknown job type: %s", job.Type)
	}
}

// GetProcessingStatus returns the processing status for a media item
func (q *Queue) GetProcessingStatus(itemID string) (string, error) {
	var jobs []models.Job
	if err := q.db.Where("payload_json LIKE ? AND status IN ?", "%"+itemID+"%", []string{models.JobStatusQueued, models.JobStatusRunning}).
		Order("created_at ASC").
		Find(&jobs).Error; err != nil {
		return "", err
	}

	if len(jobs) == 0 {
		// Check if there's a failed job
		var failedJob models.Job
		if err := q.db.Where("payload_json LIKE ? AND status = ?", "%"+itemID+"%", models.JobStatusFailed).
			First(&failedJob).Error; err == nil {
			return "failed", nil
		}
		return "ready", nil
	}

	// Check if probe is done (first job should be done or not exist)
	var probeDone bool
	var transcodeDone bool

	var probeJob models.Job
	if err := q.db.Where("type = ? AND payload_json LIKE ?", models.JobTypeProbe, "%"+itemID+"%").
		First(&probeJob).Error; err == nil {
		probeDone = probeJob.Status == models.JobStatusDone
	}

	var transcodeJob models.Job
	if err := q.db.Where("type = ? AND payload_json LIKE ?", models.JobTypeTranscode, "%"+itemID+"%").
		First(&transcodeJob).Error; err == nil {
		transcodeDone = transcodeJob.Status == models.JobStatusDone
	}

	if !probeDone {
		return "pending", nil
	}
	if !transcodeDone {
		return "processing", nil
	}
	return "ready", nil
}

// GetJobError returns the last error for a failed job
func (q *Queue) GetJobError(itemID string) (string, error) {
	var job models.Job
	if err := q.db.Where("payload_json LIKE ? AND status = ?", "%"+itemID+"%", models.JobStatusFailed).
		Order("updated_at DESC").
		First(&job).Error; err != nil {
		return "", err
	}
	return job.ErrorMessage, nil
}

// GetClipJobStatus returns the status of a clip job
func (q *Queue) GetClipJobStatus(clipID string) (string, error) {
	var job models.Job
	if err := q.db.Where("type = ? AND payload_json LIKE ?", models.JobTypeClip, "%"+clipID+"%").
		Order("created_at DESC").
		First(&job).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	return job.Status, nil
}

// UpdateClipStatus updates the status of a clip asset
func (q *Queue) UpdateClipStatus(clipID, status string) error {
	return q.db.Model(&models.ClipAsset{}).
		Where("id = ?", clipID).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
