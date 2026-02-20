package db

import (
	"fmt"
	"os"
	"path/filepath"

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
	); err != nil {
		return err
	}

	if !hadUserIsActive {
		if err := d.DB.Model(&models.User{}).Where("is_active = ?", false).Update("is_active", true).Error; err != nil {
			return fmt.Errorf("backfill users.is_active: %w", err)
		}
	}

	return nil
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
