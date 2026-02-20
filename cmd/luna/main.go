package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
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
	"luna/internal/useradmin"
	"luna/internal/video"

	"github.com/urfave/cli/v3"
	"gorm.io/gorm"
)

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
			userCommand(cfg),
			itemCommand(cfg),
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
			defer closeDB(database)

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
					IsActive:     true,
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

				if path == "/api/items/trash" && r.Method == "GET" {
					h.ListTrashItems(w, r)
					return
				}

				if path == "/api/me" && r.Method == "GET" {
					h.GetCurrentUser(w, r)
					return
				}

				if path == "/api/admin/users" {
					if r.Method == "GET" {
						h.AdminListUsers(w, r)
						return
					}
					if r.Method == "POST" {
						h.AdminCreateUser(w, r)
						return
					}
				}

				if strings.HasPrefix(path, "/api/admin/users/") {
					username := strings.TrimPrefix(path, "/api/admin/users/")
					if username == "" {
						http.NotFound(w, r)
						return
					}

					if r.Method == "POST" && strings.HasSuffix(username, "/activate") {
						username = strings.TrimSuffix(username, "/activate")
						h.AdminActivateUser(w, r, username)
						return
					}

					if r.Method == "POST" && strings.HasSuffix(username, "/deactivate") {
						username = strings.TrimSuffix(username, "/deactivate")
						h.AdminDeactivateUser(w, r, username)
						return
					}

					if r.Method == "POST" && strings.HasSuffix(username, "/password") {
						username = strings.TrimSuffix(username, "/password")
						h.AdminSetUserPassword(w, r, username)
						return
					}

					if r.Method == "PATCH" && strings.HasSuffix(username, "/role") {
						username = strings.TrimSuffix(username, "/role")
						h.AdminSetUserRole(w, r, username)
						return
					}
				}

				if path == "/api/shorts" && r.Method == "GET" {
					h.ListShorts(w, r)
					return
				}

				if path == "/api/playlists" {
					if r.Method == "GET" {
						h.ListPlaylists(w, r)
						return
					}
					if r.Method == "POST" {
						h.CreatePlaylist(w, r)
						return
					}
				}

				if strings.HasPrefix(path, "/api/playlists/") {
					remainder := strings.TrimPrefix(path, "/api/playlists/")
					parts := strings.Split(remainder, "/")
					if len(parts) >= 1 && parts[0] != "" {
						playlistID := parts[0]

						if len(parts) == 1 {
							if r.Method == "GET" {
								h.GetPlaylist(w, r, playlistID)
								return
							}
							if r.Method == "PATCH" || r.Method == "PUT" {
								h.UpdatePlaylist(w, r, playlistID)
								return
							}
							if r.Method == "DELETE" {
								h.DeletePlaylist(w, r, playlistID)
								return
							}
						}

						if len(parts) == 2 && parts[1] == "items" && r.Method == "POST" {
							h.AddPlaylistItem(w, r, playlistID)
							return
						}

						if len(parts) == 3 && parts[1] == "items" && r.Method == "DELETE" {
							h.RemovePlaylistItem(w, r, playlistID, parts[2])
							return
						}

						if len(parts) == 2 && parts[1] == "reorder" && r.Method == "POST" {
							h.ReorderPlaylistItems(w, r, playlistID)
							return
						}
					}
				}

				if strings.HasPrefix(path, "/api/items/") {
					id := strings.TrimPrefix(path, "/api/items/")

					if r.Method == "POST" && strings.HasSuffix(id, "/upload") {
						id = strings.TrimSuffix(id, "/upload")
						h.UploadItem(w, r, id)
						return
					}

					if r.Method == "POST" && strings.HasSuffix(id, "/reprocess") {
						id = strings.TrimSuffix(id, "/reprocess")
						h.ReprocessItem(w, r, id)
						return
					}

					if r.Method == "POST" && strings.HasSuffix(id, "/thumbnail") {
						id = strings.TrimSuffix(id, "/thumbnail")
						h.SetItemThumbnail(w, r, id)
						return
					}

					if r.Method == "POST" && strings.HasSuffix(id, "/restore") {
						id = strings.TrimSuffix(id, "/restore")
						h.RestoreItem(w, r, id)
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

					if strings.Contains(id, "/clips/") {
						parts := strings.SplitN(id, "/clips/", 2)
						if len(parts) == 2 && parts[0] != "" && parts[1] != "" && r.Method == "DELETE" {
							h.DeleteClip(w, r, parts[0], parts[1])
							return
						}
						if len(parts) == 2 && parts[0] != "" && parts[1] != "" && (r.Method == "PATCH" || r.Method == "PUT") {
							h.UpdateClip(w, r, parts[0], parts[1])
							return
						}
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
					if r.Method == "PATCH" || r.Method == "PUT" {
						h.UpdateItem(w, r, id)
						return
					}

					if r.Method == "DELETE" {
						if strings.HasSuffix(id, "/purge") {
							id = strings.TrimSuffix(id, "/purge")
							h.PurgeItemAdmin(w, r, id)
							return
						}
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

					if strings.HasSuffix(slug, "/audio") {
						slug = strings.TrimSuffix(slug, "/audio")
						if r.Method == "GET" {
							h.GetProfileAudio(w, r, slug)
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
				if err := ln.Close(); err != nil {
					return fmt.Errorf("close listener: %w", err)
				}
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
			if err := ln.Close(); err != nil {
				return "", fmt.Errorf("close listener: %w", err)
			}
			if i > 0 {
				logging.Info.Printf("Port %s was in use, using %s instead", portStr, testAddr)
			}
			return testAddr, nil
		}
	}

	return "", fmt.Errorf("no available ports found after %d attempts", maxAttempts)
}

func openDBAndMigrate(cfg *config.Config) (*db.DB, error) {
	database, err := db.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := database.AutoMigrate(); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return database, nil
}

func userCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "user",
		Usage: "Manage local users",
		Commands: []*cli.Command{
			userCreateCommand(cfg),
			userListCommand(cfg),
			userSetRoleCommand(cfg),
			userDeactivateCommand(cfg),
			userActivateCommand(cfg),
			userPasswdCommand(cfg),
			userPurgeCommand(cfg),
		},
	}
}

func itemCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "item",
		Usage: "Manage media items",
		Commands: []*cli.Command{
			itemPurgeCommand(cfg),
		},
	}
}

func itemPurgeCommand(cfg *config.Config) *cli.Command {
	var itemID string
	var execute bool
	var confirm string

	return &cli.Command{
		Name:  "purge",
		Usage: "Permanently remove a soft-deleted item and all associated records (default: dry-run)",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "id", Required: true, Destination: &itemID},
			&cli.BoolFlag{Name: "execute", Destination: &execute},
			&cli.StringFlag{Name: "confirm", Destination: &confirm},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			itemID = strings.TrimSpace(itemID)
			if itemID == "" {
				return fmt.Errorf("item id is required")
			}

			database, err := openDBAndMigrate(cfg)
			if err != nil {
				return err
			}
			defer closeDB(database)

			var item models.MediaItem
			if err := database.Unscoped().Where("id = ?", itemID).First(&item).Error; err != nil {
				return fmt.Errorf("find item %q: %w", itemID, err)
			}
			if !item.DeletedAt.Valid {
				return fmt.Errorf("item %q must be soft-deleted before purge", itemID)
			}

			var reactionCount int64
			if err := database.Model(&models.Reaction{}).Where("item_id = ?", itemID).Count(&reactionCount).Error; err != nil {
				return fmt.Errorf("count reactions: %w", err)
			}
			var favoriteCount int64
			if err := database.Model(&models.Favorite{}).Where("item_id = ?", itemID).Count(&favoriteCount).Error; err != nil {
				return fmt.Errorf("count favorites: %w", err)
			}
			var clipCount int64
			if err := database.Unscoped().Model(&models.ClipAsset{}).Where("item_id = ?", itemID).Count(&clipCount).Error; err != nil {
				return fmt.Errorf("count clips: %w", err)
			}
			var playlistRefCount int64
			if err := database.Model(&models.PlaylistItem{}).Where("item_id = ?", itemID).Count(&playlistRefCount).Error; err != nil {
				return fmt.Errorf("count playlist refs: %w", err)
			}
			var jobCount int64
			if err := database.Model(&models.Job{}).Where("payload_json LIKE ?", "%"+itemID+"%").Count(&jobCount).Error; err != nil {
				return fmt.Errorf("count jobs: %w", err)
			}
			itemDir := filepath.Join(cfg.MediaRoot, "items", itemID)
			size, err := dirSize(itemDir)
			if err != nil {
				return fmt.Errorf("measure item directory: %w", err)
			}

			fmt.Printf("Purge summary for item %q\n", itemID)
			fmt.Printf("  title: %s\n", item.Title)
			fmt.Printf("  type: %s\n", item.Type)
			fmt.Printf("  owner_user_id: %d\n", item.UserID)
			fmt.Printf("  deleted_at: %s\n", item.DeletedAt.Time.UTC().Format(time.RFC3339))
			fmt.Printf("  reactions: %d\n", reactionCount)
			fmt.Printf("  favorites: %d\n", favoriteCount)
			fmt.Printf("  clip_assets: %d\n", clipCount)
			fmt.Printf("  playlist_refs: %d\n", playlistRefCount)
			fmt.Printf("  jobs_matching_item: %d\n", jobCount)
			fmt.Printf("  estimated_item_bytes: %d\n", size)

			if !execute {
				fmt.Println("Dry-run only. Re-run with --execute --confirm <item_id> to apply purge.")
				return nil
			}
			if strings.TrimSpace(confirm) != itemID {
				return fmt.Errorf("confirmation mismatch: set --confirm %s", itemID)
			}

			if err := database.Transaction(func(tx *gorm.DB) error {
				if err := tx.Where("item_id = ?", itemID).Delete(&models.Reaction{}).Error; err != nil {
					return err
				}
				if err := tx.Where("item_id = ?", itemID).Delete(&models.Favorite{}).Error; err != nil {
					return err
				}
				if err := tx.Where("item_id = ?", itemID).Delete(&models.PlaylistItem{}).Error; err != nil {
					return err
				}
				if err := tx.Unscoped().Where("item_id = ?", itemID).Delete(&models.ClipAsset{}).Error; err != nil {
					return err
				}
				if err := tx.Where("payload_json LIKE ?", "%"+itemID+"%").Delete(&models.Job{}).Error; err != nil {
					return err
				}
				if err := tx.Unscoped().Delete(&models.MediaItem{}, "id = ?", itemID).Error; err != nil {
					return err
				}
				return nil
			}); err != nil {
				return fmt.Errorf("purge item records: %w", err)
			}

			if err := os.RemoveAll(itemDir); err != nil {
				return fmt.Errorf("remove item directory %s: %w", itemDir, err)
			}

			fmt.Printf("Purged item %q and associated content.\n", itemID)
			return nil
		},
	}
}

func userCreateCommand(cfg *config.Config) *cli.Command {
	var username string
	var password string
	var role string

	return &cli.Command{
		Name:  "create",
		Usage: "Create a user",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "username", Required: true, Destination: &username},
			&cli.StringFlag{Name: "password", Required: true, Destination: &password},
			&cli.StringFlag{Name: "role", Value: models.RoleUser, Destination: &role},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			normalizedRole, err := useradmin.NormalizeRole(role)
			if err != nil {
				return err
			}
			if strings.TrimSpace(username) == "" {
				return fmt.Errorf("username is required")
			}
			if strings.TrimSpace(password) == "" {
				return fmt.Errorf("password is required")
			}

			database, err := openDBAndMigrate(cfg)
			if err != nil {
				return err
			}
			defer closeDB(database)

			var existing models.User
			if err := database.Where("username = ?", username).First(&existing).Error; err == nil {
				return fmt.Errorf("user %q already exists", username)
			}

			passwordHash, err := auth.HashPassword(password)
			if err != nil {
				return fmt.Errorf("hash password: %w", err)
			}

			user := models.User{
				Username:     username,
				PasswordHash: passwordHash,
				Role:         normalizedRole,
				IsActive:     true,
			}
			if err := database.Create(&user).Error; err != nil {
				return fmt.Errorf("create user: %w", err)
			}

			logging.Info.Printf("Created user %q with role %q", user.Username, user.Role)
			return nil
		},
	}
}

func userListCommand(cfg *config.Config) *cli.Command {
	var showAll bool
	var showActive bool
	var showInactive bool

	return &cli.Command{
		Name:  "list",
		Usage: "List users",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "all", Destination: &showAll},
			&cli.BoolFlag{Name: "active", Destination: &showActive},
			&cli.BoolFlag{Name: "inactive", Destination: &showInactive},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if (showAll && showActive) || (showAll && showInactive) || (showActive && showInactive) {
				return fmt.Errorf("use at most one of --all, --active, --inactive")
			}

			database, err := openDBAndMigrate(cfg)
			if err != nil {
				return err
			}
			defer closeDB(database)

			query := database.Model(&models.User{})
			if showActive {
				query = query.Where("is_active = ?", true)
			}
			if showInactive {
				query = query.Where("is_active = ?", false)
			}

			var users []models.User
			if err := query.Order("id ASC").Find(&users).Error; err != nil {
				return fmt.Errorf("list users: %w", err)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(w, "ID\tUSERNAME\tROLE\tSTATUS\tCREATED"); err != nil {
				return fmt.Errorf("write user list header: %w", err)
			}
			for _, user := range users {
				status := "active"
				if !user.IsActive {
					status = "inactive"
				}
				if _, err := fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
					user.ID,
					user.Username,
					user.Role,
					status,
					user.CreatedAt.UTC().Format(time.RFC3339),
				); err != nil {
					return fmt.Errorf("write user list row: %w", err)
				}
			}
			_ = w.Flush()
			return nil
		},
	}
}

func userSetRoleCommand(cfg *config.Config) *cli.Command {
	var username string
	var role string

	return &cli.Command{
		Name:  "set-role",
		Usage: "Set a user's role",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "username", Required: true, Destination: &username},
			&cli.StringFlag{Name: "role", Required: true, Destination: &role},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			normalizedRole, err := useradmin.NormalizeRole(role)
			if err != nil {
				return err
			}

			database, err := openDBAndMigrate(cfg)
			if err != nil {
				return err
			}
			defer closeDB(database)

			var user models.User
			if err := database.Where("username = ?", username).First(&user).Error; err != nil {
				return fmt.Errorf("find user %q: %w", username, err)
			}
			if user.Role == normalizedRole {
				logging.Info.Printf("User %q already has role %q", username, normalizedRole)
				return nil
			}
			if err := useradmin.EnsureCanChangeRole(database.DB, user, normalizedRole); err != nil {
				return err
			}

			if err := database.Model(&user).Update("role", normalizedRole).Error; err != nil {
				return fmt.Errorf("update role: %w", err)
			}
			logging.Info.Printf("Updated user %q role to %q", username, normalizedRole)
			return nil
		},
	}
}

func userDeactivateCommand(cfg *config.Config) *cli.Command {
	var username string
	var reason string

	return &cli.Command{
		Name:  "deactivate",
		Usage: "Deactivate a user without deleting content",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "username", Required: true, Destination: &username},
			&cli.StringFlag{Name: "reason", Destination: &reason},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			database, err := openDBAndMigrate(cfg)
			if err != nil {
				return err
			}
			defer closeDB(database)

			var user models.User
			if err := database.Where("username = ?", username).First(&user).Error; err != nil {
				return fmt.Errorf("find user %q: %w", username, err)
			}
			if !user.IsActive {
				logging.Info.Printf("User %q is already inactive", username)
				return nil
			}
			if err := useradmin.EnsureCanDeactivate(database.DB, user); err != nil {
				return err
			}

			now := time.Now().UTC()
			updates := map[string]interface{}{
				"is_active":      false,
				"deactivated_at": now,
			}
			if err := database.Model(&user).Updates(updates).Error; err != nil {
				return fmt.Errorf("deactivate user: %w", err)
			}
			if strings.TrimSpace(reason) != "" {
				logging.Info.Printf("Deactivated user %q (reason: %s)", username, reason)
				return nil
			}
			logging.Info.Printf("Deactivated user %q", username)
			return nil
		},
	}
}

func userActivateCommand(cfg *config.Config) *cli.Command {
	var username string

	return &cli.Command{
		Name:  "activate",
		Usage: "Activate a deactivated user",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "username", Required: true, Destination: &username},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			database, err := openDBAndMigrate(cfg)
			if err != nil {
				return err
			}
			defer closeDB(database)

			var user models.User
			if err := database.Where("username = ?", username).First(&user).Error; err != nil {
				return fmt.Errorf("find user %q: %w", username, err)
			}
			if user.IsActive {
				logging.Info.Printf("User %q is already active", username)
				return nil
			}

			updates := map[string]interface{}{
				"is_active":      true,
				"deactivated_at": nil,
			}
			if err := database.Model(&user).Updates(updates).Error; err != nil {
				return fmt.Errorf("activate user: %w", err)
			}
			logging.Info.Printf("Activated user %q", username)
			return nil
		},
	}
}

func userPasswdCommand(cfg *config.Config) *cli.Command {
	var username string
	var password string

	return &cli.Command{
		Name:  "passwd",
		Usage: "Set a user's password",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "username", Required: true, Destination: &username},
			&cli.StringFlag{Name: "password", Required: true, Destination: &password},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if strings.TrimSpace(password) == "" {
				return fmt.Errorf("password is required")
			}

			database, err := openDBAndMigrate(cfg)
			if err != nil {
				return err
			}
			defer closeDB(database)

			var user models.User
			if err := database.Where("username = ?", username).First(&user).Error; err != nil {
				return fmt.Errorf("find user %q: %w", username, err)
			}

			passwordHash, err := auth.HashPassword(password)
			if err != nil {
				return fmt.Errorf("hash password: %w", err)
			}
			if err := database.Model(&user).Update("password_hash", passwordHash).Error; err != nil {
				return fmt.Errorf("update password: %w", err)
			}
			logging.Info.Printf("Updated password for user %q", username)
			return nil
		},
	}
}

func userPurgeCommand(cfg *config.Config) *cli.Command {
	var username string
	var execute bool
	var confirm string

	return &cli.Command{
		Name:  "purge",
		Usage: "Permanently remove a user and all owned content (default: dry-run)",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "username", Required: true, Destination: &username},
			&cli.BoolFlag{Name: "execute", Destination: &execute},
			&cli.StringFlag{Name: "confirm", Destination: &confirm},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			database, err := openDBAndMigrate(cfg)
			if err != nil {
				return err
			}
			defer closeDB(database)

			var user models.User
			if err := database.Where("username = ?", username).First(&user).Error; err != nil {
				return fmt.Errorf("find user %q: %w", username, err)
			}

			var itemIDs []string
			if err := database.Unscoped().
				Model(&models.MediaItem{}).
				Where("user_id = ?", user.ID).
				Pluck("id", &itemIDs).Error; err != nil {
				return fmt.Errorf("list owned items: %w", err)
			}

			var personaCount int64
			if err := database.Unscoped().Model(&models.Persona{}).Where("user_id = ?", user.ID).Count(&personaCount).Error; err != nil {
				return fmt.Errorf("count personas: %w", err)
			}

			var mediaCount int64
			if err := database.Unscoped().Model(&models.MediaItem{}).Where("user_id = ?", user.ID).Count(&mediaCount).Error; err != nil {
				return fmt.Errorf("count media items: %w", err)
			}

			var sessionCount int64
			if err := database.Model(&models.Session{}).Where("user_id = ?", user.ID).Count(&sessionCount).Error; err != nil {
				return fmt.Errorf("count sessions: %w", err)
			}

			var ownReactionCount int64
			if err := database.Model(&models.Reaction{}).Where("user_id = ?", user.ID).Count(&ownReactionCount).Error; err != nil {
				return fmt.Errorf("count user reactions: %w", err)
			}

			var ownFavoriteCount int64
			if err := database.Model(&models.Favorite{}).Where("user_id = ?", user.ID).Count(&ownFavoriteCount).Error; err != nil {
				return fmt.Errorf("count user favorites: %w", err)
			}

			var playlistCount int64
			if err := database.Model(&models.Playlist{}).Where("user_id = ?", user.ID).Count(&playlistCount).Error; err != nil {
				return fmt.Errorf("count playlists: %w", err)
			}

			var playlistItemCount int64
			if playlistCount > 0 {
				if err := database.Model(&models.PlaylistItem{}).
					Joins("JOIN playlists ON playlists.id = playlist_items.playlist_id").
					Where("playlists.user_id = ?", user.ID).
					Count(&playlistItemCount).Error; err != nil {
					return fmt.Errorf("count playlist items: %w", err)
				}
			}

			var assetReactionCount int64
			var assetFavoriteCount int64
			var clipCount int64
			if len(itemIDs) > 0 {
				if err := database.Model(&models.Reaction{}).Where("item_id IN ?", itemIDs).Count(&assetReactionCount).Error; err != nil {
					return fmt.Errorf("count item reactions: %w", err)
				}
				if err := database.Model(&models.Favorite{}).Where("item_id IN ?", itemIDs).Count(&assetFavoriteCount).Error; err != nil {
					return fmt.Errorf("count item favorites: %w", err)
				}
				if err := database.Model(&models.ClipAsset{}).Where("item_id IN ?", itemIDs).Count(&clipCount).Error; err != nil {
					return fmt.Errorf("count clip assets: %w", err)
				}
			}

			var bytesTotal int64
			for _, itemID := range itemIDs {
				size, err := dirSize(filepath.Join(cfg.MediaRoot, "items", itemID))
				if err != nil {
					return fmt.Errorf("measure item %s: %w", itemID, err)
				}
				bytesTotal += size
			}

			fmt.Printf("Purge summary for user %q (id=%d)\n", user.Username, user.ID)
			fmt.Printf("  personas: %d\n", personaCount)
			fmt.Printf("  media_items: %d\n", mediaCount)
			fmt.Printf("  clip_assets: %d\n", clipCount)
			fmt.Printf("  sessions: %d\n", sessionCount)
			fmt.Printf("  reactions_by_user: %d\n", ownReactionCount)
			fmt.Printf("  favorites_by_user: %d\n", ownFavoriteCount)
			fmt.Printf("  playlists: %d\n", playlistCount)
			fmt.Printf("  playlist_items: %d\n", playlistItemCount)
			fmt.Printf("  reactions_on_user_items: %d\n", assetReactionCount)
			fmt.Printf("  favorites_on_user_items: %d\n", assetFavoriteCount)
			fmt.Printf("  estimated_item_bytes: %d\n", bytesTotal)

			if !execute {
				fmt.Println("Dry-run only. Re-run with --execute --confirm <username> to apply purge.")
				return nil
			}
			if strings.TrimSpace(confirm) != user.Username {
				return fmt.Errorf("confirmation mismatch: set --confirm %s", user.Username)
			}

			if user.Role == models.RoleAdmin && user.IsActive {
				adminCount, err := useradmin.CountActiveAdmins(database.DB)
				if err != nil {
					return fmt.Errorf("count active admins: %w", err)
				}
				if adminCount <= 1 {
					return fmt.Errorf("cannot purge the last active admin")
				}
			}

			if err := database.Transaction(func(tx *gorm.DB) error {
				if len(itemIDs) > 0 {
					if err := tx.Where("item_id IN ?", itemIDs).Delete(&models.Reaction{}).Error; err != nil {
						return err
					}
					if err := tx.Where("item_id IN ?", itemIDs).Delete(&models.Favorite{}).Error; err != nil {
						return err
					}
					if err := tx.Unscoped().Where("item_id IN ?", itemIDs).Delete(&models.ClipAsset{}).Error; err != nil {
						return err
					}
					if err := tx.Unscoped().Where("id IN ?", itemIDs).Delete(&models.MediaItem{}).Error; err != nil {
						return err
					}
				}

				if err := tx.Where("user_id = ?", user.ID).Delete(&models.Reaction{}).Error; err != nil {
					return err
				}
				if err := tx.Where("user_id = ?", user.ID).Delete(&models.Favorite{}).Error; err != nil {
					return err
				}
				var playlistIDs []uint
				if err := tx.Model(&models.Playlist{}).Where("user_id = ?", user.ID).Pluck("id", &playlistIDs).Error; err != nil {
					return err
				}
				if len(playlistIDs) > 0 {
					if err := tx.Where("playlist_id IN ?", playlistIDs).Delete(&models.PlaylistItem{}).Error; err != nil {
						return err
					}
					if err := tx.Where("id IN ?", playlistIDs).Delete(&models.Playlist{}).Error; err != nil {
						return err
					}
				}
				if err := tx.Where("user_id = ?", user.ID).Delete(&models.Session{}).Error; err != nil {
					return err
				}
				if err := tx.Unscoped().Where("user_id = ?", user.ID).Delete(&models.Persona{}).Error; err != nil {
					return err
				}
				if err := tx.Unscoped().Delete(&models.User{}, user.ID).Error; err != nil {
					return err
				}
				return nil
			}); err != nil {
				return fmt.Errorf("purge user records: %w", err)
			}

			for _, itemID := range itemIDs {
				itemDir := filepath.Join(cfg.MediaRoot, "items", itemID)
				if err := os.RemoveAll(itemDir); err != nil {
					return fmt.Errorf("remove item directory %s: %w", itemDir, err)
				}
			}

			userAvatarDir := filepath.Join(cfg.MediaRoot, "avatars", "users", fmt.Sprintf("%d", user.ID))
			if err := os.RemoveAll(userAvatarDir); err != nil {
				return fmt.Errorf("remove avatar directory %s: %w", userAvatarDir, err)
			}

			fmt.Printf("Purged user %q and owned content.\n", user.Username)
			return nil
		},
	}
}

func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		total += info.Size()
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return total, err
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
			defer closeDB(database)

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

			const (
				minIdleBackoff = 1 * time.Second
				maxIdleBackoff = 30 * time.Second
			)
			idleBackoff := time.Duration(0)

			for runLoop {
				job, err := jobQueue.Claim(workerID, cfg.VideoTranscodeConcurrency, jobTypes)
				if err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						if idleBackoff == 0 {
							idleBackoff = minIdleBackoff
						} else if idleBackoff < maxIdleBackoff {
							idleBackoff += minIdleBackoff
							if idleBackoff > maxIdleBackoff {
								idleBackoff = maxIdleBackoff
							}
						}
						time.Sleep(idleBackoff)
						continue
					}
					logging.Error.Printf("Failed to claim job: %v", err)
					if idleBackoff == 0 {
						idleBackoff = minIdleBackoff
					}
					time.Sleep(idleBackoff)
					continue
				}
				idleBackoff = 0

				logging.Info.Printf("Processing job %d (type=%s, item=%s)", job.ID, job.Type, extractItemID(job.PayloadJSON))

				payload, err := jobs.GetJobPayload(job)
				if err != nil {
					logging.Error.Printf("Failed to parse payload: %v", err)
					if failErr := jobQueue.Fail(job.ID, fmt.Sprintf("Failed to parse payload: %v", err)); failErr != nil {
						logging.Error.Printf("Failed to update failed job %d: %v", job.ID, failErr)
					}
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
							_, hlsErr := jobQueue.Enqueue(models.JobTypeHLS, models.JobStatusQueued, 40, jobs.HLSPayload(j))
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
					if failErr := jobQueue.Fail(job.ID, execErr.Error()); failErr != nil {
						logging.Error.Printf("Failed to update failed job %d: %v", job.ID, failErr)
					}
					if clipID != "" {
						if clipErr := jobQueue.UpdateClipStatus(clipID, models.ClipStatusFailed); clipErr != nil {
							logging.Error.Printf("Failed to update clip %s status: %v", clipID, clipErr)
						}
					}
				} else {
					logging.Info.Printf("Job %d completed successfully", job.ID)
					if completeErr := jobQueue.Complete(job.ID); completeErr != nil {
						logging.Error.Printf("Failed to mark job %d complete: %v", job.ID, completeErr)
					}
					if clipID != "" {
						if clipErr := jobQueue.UpdateClipStatus(clipID, models.ClipStatusReady); clipErr != nil {
							logging.Error.Printf("Failed to update clip %s status: %v", clipID, clipErr)
						}
					}
				}
				idleBackoff = 0
			}

			logging.Info.Println("Worker shutdown complete")
			return nil
		},
	}
}

func importCommand(cfg *config.Config) *cli.Command {
	var srcDir string
	var username string

	return &cli.Command{
		Name:  "import",
		Usage: "Import media from MediaCMS",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "src",
				Usage:       "Source directory to import files from",
				Required:    true,
				Destination: &srcDir,
			},
			&cli.StringFlag{
				Name:        "user",
				Usage:       "Owner username for imported files",
				Value:       "devuser",
				Destination: &username,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if err := storage.EnsureRootLayout(cfg.MediaRoot); err != nil {
				return fmt.Errorf("ensure media root layout: %w", err)
			}

			database, err := db.New(cfg)
			if err != nil {
				return fmt.Errorf("connect to database: %w", err)
			}
			defer closeDB(database)

			if err := database.AutoMigrate(); err != nil {
				return fmt.Errorf("run migrations: %w", err)
			}

			var user models.User
			if err := database.Where("username = ?", username).First(&user).Error; err != nil {
				passwordHash, hashErr := auth.HashPassword("changeme")
				if hashErr != nil {
					return fmt.Errorf("hash imported user password: %w", hashErr)
				}
				user = models.User{
					Username:     username,
					PasswordHash: passwordHash,
					Role:         models.RoleUser,
					IsActive:     true,
				}
				if createErr := database.Create(&user).Error; createErr != nil {
					return fmt.Errorf("create import user: %w", createErr)
				}
				logging.Info.Printf("Created import user %q with default password %q", username, "changeme")
			}

			jobQueue := jobs.New(database.DB)
			imported := 0
			skipped := 0

			err = filepath.WalkDir(srcDir, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					return nil
				}

				ext := strings.ToLower(filepath.Ext(d.Name()))
				mediaType := inferMediaTypeByExt(ext)
				if mediaType == "" {
					skipped++
					return nil
				}

				itemID := id.NewULID()
				if err := storage.EnsureItemDirs(cfg.MediaRoot, itemID); err != nil {
					return fmt.Errorf("ensure item dirs for %s: %w", itemID, err)
				}

				srcFile, err := os.Open(path)
				if err != nil {
					return fmt.Errorf("open source file %s: %w", path, err)
				}

				destPath := filepath.Join(cfg.MediaRoot, "items", itemID, "original", "upload"+ext)
				destFile, err := os.Create(destPath)
				if err != nil {
					_ = srcFile.Close()
					return fmt.Errorf("create dest file %s: %w", destPath, err)
				}

				if _, err := io.Copy(destFile, srcFile); err != nil {
					_ = srcFile.Close()
					_ = destFile.Close()
					return fmt.Errorf("copy %s to %s: %w", path, destPath, err)
				}
				if err := srcFile.Close(); err != nil {
					_ = destFile.Close()
					return fmt.Errorf("close source file %s: %w", path, err)
				}
				if err := destFile.Close(); err != nil {
					return fmt.Errorf("close dest file %s: %w", destPath, err)
				}

				title := strings.TrimSuffix(d.Name(), ext)
				now := time.Now().UTC()
				item := models.MediaItem{
					ID:          itemID,
					UserID:      user.ID,
					Type:        mediaType,
					Title:       title,
					Description: "",
					CreatedAt:   now,
					UpdatedAt:   now,
				}

				if err := database.Create(&item).Error; err != nil {
					return fmt.Errorf("create media item for %s: %w", path, err)
				}

				itemMeta := &meta.ItemMeta{
					Schema:        meta.SchemaVersion,
					ItemID:        itemID,
					Type:          mediaType,
					OwnerUsername: user.Username,
					Title:         title,
					Description:   "",
					CreatedAt:     now.Format(time.RFC3339),
					State: meta.ItemState{
						Highlighted: false,
					},
					Original: meta.OriginalFile{
						Filename: d.Name(),
						Path:     "original/upload" + ext,
					},
				}
				if err := meta.WriteItemMetaAtomic(cfg.MediaRoot, itemID, itemMeta); err != nil {
					return fmt.Errorf("write item meta for %s: %w", itemID, err)
				}

				assetsMeta := &meta.AssetsMeta{
					Schema:     meta.SchemaVersion,
					Assets:     []meta.Asset{},
					Thumbnails: []meta.Thumbnail{},
					Photos:     []meta.Photo{},
				}
				if err := meta.WriteAssetsMetaAtomic(cfg.MediaRoot, itemID, assetsMeta); err != nil {
					return fmt.Errorf("write assets meta for %s: %w", itemID, err)
				}

				switch mediaType {
				case models.MediaTypeVideo:
					if err := jobQueue.EnqueueVideoProcessing(itemID); err != nil {
						return fmt.Errorf("enqueue video processing for %s: %w", itemID, err)
					}
				case models.MediaTypeAudio:
					if err := jobQueue.EnqueueAudioProcessing(itemID); err != nil {
						return fmt.Errorf("enqueue audio processing for %s: %w", itemID, err)
					}
				case models.MediaTypePhoto:
					if err := jobQueue.EnqueuePhotoProcessing(itemID); err != nil {
						return fmt.Errorf("enqueue photo processing for %s: %w", itemID, err)
					}
				}

				imported++
				return nil
			})
			if err != nil {
				return fmt.Errorf("walk source directory: %w", err)
			}

			logging.Info.Printf("Import complete. Imported=%d Skipped=%d", imported, skipped)
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
			defer closeDB(database)

			if err := database.DropTable(
				&models.MediaItem{},
				&models.Persona{},
				&models.User{},
				&models.Session{},
				&models.Job{},
				&models.ClipAsset{},
				&models.Reaction{},
				&models.Favorite{},
				&models.PlaylistItem{},
				&models.Playlist{},
			); err != nil {
				return fmt.Errorf("drop tables: %w", err)
			}

			if err := database.AutoMigrateModels(
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
			); err != nil {
				return fmt.Errorf("run migrations: %w", err)
			}
			logging.Info.Println("Database tables recreated")

			jobQueue := jobs.New(database.DB)
			var userCount, personaCount, itemCount, errorCount, jobsEnqueued int

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
					passwordHash, hashErr := auth.HashPassword("changeme")
					if hashErr != nil {
						logging.Error.Printf("Failed to hash password for user %s: %v", itemMeta.OwnerUsername, hashErr)
						errorCount++
						continue
					}
					user = models.User{
						Username:     itemMeta.OwnerUsername,
						PasswordHash: passwordHash,
						Role:         models.RoleUser,
						IsActive:     true,
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
				if itemMeta.State.DeletedAt != nil {
					if deletedAt, err := time.Parse(time.RFC3339, *itemMeta.State.DeletedAt); err == nil {
						database.Model(&mediaItem).Update("deleted_at", deletedAt)
					}
				}

				assetsMeta, _ := meta.ReadAssetsMetaByID(mediaRoot, itemID)
				if assetsMeta != nil {
					for _, asset := range assetsMeta.Assets {
						if asset.Kind != "short_clip" || asset.ClipID == "" {
							continue
						}

						clipPath := filepath.Join(mediaRoot, "items", itemID, asset.StoragePath)
						clipStatus := models.ClipStatusPending
						if _, err := os.Stat(clipPath); err == nil {
							clipStatus = models.ClipStatusReady
						}

						clip := models.ClipAsset{
							ID:          asset.ClipID,
							ItemID:      itemID,
							StoragePath: asset.StoragePath,
							Status:      clipStatus,
							StartMs:     asset.StartMs,
							EndMs:       asset.EndMs,
							DurationMs:  asset.EndMs - asset.StartMs,
							Width:       asset.Width,
							Height:      asset.Height,
							CropMode:    asset.CropMode,
							CreatedAt:   time.Now(),
							UpdatedAt:   time.Now(),
						}
						if clip.DurationMs < 0 {
							clip.DurationMs = 0
						}
						database.Where("id = ?", clip.ID).FirstOrCreate(&clip)

						if clipStatus != models.ClipStatusReady {
							if _, err := jobQueue.Enqueue(models.JobTypeClip, models.JobStatusQueued, 75, jobs.ClipPayload{
								ItemID:  itemID,
								ClipID:  asset.ClipID,
								StartMs: asset.StartMs,
								EndMs:   asset.EndMs,
							}); err == nil {
								jobsEnqueued++
							}
						}
					}
				}

				if itemMeta.Type == models.MediaTypeVideo {
					if assetsMeta == nil || assetsMeta.SourceInfo == nil {
						if _, err := jobQueue.Enqueue(models.JobTypeProbe, models.JobStatusQueued, 100, jobs.ProbePayload{ItemID: itemID}); err == nil {
							jobsEnqueued++
						}
					}

					masterPath := filepath.Join(mediaRoot, "items", itemID, "derived", "master.mp4")
					if _, err := os.Stat(masterPath); os.IsNotExist(err) {
						if _, err := jobQueue.Enqueue(models.JobTypeTranscode, models.JobStatusQueued, 50, jobs.TranscodePayload{ItemID: itemID}); err == nil {
							jobsEnqueued++
						}
					}

					if assetsMeta == nil || len(assetsMeta.Thumbnails) == 0 {
						if _, err := jobQueue.Enqueue(models.JobTypeThumbs, models.JobStatusQueued, 50, jobs.ThumbsPayload{ItemID: itemID}); err == nil {
							jobsEnqueued++
						}
					}
				}

				if itemMeta.Type == models.MediaTypeAudio {
					if assetsMeta == nil || assetsMeta.SourceInfo == nil {
						if _, err := jobQueue.Enqueue(models.JobTypeProbe, models.JobStatusQueued, 100, jobs.ProbePayload{ItemID: itemID}); err == nil {
							jobsEnqueued++
						}
					}
					audioPath := filepath.Join(mediaRoot, "items", itemID, "derived", "master.m4a")
					if _, err := os.Stat(audioPath); os.IsNotExist(err) {
						if _, err := jobQueue.Enqueue(models.JobTypeTranscode, models.JobStatusQueued, 50, jobs.TranscodePayload{ItemID: itemID}); err == nil {
							jobsEnqueued++
						}
					}
				}

				if itemMeta.Type == models.MediaTypePhoto {
					needPhotoJob := false
					if assetsMeta == nil || len(assetsMeta.Photos) == 0 {
						needPhotoJob = true
					} else {
						var displayPath, thumbPath string
						for _, photo := range assetsMeta.Photos {
							switch photo.Kind {
							case "display":
								displayPath = photo.StoragePath
							case "thumb":
								thumbPath = photo.StoragePath
							}
						}
						if displayPath == "" || thumbPath == "" {
							needPhotoJob = true
						} else {
							displayAbs := filepath.Join(mediaRoot, "items", itemID, displayPath)
							thumbAbs := filepath.Join(mediaRoot, "items", itemID, thumbPath)
							if _, err := os.Stat(displayAbs); os.IsNotExist(err) {
								needPhotoJob = true
							}
							if _, err := os.Stat(thumbAbs); os.IsNotExist(err) {
								needPhotoJob = true
							}
						}
					}
					if needPhotoJob {
						if _, err := jobQueue.Enqueue(models.JobTypePhotoThumb, models.JobStatusQueued, 60, jobs.PhotoThumbPayload{ItemID: itemID}); err == nil {
							jobsEnqueued++
						}
					}
				}

				itemCount++
			}

			logging.Info.Println("=== Rebuild Complete ===")
			logging.Info.Printf("Users created: %d", userCount)
			logging.Info.Printf("Personas created: %d", personaCount)
			logging.Info.Printf("Media items restored: %d", itemCount)
			logging.Info.Printf("Jobs enqueued: %d", jobsEnqueued)
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

			sqliteDir := dir(cfg.SQLitePath)
			logging.Info.Printf("SQLITE_PATH dir exists: %v (%s)", dirExists(sqliteDir), sqliteDir)

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
				defer closeDB(database)

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
					database.Unscoped().Model(&models.MediaItem{}).Where("id = ?", itemID).Count(&count)
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
			defer closeDB(database)

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
		if err := server.Shutdown(ctx); err != nil {
			logging.Error.Printf("Server shutdown error: %v", err)
		}
	}
	logging.Info.Println("Shutdown complete")
}

func closeDB(database *db.DB) {
	if err := database.Close(); err != nil {
		logging.Error.Printf("Failed to close database: %v", err)
	}
}

func inferMediaTypeByExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4", ".mov", ".m4v", ".mkv", ".webm", ".avi":
		return models.MediaTypeVideo
	case ".mp3", ".m4a", ".aac", ".wav", ".flac", ".ogg":
		return models.MediaTypeAudio
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return models.MediaTypePhoto
	default:
		return ""
	}
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
	return filepath.Dir(path)
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
