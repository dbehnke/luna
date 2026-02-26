package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"luna/internal/config"
	"luna/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	*gorm.DB
}

func New(cfg *config.Config) (*DB, error) {
	dbDir := filepath.Dir(cfg.SQLitePath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(cfg.SQLitePath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	return &DB{db}, nil
}

func (d *DB) AutoMigrate() error {
	hadUserIsActive := d.DB.Migrator().HasColumn(&models.User{}, "is_active")

	if err := d.DB.AutoMigrate(
		&models.User{},
		&models.Session{},
		&models.Persona{},
		&models.MediaItem{},
		&models.Job{},
		&models.ClipAsset{},
		&models.Reaction{},
		&models.Favorite{},
		&models.Playlist{},
		&models.PlaylistItem{},
		&models.SchemaVersion{},
		&models.SessionFeedState{},
	); err != nil {
		return err
	}

	if !hadUserIsActive {
		if err := d.DB.Model(&models.User{}).Where("is_active = ?", false).Update("is_active", true).Error; err != nil {
			return fmt.Errorf("backfill users.is_active: %w", err)
		}
	}

	if err := d.migratePersonaIDToText(); err != nil {
		return fmt.Errorf("migrate personas.id to text: %w", err)
	}

	return nil
}

func (d *DB) migratePersonaIDToText() error {
	if !d.DB.Migrator().HasTable(&models.Persona{}) {
		return nil
	}

	type tableInfoRow struct {
		CID          int    `gorm:"column:cid"`
		Name         string `gorm:"column:name"`
		Type         string `gorm:"column:type"`
		NotNull      int    `gorm:"column:notnull"`
		DefaultValue string `gorm:"column:dflt_value"`
		PK           int    `gorm:"column:pk"`
	}

	var info []tableInfoRow
	if err := d.DB.Raw("PRAGMA table_info(personas)").Scan(&info).Error; err != nil {
		return err
	}

	idType := ""
	for _, col := range info {
		if col.Name == "id" {
			idType = strings.ToUpper(strings.TrimSpace(col.Type))
			break
		}
	}
	if idType == "" {
		return fmt.Errorf("personas.id column not found")
	}
	if strings.Contains(idType, "TEXT") || strings.Contains(idType, "CHAR") || strings.Contains(idType, "CLOB") {
		return nil
	}

	return d.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			CREATE TABLE personas_new (
				id TEXT PRIMARY KEY,
				user_id INTEGER NOT NULL,
				display_name TEXT NOT NULL,
				slug TEXT NOT NULL,
				description TEXT,
				avatar_path TEXT,
				created_at datetime,
				updated_at datetime,
				deleted_at datetime,
				CONSTRAINT fk_users_personas FOREIGN KEY (user_id) REFERENCES users(id)
			)
		`).Error; err != nil {
			return err
		}

		if err := tx.Exec(`
			INSERT INTO personas_new (
				id, user_id, display_name, slug, description, avatar_path, created_at, updated_at, deleted_at
			)
			SELECT
				CAST(id AS TEXT), user_id, display_name, slug, description, avatar_path, created_at, updated_at, deleted_at
			FROM personas
		`).Error; err != nil {
			return err
		}

		if err := tx.Exec(`DROP TABLE personas`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`ALTER TABLE personas_new RENAME TO personas`).Error; err != nil {
			return err
		}

		indexes := []string{
			`CREATE INDEX idx_personas_deleted_at ON personas(deleted_at)`,
			`CREATE INDEX idx_personas_user_id ON personas(user_id)`,
			`CREATE UNIQUE INDEX idx_persona_user_slug ON personas(slug)`,
		}
		for _, stmt := range indexes {
			if err := tx.Exec(stmt).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (d *DB) AutoMigrateModels(models ...interface{}) error {
	return d.DB.AutoMigrate(models...)
}

func (d *DB) DropTable(models ...interface{}) error {
	return d.DB.Migrator().DropTable(models...)
}

func (d *DB) Close() error {
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (d *DB) GetSchemaVersion(key string) (int, error) {
	var sv models.SchemaVersion
	err := d.DB.Where("key = ?", key).First(&sv).Error
	if err != nil {
		return 0, err
	}
	return sv.Value, nil
}

func (d *DB) SetSchemaVersion(key string, value int) error {
	sv := models.SchemaVersion{Key: key, Value: value}
	return d.DB.Save(&sv).Error
}

func (d *DB) EnsureSchemaVersion(key string, expected int) error {
	var sv models.SchemaVersion
	err := d.DB.Where("key = ?", key).First(&sv).Error
	if err == gorm.ErrRecordNotFound {
		return d.SetSchemaVersion(key, expected)
	}
	if err != nil {
		return err
	}
	if sv.Value != expected {
		return fmt.Errorf("schema version mismatch for %s: got %d, expected %d", key, sv.Value, expected)
	}
	return nil
}
