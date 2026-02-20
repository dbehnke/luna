package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"luna/internal/auth"
	"luna/internal/config"
	"luna/internal/db"
	"luna/internal/handlers"
	"luna/internal/id"
	"luna/internal/jobs"
	"luna/internal/logging"
	"luna/internal/mediahttp"
	"luna/internal/meta"
	"luna/internal/models"
	"luna/internal/reconcile"
	"luna/internal/report"
	"luna/internal/restore"
	"luna/internal/snapshot"
	"luna/internal/storage"
	"luna/internal/video"

	"github.com/urfave/cli/v3"
)

type itemIDKey string

const itemIDContextKey itemIDKey = "itemID"

func withItemID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, itemIDContextKey, id)
}

func getItemID(ctx context.Context) string {
	if id, ok := ctx.Value(itemIDContextKey).(string); ok {
		return id
	}
	return ""
}

func main() {
	logging.Init()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logging.Error.Fatalf("Invalid config: %v", err)
	}

	app := &cli.Command{
		Name:        "luna",
		Usage:       "Moments live here",
		Description: "Luna - Family media server",
		Commands: []*cli.Command{
			serveCommand(cfg),
			workerCommand(cfg),
			importCommand(cfg),
			rebuildDBCommand(cfg),
			doctorCommand(cfg),
			reconcileCommand(cfg),
			snapshotCommand(cfg),
			restoreCommand(cfg),
			reportStorageCommand(cfg),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		logging.Error.Fatalf("Error: %v", err)
	}
}

func serveCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Start the HTTP server",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			logging.Info.Println("Starting Luna...")

			database, err := db.New(cfg)
			if err != nil {
				return fmt.Errorf("connect to database: %w", err)
			}
			defer database.Close()

			if err := database.AutoMigrate(); err != nil {
				return fmt.Errorf("run migrations: %w", err)
			}

			var devUser models.User
			if err := database.First(&devUser, "id = ?", 1).Error; err != nil {
				passwordHash, hashErr := auth.HashPassword("devpass")
				if hashErr != nil {
					return fmt.Errorf("hash default password: %w", hashErr)
				}
				devUser = models.User{
					ID:           1,
					Username:     "devuser",
					PasswordHash: passwordHash,
					Role:         models.RoleAdmin,
				}
				if err := database.Create(&devUser).Error; err != nil {
					return fmt.Errorf("create default user: %w", err)
				}
				logging.Info.Println("Created default user: devuser (password: devpass)")
			} else if strings.TrimSpace(devUser.PasswordHash) == "" {
				passwordHash, hashErr := auth.HashPassword("devpass")
				if hashErr != nil {
					return fmt.Errorf("hash default password: %w", hashErr)
				}
				if err := database.Model(&devUser).Update("password_hash", passwordHash).Error; err != nil {
					return fmt.Errorf("set default password hash: %w", err)
				}
				logging.Info.Println("Updated default user password hash for devuser")
			}
			logging.Info.Println("Database initialized")

			h := handlers.New(database.DB, cfg.MediaRoot)

			frontendHandler, err := getFrontendHandler()
			if err != nil {
				logging.Info.Printf("Frontend not embedded: %v", err)
			}

			mux := http.NewServeMux()

			// Serve embedded frontend for non-API routes
			if frontendHandler != nil {
				mux.Handle("/", frontendHandler)
			}

			authMiddleware := auth.NewMiddleware(database.DB)

			mux.HandleFunc("/api/login", h.Login)
			mux.HandleFunc("/api/logout", h.Logout)

			mux.Handle("/api/", authMiddleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := r.URL.Path

				if path == "/api/items" {
					if r.Method == "POST" {
						h.CreateItem(w, r)
					} else {
						h.ListItems(w, r)
					}
					return
				}

				if path == "/api/me" && r.Method == "GET" {
					h.GetCurrentUser(w, r)
					return
				}

				if path == "/api/shorts" && r.Method == "GET" {
					h.ListShorts(w, r)
					return
				}

				if strings.HasPrefix(path, "/api/items/") {
					id := strings.TrimPrefix(path, "/api/items/")

					if r.Method == "POST" && strings.HasSuffix(id, "/upload") {
						id = strings.TrimSuffix(id, "/upload")
						h.UploadItem(w, r, id)
						return
					}

					if r.Method == "POST" && strings.HasSuffix(id, "/clip") {
						id = strings.TrimSuffix(id, "/clip")
						h.CreateClip(w, r, id)
						return
					}

					if r.Method == "POST" && strings.HasSuffix(id, "/reaction") {
						id = strings.TrimSuffix(id, "/reaction")
						h.SetReaction(w, r, id)
						return
					}

					if r.Method == "POST" && strings.HasSuffix(id, "/favorite") {
						id = strings.TrimSuffix(id, "/favorite")
						h.SetFavorite(w, r, id)
						return
					}

					if r.Method == "POST" && strings.HasSuffix(id, "/highlight") {
						id = strings.TrimSuffix(id, "/highlight")
						h.SetHighlight(w, r, id)
						return
					}

					if r.Method == "GET" && strings.HasSuffix(id, "/clips") {
						id = strings.TrimSuffix(id, "/clips")
						h.GetItemClips(w, r, id)
						return
					}

					if r.Method == "GET" {
						h.GetItem(w, r, id)
						return
					}

					if r.Method == "DELETE" {
						h.DeleteItem(w, r, id)
						return
					}
				}

				if path == "/api/personas" {
					if r.Method == "POST" {
						h.CreatePersona(w, r)
						return
					}
					if r.Method == "GET" {
						h.ListPersonas(w, r)
						return
					}
					return
				}

				if strings.HasPrefix(path, "/api/personas/") {
					id := strings.TrimPrefix(path, "/api/personas/")

					if r.Method == "GET" {
						h.GetPersona(w, r, id)
						return
					}

					if r.Method == "PATCH" || r.Method == "PUT" {
						h.UpdatePersona(w, r, id)
						return
					}

					if r.Method == "DELETE" {
						h.DeletePersona(w, r, id)
						return
					}

					if r.Method == "POST" && strings.HasSuffix(id, "/avatar") {
						id = strings.TrimSuffix(id, "/avatar")
						h.UploadAvatar(w, r, id)
						return
					}
				}

				if strings.HasPrefix(path, "/api/profile/") {
					slug := strings.TrimPrefix(path, "/api/profile/")

					if strings.HasSuffix(slug, "/items") {
						slug = strings.TrimSuffix(slug, "/items")
						if r.Method == "GET" {
							h.GetProfileItems(w, r, slug)
							return
						}
					}

					if strings.HasSuffix(slug, "/shorts") {
						slug = strings.TrimSuffix(slug, "/shorts")
						if r.Method == "GET" {
							h.GetProfileShorts(w, r, slug)
							return
						}
					}

					if r.Method == "GET" {
						h.GetProfile(w, r, slug)
						return
					}
				}

				http.NotFound(w, r)
			})))

			mediaHandler := mediahttp.New(cfg.MediaRoot)
			mux.Handle("/media/", http.StripPrefix("/media/", mediaHandler))

			addr := cfg.HTTPAddr
			if cfg.DevPortScan {
				var err error
				addr, err = findAvailablePort(cfg.HTTPAddr)
				if err != nil {
					return fmt.Errorf("find available port: %w", err)
				}
			} else {
				ln, err := net.Listen("tcp", addr)
				if err != nil {
					return fmt.Errorf("listen on %s: %w", addr, err)
				}
				ln.Close()
			}

			server := &http.Server{
				Addr:    addr,
				Handler: mux,
			}

			go func() {
				logging.Info.Printf("Server listening on %s", addr)
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logging.Error.Fatalf("Server error: %v", err)
				}
			}()

			waitForShutdown(server)
			return nil
		},
	}
}

func findAvailablePort(addr string) (string, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		host = ""
		portStr = addr
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("invalid port: %w", err)
	}

	maxAttempts := 10
	for i := 0; i < maxAttempts; i++ {
		testAddr := fmt.Sprintf("%s:%d", host, port+i)
		ln, err := net.Listen("tcp", testAddr)
		if err == nil {
			ln.Close()
			if i > 0 {
				logging.Info.Printf("Port %s was in use, using %s instead", portStr, testAddr)
			}
			return testAddr, nil
		}
	}

	return "", fmt.Errorf("no available ports found after %d attempts", maxAttempts)
}

func workerCommand(cfg *config.Config) *cli.Command {
	var workerID string

	return &cli.Command{
		Name:  "worker",
		Usage: "Start the background worker",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "id",
				Usage:       "Worker ID (default: hostname)",
				Destination: &workerID,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if workerID == "" {
				hostname, err := os.Hostname()
				if err != nil {
					workerID = "worker"
				} else {
					workerID = hostname
				}
			}

			logging.Info.Printf("Starting worker: %s", workerID)

			database, err := db.New(cfg)
			if err != nil {
				return fmt.Errorf("connect to database: %w", err)
			}
			defer database.Close()

			if err := database.AutoMigrate(); err != nil {
				return fmt.Errorf("run migrations: %w", err)
			}
			logging.Info.Println("Database initialized")

			if err := storage.EnsureRootLayout(cfg.MediaRoot); err != nil {
				return fmt.Errorf("ensure layout: %w", err)
			}

			jobQueue := jobs.New(database.DB)
			processor := video.NewProcessor(cfg.MediaRoot)

			hasFFmpeg, ffmpegPath := processor.CheckFFmpeg()
			hasFFprobe, ffprobePath := processor.CheckFFprobe()

			if !hasFFmpeg || !hasFFprobe {
				logging.Error.Println("WARNING: ffmpeg/ffprobe not found. Video processing will fail.")
			} else {
				logging.Info.Printf("ffmpeg: %s", ffmpegPath)
				logging.Info.Printf("ffprobe: %s", ffprobePath)
			}

			logging.Info.Printf("Video transcode concurrency: %d", cfg.VideoTranscodeConcurrency)

			jobTypes := []string{models.JobTypeProbe, models.JobTypeTranscode, models.JobTypeThumbs, models.JobTypePhotoThumb, models.JobTypeClip, models.JobTypeHLS}

			runLoop := true
			go func() {
				sigChan := make(chan os.Signal, 1)
				signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
				<-sigChan
				logging.Info.Println("Shutdown signal received")
				runLoop = false
			}()

			pollInterval := 2 * time.Second

			for runLoop {
				job, err := jobQueue.Claim(workerID, cfg.VideoTranscodeConcurrency, jobTypes)
				if err != nil {
					if err.Error() == "record not found" {
						time.Sleep(pollInterval)
						continue
					}
					logging.Error.Printf("Failed to claim job: %v", err)
					time.Sleep(pollInterval)
					continue
				}

				logging.Info.Printf("Processing job %d (type=%s, item=%s)", job.ID, job.Type, extractItemID(job.PayloadJSON))

				payload, err := jobs.GetJobPayload(job)
				if err != nil {
					logging.Error.Printf("Failed to parse payload: %v", err)
					jobQueue.Fail(job.ID, fmt.Sprintf("Failed to parse payload: %v", err))
					continue
				}

				var execErr error
				var clipID string
				switch j := payload.(type) {
				case jobs.ProbePayload:
					execErr = processor.ProcessProbe(j.ItemID)
				case jobs.TranscodePayload:
					execErr = processor.ProcessTranscode(j.ItemID)
					if execErr == nil {
						assetsMeta, err := meta.ReadAssetsMetaByID(cfg.MediaRoot, j.ItemID)
						isAudio := err == nil && assetsMeta != nil && assetsMeta.SourceInfo != nil && assetsMeta.SourceInfo.VideoCodec == "" && assetsMeta.SourceInfo.AudioCodec != ""
						if !isAudio {
							_, hlsErr := jobQueue.Enqueue(models.JobTypeHLS, models.JobStatusQueued, 40, jobs.HLSPayload{ItemID: j.ItemID})
							if hlsErr != nil {
								logging.Error.Printf("Failed to enqueue HLS job: %v", hlsErr)
							}
						}
					}
				case jobs.ThumbsPayload:
					execErr = processor.ProcessThumbs(j.ItemID)
				case jobs.PhotoThumbPayload:
					execErr = processor.ProcessPhoto(j.ItemID)
				case jobs.ClipPayload:
					execErr = processor.ProcessClip(j.ItemID, j.ClipID, j.StartMs, j.EndMs)
					clipID = j.ClipID
				case jobs.HLSPayload:
					execErr = processor.ProcessHLS(j.ItemID)
				default:
					execErr = fmt.Errorf("unknown job type: %s", job.Type)
				}

				if execErr != nil {
					logging.Error.Printf("Job %d failed: %v", job.ID, execErr)
					jobQueue.Fail(job.ID, execErr.Error())
					if clipID != "" {
						jobQueue.UpdateClipStatus(clipID, models.ClipStatusFailed)
					}
				} else {
					logging.Info.Printf("Job %d completed successfully", job.ID)
					jobQueue.Complete(job.ID)
					if clipID != "" {
						jobQueue.UpdateClipStatus(clipID, models.ClipStatusReady)
					}
				}
			}

			logging.Info.Println("Worker shutdown complete")
			return nil
		},
	}
}

func importCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "import",
		Usage: "Import media from MediaCMS",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			logging.Info.Println("Import functionality not implemented yet")
			return nil
		},
	}
}

func rebuildDBCommand(cfg *config.Config) *cli.Command {
	var mediaRoot string
	var force bool

	return &cli.Command{
		Name:  "rebuild-db",
		Usage: "Rebuild database from sidecar meta files",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "force",
				Usage:       "Required. Drop and recreate all tables before rebuilding",
				Required:    true,
				Destination: &force,
			},
			&cli.StringFlag{
				Name:        "media-root",
				Usage:       "Media root directory (default from config)",
				DefaultText: cfg.MediaRoot,
				Destination: &mediaRoot,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if mediaRoot == "" {
				mediaRoot = cfg.MediaRoot
			}

			if !force {
				logging.Error.Println("rebuild-db requires --force flag to drop and recreate tables")
				logging.Error.Println("Usage: luna rebuild-db --force --media-root <path>")
				os.Exit(1)
			}

			logging.Info.Printf("=== Rebuild Database ===")
			logging.Info.Printf("Media root: %s", mediaRoot)
			logging.Info.Printf("SQLite path: %s", cfg.SQLitePath)

			itemsDir := storage.ItemsDir(mediaRoot)
			entries, err := os.ReadDir(itemsDir)
			if err != nil {
				return fmt.Errorf("read items directory: %w", err)
			}

			database, err := db.New(cfg)
			if err != nil {
				return fmt.Errorf("connect to database: %w", err)
			}
			defer database.Close()

			if err := database.DropTable(
				&models.MediaItem{},
				&models.Persona{},
				&models.User{},
				&models.Job{},
				&models.ClipAsset{},
				&models.Reaction{},
			); err != nil {
				return fmt.Errorf("drop tables: %w", err)
			}

			if err := database.AutoMigrateModels(
				&models.User{},
				&models.Persona{},
				&models.MediaItem{},
				&models.Job{},
				&models.ClipAsset{},
				&models.Reaction{},
			); err != nil {
				return fmt.Errorf("run migrations: %w", err)
			}
			logging.Info.Println("Database tables recreated")

			var userCount, personaCount, itemCount, errorCount int

			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}

				itemID := entry.Name()
				itemMetaPath := filepath.Join(itemsDir, itemID, "meta", meta.ItemMetaFile)

				itemMeta, err := meta.ReadItemMeta(itemMetaPath)
				if err != nil {
					logging.Error.Printf("Failed to read meta for %s: %v", itemID, err)
					errorCount++
					continue
				}

				var user models.User
				result := database.Where("username = ?", itemMeta.OwnerUsername).First(&user)
				if result.Error != nil {
					user = models.User{
						Username: itemMeta.OwnerUsername,
						Role:     models.RoleUser,
					}
					if err := database.Create(&user).Error; err != nil {
						logging.Error.Printf("Failed to create user %s: %v", itemMeta.OwnerUsername, err)
						errorCount++
						continue
					}
					userCount++
				}

				var personaID *string
				if itemMeta.PersonaDisplayName != nil && *itemMeta.PersonaDisplayName != "" {
					var persona models.Persona
					result := database.Where("user_id = ? AND display_name = ?", user.ID, *itemMeta.PersonaDisplayName).First(&persona)
					if result.Error != nil {
						slug := *itemMeta.PersonaDisplayName
						persona = models.Persona{
							ID:          id.NewULID(),
							UserID:      user.ID,
							DisplayName: *itemMeta.PersonaDisplayName,
							Slug:        slug,
						}
						if err := database.Create(&persona).Error; err != nil {
							logging.Error.Printf("Failed to create persona %s: %v", *itemMeta.PersonaDisplayName, err)
							errorCount++
							continue
						}
						personaCount++
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
					CreatedAt:     createdAt,
					UpdatedAt:     time.Now(),
				}

				if err := database.Create(&mediaItem).Error; err != nil {
					logging.Error.Printf("Failed to create media item %s: %v", itemMeta.ItemID, err)
					errorCount++
					continue
				}
				itemCount++
			}

			logging.Info.Println("=== Rebuild Complete ===")
			logging.Info.Printf("Users created: %d", userCount)
			logging.Info.Printf("Personas created: %d", personaCount)
			logging.Info.Printf("Media items restored: %d", itemCount)
			logging.Info.Printf("Errors: %d", errorCount)

			return nil
		},
	}
}

func doctorCommand(cfg *config.Config) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "doctor",
		Usage: "Check environment and dependencies",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output as JSON",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			logging.Info.Println("=== Luna Doctor ===")
			logging.Info.Printf("Media root: %s", cfg.MediaRoot)
			logging.Info.Printf("SQLite path: %s", cfg.SQLitePath)
			logging.Info.Printf("HTTP addr: %s", cfg.HTTPAddr)
			logging.Info.Printf("Video transcode concurrency: %d", cfg.VideoTranscodeConcurrency)

			logging.Info.Println("\n=== Environment Check ===")
			logging.Info.Printf("MEDIA_ROOT exists: %v", dirExists(cfg.MediaRoot))

			requiredDirs := []string{"items", "tmp", "db", "avatars"}
			for _, dir := range requiredDirs {
				exists := dirExists(filepath.Join(cfg.MediaRoot, dir))
				logging.Info.Printf("  %s/: %v", dir, exists)
			}

			logging.Info.Printf("SQLITE_PATH dir exists: %v", dirExists(dir(cfg.SQLitePath)))

			storageReport, err := report.GenerateStorageReport(cfg.MediaRoot, cfg.SQLitePath)
			if err != nil {
				logging.Error.Printf("Failed to generate storage report: %v", err)
			} else {
				logging.Info.Println("\n=== Storage Usage ===")
				logging.Info.Printf("Total: %s", report.FormatBytes(storageReport.TotalBytes))
				logging.Info.Printf("  Originals:  %s", report.FormatBytes(storageReport.Breakdown.Originals))
				logging.Info.Printf("  Derived:     %s", report.FormatBytes(storageReport.Breakdown.Derived))
				logging.Info.Printf("  Thumbnails:  %s", report.FormatBytes(storageReport.Breakdown.Thumbs))
				logging.Info.Printf("  Avatars:     %s", report.FormatBytes(storageReport.Breakdown.Avatars))
				logging.Info.Printf("  Database:    %s", report.FormatBytes(storageReport.Breakdown.DB))
				logging.Info.Printf("  Meta files:  %s", report.FormatBytes(storageReport.Breakdown.Meta))
			}

			processor := video.NewProcessor(cfg.MediaRoot)

			logging.Info.Println("\n=== Dependencies ===")
			hasFFmpeg, ffmpegPath := processor.CheckFFmpeg()
			if hasFFmpeg {
				logging.Info.Printf("ffmpeg: found at %s", ffmpegPath)
			} else {
				logging.Error.Println("ffmpeg: NOT FOUND - video processing will not work")
			}

			hasFFprobe, ffprobePath := processor.CheckFFprobe()
			if hasFFprobe {
				logging.Info.Printf("ffprobe: found at %s", ffprobePath)
			} else {
				logging.Error.Println("ffprobe: NOT FOUND - video probe will not work")
			}

			logging.Info.Println("\n=== Consistency Checks ===")

			database, err := db.New(cfg)
			if err != nil {
				logging.Error.Printf("Failed to connect to database: %v", err)
			} else {
				defer database.Close()

				if err := database.AutoMigrate(); err != nil {
					logging.Error.Printf("Failed to run migrations: %v", err)
				} else {
					dbSchemaVersion, err := database.GetSchemaVersion(meta.DBSchemaVersionKey)
					if err != nil {
						logging.Info.Printf("DB Schema Version: not set (will be initialized)")
						if setErr := database.SetSchemaVersion(meta.DBSchemaVersionKey, models.CurrentDBSchemaVersion); setErr != nil {
							logging.Error.Printf("Failed to set schema version: %v", setErr)
						}
					} else {
						if dbSchemaVersion != models.CurrentDBSchemaVersion {
							logging.Error.Printf("DB Schema Version: %d (expected %d) - MISMATCH", dbSchemaVersion, models.CurrentDBSchemaVersion)
						} else {
							logging.Info.Printf("DB Schema Version: %d", dbSchemaVersion)
						}
					}
				}

				var mediaItems []models.MediaItem
				if err := database.Find(&mediaItems).Error; err != nil {
					logging.Error.Printf("Failed to query media items: %v", err)
				} else {
					missingMeta := 0
					missingMaster := 0

					for _, item := range mediaItems {
						metaPath := filepath.Join(cfg.MediaRoot, "items", item.ID, "meta", "item.json")
						if _, err := os.Stat(metaPath); os.IsNotExist(err) {
							missingMeta++
						}

						if item.Type == "video" {
							masterPath := filepath.Join(cfg.MediaRoot, "items", item.ID, "derived", "master.mp4")
							if _, err := os.Stat(masterPath); os.IsNotExist(err) {
								missingMaster++
							}
						}
					}

					logging.Info.Printf("Media items in DB: %d", len(mediaItems))
					logging.Info.Printf("  Items with missing meta: %d", missingMeta)
					logging.Info.Printf("  Videos missing master.mp4: %d", missingMaster)

					if len(mediaItems) > 0 {
						metaCoverage := float64(len(mediaItems)-missingMeta) / float64(len(mediaItems)) * 100
						logging.Info.Printf("  Meta coverage: %.1f%%", metaCoverage)
					}

					readyVideos := 0
					missingDerived := 0
					for _, item := range mediaItems {
						if item.Type == "video" && item.HLSStatus == "ready" {
							readyVideos++
							masterPath := filepath.Join(cfg.MediaRoot, "items", item.ID, "derived", "master.mp4")
							if _, err := os.Stat(masterPath); os.IsNotExist(err) {
								missingDerived++
							}
						}
					}
					if readyVideos > 0 {
						derivedCoverage := float64(readyVideos-missingDerived) / float64(readyVideos) * 100
						logging.Info.Printf("  Derived coverage (ready videos): %.1f%%", derivedCoverage)
					}
				}

				itemsDir := filepath.Join(cfg.MediaRoot, "items")
				entries, _ := os.ReadDir(itemsDir)
				orphanMeta := 0

				for _, entry := range entries {
					if !entry.IsDir() {
						continue
					}
					itemID := entry.Name()
					metaPath := filepath.Join(itemsDir, itemID, "meta", "item.json")

					var count int64
					database.Model(&models.MediaItem{}).Where("id = ?", itemID).Count(&count)
					if count == 0 {
						if _, err := os.Stat(metaPath); err == nil {
							orphanMeta++
						}
					}
				}

				logging.Info.Printf("Orphan meta files (no DB record): %d", orphanMeta)

				var jobs []models.Job
				if err := database.Find(&jobs).Error; err != nil {
					logging.Error.Printf("Failed to query jobs: %v", err)
				} else {
					queued := 0
					running := 0
					failed := 0
					done := 0
					var oldestQueued *time.Time

					for _, job := range jobs {
						switch job.Status {
						case "queued":
							queued++
							if oldestQueued == nil || job.CreatedAt.Before(*oldestQueued) {
								oldestQueued = &job.CreatedAt
							}
						case "running":
							running++
						case "failed":
							failed++
						case "done":
							done++
						}
					}

					logging.Info.Println("\n=== Job Health ===")
					logging.Info.Printf("Total jobs: %d", len(jobs))
					logging.Info.Printf("  Queued:  %d", queued)
					logging.Info.Printf("  Running: %d", running)
					logging.Info.Printf("  Failed:  %d", failed)
					logging.Info.Printf("  Done:    %d", done)
					if oldestQueued != nil {
						logging.Info.Printf("  Oldest queued: %s", time.Since(*oldestQueued).Round(time.Second))
					}

					if running > 0 {
						lockTimeout := 5 * time.Minute
						staleRunning := 0

						for _, job := range jobs {
							if job.Status == "running" && job.LockedAt != nil {
								if time.Since(*job.LockedAt) > lockTimeout {
									staleRunning++
								}
							}
						}

						if staleRunning > 0 {
							logging.Info.Printf("WARNING: Stale running jobs (locked > 5min): %d", staleRunning)
						}
					}
				}
			}

			if jsonOutput {
				type doctorReport struct {
					MediaRoot     string                   `json:"media_root"`
					Storage       report.CategoryBreakdown `json:"storage,omitempty"`
					MediaItems    int                      `json:"media_items"`
					MissingMeta   int                      `json:"missing_meta"`
					MissingMaster int                      `json:"missing_master"`
					OrphanMeta    int                      `json:"orphan_meta"`
					JobCounts     map[string]int           `json:"job_counts"`
				}

				br := report.CategoryBreakdown{}
				if storageReport != nil {
					br = storageReport.Breakdown
				}

				dr := doctorReport{
					MediaRoot: cfg.MediaRoot,
					Storage:   br,
				}

				data, _ := json.MarshalIndent(dr, "", "  ")
				fmt.Println(string(data))
			}

			return nil
		},
	}
}

func reconcileCommand(cfg *config.Config) *cli.Command {
	var mediaRoot string
	var jsonOutput bool
	var fix bool
	var limit int

	return &cli.Command{
		Name:  "reconcile",
		Usage: "Scan and repair drift between DB and filesystem",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "media-root",
				Usage:       "Media root directory (default from config)",
				DefaultText: cfg.MediaRoot,
				Destination: &mediaRoot,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output as JSON",
				Destination: &jsonOutput,
			},
			&cli.BoolFlag{
				Name:        "fix",
				Usage:       "Apply safe fixes (regenerate meta, recreate DB rows, enqueue processing jobs)",
				Destination: &fix,
			},
			&cli.IntFlag{
				Name:        "limit",
				Usage:       "Limit number of examples in report",
				DefaultText: "5",
				Destination: &limit,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if mediaRoot == "" {
				mediaRoot = cfg.MediaRoot
			}

			logging.Info.Println("=== Luna Reconcile ===")
			logging.Info.Printf("Media root: %s", mediaRoot)
			logging.Info.Printf("SQLite path: %s", cfg.SQLitePath)

			database, err := db.New(cfg)
			if err != nil {
				return fmt.Errorf("connect to database: %w", err)
			}
			defer database.Close()

			if err := database.AutoMigrate(); err != nil {
				return fmt.Errorf("run migrations: %w", err)
			}

			if err := database.EnsureSchemaVersion(meta.DBSchemaVersionKey, models.CurrentDBSchemaVersion); err != nil {
				logging.Error.Printf("Schema version check failed: %v", err)
			}

			logging.Info.Println("\n=== Running Reconciliation ===")

			report, err := reconcile.Reconcile(database.DB, mediaRoot, limit)
			if err != nil {
				return fmt.Errorf("reconciliation failed: %w", err)
			}

			if jsonOutput {
				data, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal json: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			logging.Info.Println("\n=== Summary ===")
			logging.Info.Printf("DB Items:        %d", report.TotalDBItems)
			logging.Info.Printf("Meta Files:      %d", report.TotalMetaFiles)

			logging.Info.Println("\n--- DB -> Filesystem ---")
			logging.Info.Printf("Item dirs:       %d", report.DBToFS.Total)
			logging.Info.Printf("  Missing meta:  %d", report.DBToFS.ItemMetaMissing)
			logging.Info.Printf("  Schema mismatch: %d", report.DBToFS.SchemaMismatch)
			logging.Info.Printf("  Missing master: %d", report.DBToFS.MasterMissing)
			logging.Info.Printf("  Missing thumbs: %d", report.DBToFS.ThumbsMissing)

			logging.Info.Println("\n--- Filesystem -> DB ---")
			logging.Info.Printf("Item dirs:       %d", report.FSToDB.Total)
			logging.Info.Printf("  Orphan meta:   %d", report.FSToDB.OrphanMeta)

			logging.Info.Println("\n--- Jobs ---")
			logging.Info.Printf("Total:           %d", report.Jobs.Total)
			logging.Info.Printf("  Queued:        %d", report.Jobs.Queued)
			logging.Info.Printf("  Running:       %d", report.Jobs.Running)
			logging.Info.Printf("  Failed:        %d", report.Jobs.Failed)
			logging.Info.Printf("  Done:          %d", report.Jobs.Done)
			logging.Info.Printf("  Stuck running: %d", report.Jobs.StuckRunning)

			if len(report.Examples.MissingMeta) > 0 {
				logging.Info.Println("\n--- Missing Meta Examples ---")
				for _, id := range report.Examples.MissingMeta {
					logging.Info.Printf("  %s", id)
				}
			}

			if len(report.Examples.SchemaMismatch) > 0 {
				logging.Info.Println("\n--- Schema Mismatch Examples ---")
				for _, s := range report.Examples.SchemaMismatch {
					logging.Info.Printf("  %s", s)
				}
			}

			if len(report.Examples.OrphanMeta) > 0 {
				logging.Info.Println("\n--- Orphan Meta Examples ---")
				for _, id := range report.Examples.OrphanMeta {
					logging.Info.Printf("  %s", id)
				}
			}

			if fix {
				logging.Info.Println("\n=== Applying Fixes ===")
				fixResult, err := reconcile.Fix(database.DB, mediaRoot, report)
				if err != nil {
					return fmt.Errorf("fix failed: %w", err)
				}

				logging.Info.Printf("Meta regenerated:  %d", fixResult.MetaRegenerated)
				logging.Info.Printf("DB rows recreated: %d", fixResult.DBRowsRecreated)
				logging.Info.Printf("Jobs enqueued:     %d", fixResult.JobsEnqueued)
				logging.Info.Printf("Jobs requeued:     %d", fixResult.JobsRequeued)

				if len(fixResult.Errors) > 0 {
					logging.Info.Println("\n--- Fix Errors ---")
					for _, err := range fixResult.Errors {
						logging.Error.Printf("  %s", err)
					}
				}
			} else {
				logging.Info.Println("\nNote: Run with --fix to apply safe repairs")
			}

			return nil
		},
	}
}

func snapshotCommand(cfg *config.Config) *cli.Command {
	var outputPath string
	var includeMedia bool
	var includeDerived bool
	var includeOriginals bool

	return &cli.Command{
		Name:  "snapshot",
		Usage: "Create a backup snapshot of the media library",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "out",
				Usage:       "Output path for the snapshot (.tar.gz)",
				Required:    true,
				Destination: &outputPath,
			},
			&cli.BoolFlag{
				Name:        "include-media",
				Usage:       "Include all media (derived + originals)",
				Destination: &includeMedia,
			},
			&cli.BoolFlag{
				Name:        "include-derived",
				Usage:       "Include derived files (master.mp4, shorts, thumbs)",
				Destination: &includeDerived,
			},
			&cli.BoolFlag{
				Name:        "include-originals",
				Usage:       "Include original upload files",
				Destination: &includeOriginals,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			logging.Info.Println("=== Luna Snapshot ===")
			logging.Info.Printf("Media root: %s", cfg.MediaRoot)
			logging.Info.Printf("SQLite path: %s", cfg.SQLitePath)
			logging.Info.Printf("Output: %s", outputPath)

			opts := snapshot.SnapshotOptions{
				IncludeMedia:     includeMedia,
				IncludeDerived:   includeDerived,
				IncludeOriginals: includeOriginals,
				IncludeAvatars:   true,
				IncludeDB:        true,
				IncludeMeta:      true,
			}

			result, err := snapshot.Snapshot(cfg.MediaRoot, cfg.SQLitePath, outputPath, opts)
			if err != nil {
				return fmt.Errorf("snapshot failed: %w", err)
			}

			logging.Info.Println("\n=== Snapshot Complete ===")
			logging.Info.Printf("Created: %s", result.OutputPath)
			logging.Info.Printf("DB files: %d", result.Manifest.Counts.DBFiles)
			logging.Info.Printf("Meta files: %d", result.Manifest.Counts.MetaFiles)
			logging.Info.Printf("Avatars: %d", result.Manifest.Counts.Avatars)
			logging.Info.Printf("Derived files: %d", result.Manifest.Counts.Derived)
			logging.Info.Printf("Original files: %d", result.Manifest.Counts.Originals)
			logging.Info.Printf("Total files: %d", result.Manifest.Counts.TotalFiles)
			logging.Info.Printf("Total size: %s", report.FormatBytes(result.Manifest.Counts.TotalBytes))

			return nil
		},
	}
}

func restoreCommand(cfg *config.Config) *cli.Command {
	var inputPath string
	var mediaRoot string
	var force bool
	var rebuildDB bool

	return &cli.Command{
		Name:  "restore",
		Usage: "Restore from a backup snapshot",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "in",
				Usage:       "Input snapshot path (.tar.gz)",
				Required:    true,
				Destination: &inputPath,
			},
			&cli.StringFlag{
				Name:        "media-root",
				Usage:       "Target media root directory",
				DefaultText: cfg.MediaRoot,
				Destination: &mediaRoot,
			},
			&cli.BoolFlag{
				Name:        "force",
				Usage:       "Force restore to non-empty directory",
				Destination: &force,
			},
			&cli.BoolFlag{
				Name:        "rebuild-db",
				Usage:       "Run rebuild-db after restore",
				Destination: &rebuildDB,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if mediaRoot == "" {
				mediaRoot = cfg.MediaRoot
			}

			logging.Info.Println("=== Luna Restore ===")
			logging.Info.Printf("Snapshot: %s", inputPath)
			logging.Info.Printf("Target: %s", mediaRoot)

			if !force {
				logging.Info.Println("Note: Will refuse to overwrite existing files")
			}

			opts := restore.RestoreOptions{
				Force:     force,
				MediaRoot: mediaRoot,
			}

			result, err := restore.Restore(inputPath, opts)
			if err != nil {
				return fmt.Errorf("restore failed: %w", err)
			}

			logging.Info.Println("\n=== Restore Complete ===")
			logging.Info.Printf("DB files restored: %d", result.DBFilesRestored)
			logging.Info.Printf("Meta files restored: %d", result.MetaFilesRestored)
			logging.Info.Printf("Avatars restored: %d", result.AvatarsRestored)
			logging.Info.Printf("Total files: %d", result.TotalFiles)
			logging.Info.Printf("Total size: %s", report.FormatBytes(result.TotalBytes))

			if len(result.Warnings) > 0 {
				logging.Info.Println("\nWarnings:")
				for _, w := range result.Warnings {
					logging.Info.Printf("  %s", w)
				}
			}

			if rebuildDB {
				logging.Info.Println("\nNote: Run 'luna rebuild-db --force --media-root " + mediaRoot + "' manually to rebuild the database")
			}

			return nil
		},
	}
}

func reportStorageCommand(cfg *config.Config) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "report",
		Usage: "Generate storage and health reports",
		Commands: []*cli.Command{
			{
				Name:  "storage",
				Usage: "Show storage usage report",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:        "json",
						Usage:       "Output as JSON",
						Destination: &jsonOutput,
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					storageReport, err := report.GenerateStorageReport(cfg.MediaRoot, cfg.SQLitePath)
					if err != nil {
						return fmt.Errorf("generate report: %w", err)
					}

					if jsonOutput {
						data, err := json.MarshalIndent(storageReport, "", "  ")
						if err != nil {
							return fmt.Errorf("marshal json: %w", err)
						}
						fmt.Println(string(data))
						return nil
					}

					logging.Info.Println("=== Storage Report ===")
					logging.Info.Printf("Total size: %s", report.FormatBytes(storageReport.TotalBytes))

					logging.Info.Println("\n=== Breakdown ===")
					logging.Info.Printf("Originals:  %s", report.FormatBytes(storageReport.Breakdown.Originals))
					logging.Info.Printf("Derived:     %s", report.FormatBytes(storageReport.Breakdown.Derived))
					logging.Info.Printf("Thumbnails:  %s", report.FormatBytes(storageReport.Breakdown.Thumbs))
					logging.Info.Printf("Avatars:     %s", report.FormatBytes(storageReport.Breakdown.Avatars))
					logging.Info.Printf("Database:    %s", report.FormatBytes(storageReport.Breakdown.DB))
					logging.Info.Printf("Meta files:  %s", report.FormatBytes(storageReport.Breakdown.Meta))

					logging.Info.Println("\n=== Counts ===")
					logging.Info.Printf("Videos:  %d", storageReport.Counts.Videos)
					logging.Info.Printf("Photos:  %d", storageReport.Counts.Photos)
					logging.Info.Printf("Clips:   %d", storageReport.Counts.Clips)
					logging.Info.Printf("Total:   %d", storageReport.Counts.Total)

					if len(storageReport.TopItems) > 0 {
						logging.Info.Println("\n=== Top 10 Largest Items ===")
						for i, item := range storageReport.TopItems {
							title := item.Title
							if title == "" {
								title = item.ItemID
							}
							logging.Info.Printf("%d. %s - %s (%s)", i+1, title, item.Type, report.FormatBytes(item.Size))
						}
					}

					if len(storageReport.PersonaCounts) > 0 {
						logging.Info.Println("\n=== Items per Persona ===")
						for name, count := range storageReport.PersonaCounts {
							logging.Info.Printf("%s: %d", name, count)
						}
					}

					return nil
				},
			},
		},
	}
}

func waitForShutdown(server *http.Server) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logging.Info.Println("Shutting down...")
	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}
	logging.Info.Println("Shutdown complete")
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func dir(path string) string {
	if path == "" {
		return "."
	}
	last := len(path) - 1
	if path[last] == '/' {
		return path[:last]
	}
	return path
}

func extractItemID(payloadJSON string) string {
	if idx := strings.Index(payloadJSON, `"item_id":"`); idx >= 0 {
		start := idx + len(`"item_id":"`)
		end := start
		for end < len(payloadJSON) && payloadJSON[end] != '"' {
			end++
		}
		if end > start {
			return payloadJSON[start:end]
		}
	}
	return "unknown"
}
