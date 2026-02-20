package jobs

import (
	"testing"
	"time"

	"luna/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestQueue(t *testing.T) (*Queue, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Job{}); err != nil {
		t.Fatalf("migrate jobs: %v", err)
	}
	return New(db), db
}

func TestClaimPrefersHigherPriorityThenOldest(t *testing.T) {
	q, db := newTestQueue(t)
	now := time.Now().Add(-1 * time.Minute)

	jobs := []models.Job{
		{
			Type:        models.JobTypeThumbs,
			Status:      models.JobStatusQueued,
			Priority:    10,
			PayloadJSON: `{"item_id":"a"}`,
			RunAfter:    now,
			CreatedAt:   now.Add(-10 * time.Second),
		},
		{
			Type:        models.JobTypeProbe,
			Status:      models.JobStatusQueued,
			Priority:    50,
			PayloadJSON: `{"item_id":"b"}`,
			RunAfter:    now,
			CreatedAt:   now.Add(-5 * time.Second),
		},
		{
			Type:        models.JobTypeClip,
			Status:      models.JobStatusQueued,
			Priority:    50,
			PayloadJSON: `{"item_id":"c"}`,
			RunAfter:    now,
			CreatedAt:   now.Add(-15 * time.Second),
		},
	}
	for _, j := range jobs {
		if err := db.Create(&j).Error; err != nil {
			t.Fatalf("create job: %v", err)
		}
	}

	claimed, err := q.Claim("worker-1", 1, []string{
		models.JobTypeProbe,
		models.JobTypeThumbs,
		models.JobTypeClip,
	})
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}

	if claimed.Type != models.JobTypeClip {
		t.Fatalf("expected clip job first by same priority + older created_at, got %s", claimed.Type)
	}
	if claimed.Status != models.JobStatusRunning {
		t.Fatalf("expected running status, got %s", claimed.Status)
	}
	if claimed.LockedBy != "worker-1" {
		t.Fatalf("expected locked_by worker-1, got %s", claimed.LockedBy)
	}
}

func TestClaimRecoversStaleRunningLock(t *testing.T) {
	q, db := newTestQueue(t)
	stale := time.Now().Add(-2 * LockTimeout)

	job := models.Job{
		Type:        models.JobTypeProbe,
		Status:      models.JobStatusRunning,
		Priority:    100,
		PayloadJSON: `{"item_id":"stale"}`,
		RunAfter:    time.Now().Add(-1 * time.Minute),
		LockedAt:    &stale,
		LockedBy:    "worker-old",
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create stale job: %v", err)
	}

	claimed, err := q.Claim("worker-new", 1, []string{models.JobTypeProbe})
	if err != nil {
		t.Fatalf("claim stale-recovered job: %v", err)
	}
	if claimed.ID != job.ID {
		t.Fatalf("expected to reclaim stale job id=%d, got id=%d", job.ID, claimed.ID)
	}
	if claimed.Status != models.JobStatusRunning {
		t.Fatalf("expected running status, got %s", claimed.Status)
	}
	if claimed.LockedBy != "worker-new" {
		t.Fatalf("expected lock transfer to worker-new, got %s", claimed.LockedBy)
	}
}

func TestClaimHonorsTranscodeConcurrencyPerWorker(t *testing.T) {
	q, db := newTestQueue(t)
	now := time.Now().Add(-1 * time.Minute)

	running := models.Job{
		Type:        models.JobTypeTranscode,
		Status:      models.JobStatusRunning,
		Priority:    50,
		PayloadJSON: `{"item_id":"x"}`,
		RunAfter:    now,
		LockedBy:    "worker-a",
	}
	if err := db.Create(&running).Error; err != nil {
		t.Fatalf("create running transcode: %v", err)
	}

	queued := models.Job{
		Type:        models.JobTypeTranscode,
		Status:      models.JobStatusQueued,
		Priority:    50,
		PayloadJSON: `{"item_id":"y"}`,
		RunAfter:    now,
	}
	if err := db.Create(&queued).Error; err != nil {
		t.Fatalf("create queued transcode: %v", err)
	}

	_, err := q.Claim("worker-a", 1, []string{models.JobTypeTranscode})
	if err == nil {
		t.Fatalf("expected claim to fail due concurrency cap")
	}
	if err != gorm.ErrRecordNotFound {
		t.Fatalf("expected gorm.ErrRecordNotFound, got %v", err)
	}
}
