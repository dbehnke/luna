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
	return d.DB.AutoMigrate(
		&models.User{},
		&models.Persona{},
		&models.MediaItem{},
		&models.Job{},
		&models.ClipAsset{},
		&models.Reaction{},
	)
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
