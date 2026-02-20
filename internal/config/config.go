package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	AppName    = "Luna"
	AppTagline = "Moments live here"
)

func defaultMediaRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".luna"
	}
	return filepath.Join(home, ".luna")
}

func defaultSQLitePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".luna/db/app.sqlite"
	}
	return filepath.Join(home, ".luna", "db", "app.sqlite")
}

type Config struct {
	MediaRoot                 string
	SQLitePath                string
	HTTPAddr                  string
	VideoTranscodeConcurrency int
	DevPortScan               bool
}

func Load() *Config {
	return &Config{
		MediaRoot:                 getEnv("MEDIA_ROOT", defaultMediaRoot()),
		SQLitePath:                getEnv("SQLITE_PATH", defaultSQLitePath()),
		HTTPAddr:                  getEnv("HTTP_ADDR", ":8080"),
		VideoTranscodeConcurrency: getEnvInt("VIDEO_TRANSCODE_CONCURRENCY", 1),
		DevPortScan:               getEnvBool("LUNA_DEV_PORTSCAN", false),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		return value == "1" || value == "true" || value == "yes"
	}
	return defaultValue
}

func (c *Config) Validate() error {
	if c.MediaRoot == "" {
		return fmt.Errorf("MEDIA_ROOT is required")
	}
	if c.SQLitePath == "" {
		return fmt.Errorf("SQLITE_PATH is required")
	}
	if c.VideoTranscodeConcurrency < 1 {
		return fmt.Errorf("VIDEO_TRANSCODE_CONCURRENCY must be at least 1")
	}
	return nil
}
