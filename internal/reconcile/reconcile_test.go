package reconcile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"luna/internal/meta"
	"luna/internal/models"
	"luna/internal/storage"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	tmpFile := t.TempDir() + "/test.db"
	database, err := gorm.Open(sqlite.Open(tmpFile), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}

	if err := database.AutoMigrate(
		&models.User{},
		&models.Persona{},
		&models.MediaItem{},
		&models.Job{},
		&models.ClipAsset{},
		&models.Reaction{},
		&models.SchemaVersion{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return database
}

func setupTestMediaRoot(t *testing.T) string {
	tmpDir := t.TempDir()
	if err := storage.EnsureRootLayout(tmpDir); err != nil {
		t.Fatalf("failed to create media root: %v", err)
	}
	return tmpDir
}

func TestReconcileReportsMissingMeta(t *testing.T) {
	database := setupTestDB(t)
	mediaRoot := setupTestMediaRoot(t)

	user := models.User{Username: "testuser", Role: models.RoleUser}
	database.Create(&user)

	item := models.MediaItem{
		ID:        "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		UserID:    user.ID,
		Type:      models.MediaTypeVideo,
		Title:     "Test Video",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	database.Create(&item)

	itemDir := filepath.Join(mediaRoot, "items", item.ID)
	if err := os.MkdirAll(filepath.Join(itemDir, "original"), 0755); err != nil {
		t.Fatalf("failed to create item dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(itemDir, "derived"), 0755); err != nil {
		t.Fatalf("failed to create derived dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(itemDir, "thumbs"), 0755); err != nil {
		t.Fatalf("failed to create thumbs dir: %v", err)
	}

	report, err := Reconcile(database, mediaRoot, 5)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	if report.DBToFS.ItemMetaMissing != 1 {
		t.Errorf("expected 1 missing meta, got %d", report.DBToFS.ItemMetaMissing)
	}

	if len(report.Examples.MissingMeta) != 1 || report.Examples.MissingMeta[0] != item.ID {
		t.Errorf("expected missing meta example to contain item ID")
	}
}

func TestReconcileReportsOrphanMeta(t *testing.T) {
	database := setupTestDB(t)
	mediaRoot := setupTestMediaRoot(t)

	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	itemDir := filepath.Join(mediaRoot, "items", itemID)
	if err := os.MkdirAll(filepath.Join(itemDir, "meta"), 0755); err != nil {
		t.Fatalf("failed to create meta dir: %v", err)
	}

	itemMeta := &meta.ItemMeta{
		Schema:        meta.CurrentItemSchema,
		ItemID:        itemID,
		Type:          models.MediaTypeVideo,
		OwnerUsername: "orphan",
		Title:         "Orphan Video",
		CreatedAt:     time.Now().Format(time.RFC3339),
		State:         meta.ItemState{},
	}
	if err := meta.WriteItemMetaAtomic(mediaRoot, itemID, itemMeta); err != nil {
		t.Fatalf("failed to write meta: %v", err)
	}

	report, err := Reconcile(database, mediaRoot, 5)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	if report.FSToDB.OrphanMeta != 1 {
		t.Errorf("expected 1 orphan meta, got %d", report.FSToDB.OrphanMeta)
	}

	if len(report.Examples.OrphanMeta) != 1 || report.Examples.OrphanMeta[0] != itemID {
		t.Errorf("expected orphan meta example to contain item ID")
	}
}

func TestReconcileFixCreatesMissingMeta(t *testing.T) {
	database := setupTestDB(t)
	mediaRoot := setupTestMediaRoot(t)

	user := models.User{Username: "testuser", Role: models.RoleUser}
	database.Create(&user)

	item := models.MediaItem{
		ID:        "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		UserID:    user.ID,
		Type:      models.MediaTypeVideo,
		Title:     "Test Video",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	database.Create(&item)

	itemDir := filepath.Join(mediaRoot, "items", item.ID)
	if err := os.MkdirAll(filepath.Join(itemDir, "original"), 0755); err != nil {
		t.Fatalf("failed to create item dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(itemDir, "derived"), 0755); err != nil {
		t.Fatalf("failed to create derived dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(itemDir, "meta"), 0755); err != nil {
		t.Fatalf("failed to create meta dir: %v", err)
	}

	report := &ReconcileReport{}
	fixResult, err := Fix(database, mediaRoot, report)
	if err != nil {
		t.Fatalf("fix failed: %v", err)
	}

	if fixResult.MetaRegenerated != 1 {
		t.Errorf("expected 1 meta regenerated, got %d", fixResult.MetaRegenerated)
	}

	metaPath := filepath.Join(mediaRoot, "items", item.ID, "meta", meta.ItemMetaFile)
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Errorf("expected meta file to exist after fix")
	}
}

func TestReconcileFixRecreatesMissingDBRow(t *testing.T) {
	database := setupTestDB(t)
	mediaRoot := setupTestMediaRoot(t)

	user := models.User{Username: "testuser", Role: models.RoleUser}
	database.Create(&user)

	itemID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	itemDir := filepath.Join(mediaRoot, "items", itemID)
	if err := os.MkdirAll(filepath.Join(itemDir, "meta"), 0755); err != nil {
		t.Fatalf("failed to create meta dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(itemDir, "original"), 0755); err != nil {
		t.Fatalf("failed to create original dir: %v", err)
	}

	itemMeta := &meta.ItemMeta{
		Schema:        meta.CurrentItemSchema,
		ItemID:        itemID,
		Type:          models.MediaTypeVideo,
		OwnerUsername: "testuser",
		Title:         "Test Video",
		CreatedAt:     time.Now().Format(time.RFC3339),
		State:         meta.ItemState{},
		Original: meta.OriginalFile{
			Filename: "test.mp4",
			Path:     "original/test.mp4",
		},
	}
	if err := meta.WriteItemMetaAtomic(mediaRoot, itemID, itemMeta); err != nil {
		t.Fatalf("failed to write meta: %v", err)
	}

	report := &ReconcileReport{}
	fixResult, err := Fix(database, mediaRoot, report)
	if err != nil {
		t.Fatalf("fix failed: %v", err)
	}

	if fixResult.DBRowsRecreated != 1 {
		t.Errorf("expected 1 DB row recreated, got %d", fixResult.DBRowsRecreated)
	}

	var count int64
	database.Model(&models.MediaItem{}).Where("id = ?", itemID).Count(&count)
	if count != 1 {
		t.Errorf("expected DB row to exist after fix")
	}
}

func TestSchemaMismatchReported(t *testing.T) {
	database := setupTestDB(t)
	mediaRoot := setupTestMediaRoot(t)

	user := models.User{Username: "testuser", Role: models.RoleUser}
	database.Create(&user)

	item := models.MediaItem{
		ID:        "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		UserID:    user.ID,
		Type:      models.MediaTypeVideo,
		Title:     "Test Video",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	database.Create(&item)

	itemID := item.ID
	itemDir := filepath.Join(mediaRoot, "items", itemID)
	if err := os.MkdirAll(filepath.Join(itemDir, "meta"), 0755); err != nil {
		t.Fatalf("failed to create meta dir: %v", err)
	}

	itemMeta := &meta.ItemMeta{
		Schema:        999,
		ItemID:        itemID,
		Type:          models.MediaTypeVideo,
		OwnerUsername: "testuser",
		Title:         "Test Video",
		CreatedAt:     time.Now().Format(time.RFC3339),
		State:         meta.ItemState{},
	}
	if err := meta.WriteItemMetaAtomic(mediaRoot, itemID, itemMeta); err != nil {
		t.Fatalf("failed to write meta: %v", err)
	}

	report, err := Reconcile(database, mediaRoot, 5)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	if report.DBToFS.SchemaMismatch != 1 {
		t.Errorf("expected 1 schema mismatch, got %d", report.DBToFS.SchemaMismatch)
	}
}

func TestReconcileReportsMissingMaster(t *testing.T) {
	database := setupTestDB(t)
	mediaRoot := setupTestMediaRoot(t)

	user := models.User{Username: "testuser", Role: models.RoleUser}
	database.Create(&user)

	item := models.MediaItem{
		ID:        "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		UserID:    user.ID,
		Type:      models.MediaTypeVideo,
		Title:     "Test Video",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	database.Create(&item)

	itemID := item.ID
	itemDir := filepath.Join(mediaRoot, "items", itemID)
	if err := os.MkdirAll(filepath.Join(itemDir, "meta"), 0755); err != nil {
		t.Fatalf("failed to create meta dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(itemDir, "original"), 0755); err != nil {
		t.Fatalf("failed to create original dir: %v", err)
	}

	itemMeta := &meta.ItemMeta{
		Schema:        meta.CurrentItemSchema,
		ItemID:        itemID,
		Type:          models.MediaTypeVideo,
		OwnerUsername: "testuser",
		Title:         "Test Video",
		CreatedAt:     time.Now().Format(time.RFC3339),
		State:         meta.ItemState{},
	}
	if err := meta.WriteItemMetaAtomic(mediaRoot, itemID, itemMeta); err != nil {
		t.Fatalf("failed to write meta: %v", err)
	}

	report, err := Reconcile(database, mediaRoot, 5)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	if report.DBToFS.MasterMissing != 1 {
		t.Errorf("expected 1 missing master, got %d", report.DBToFS.MasterMissing)
	}
}
