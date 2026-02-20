package snapshot

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	SchemaVersion = 1
	ManifestFile  = "snapshot.json"
)

type SnapshotOptions struct {
	IncludeMedia     bool
	IncludeDerived   bool
	IncludeOriginals bool
	IncludeAvatars   bool
	IncludeDB        bool
	IncludeMeta      bool
}

type SnapshotManifest struct {
	Schema             int       `json:"schema"`
	LunaVersion        string    `json:"luna_version,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	MediaRootLayoutVer int       `json:"media_root_layout_version"`
	Options            Options   `json:"options"`
	Counts             Counts    `json:"counts"`
}

type Options struct {
	IncludeMedia     bool `json:"include_media"`
	IncludeDerived   bool `json:"include_derived"`
	IncludeOriginals bool `json:"include_originals"`
	IncludeAvatars   bool `json:"include_avatars"`
	IncludeDB        bool `json:"include_db"`
	IncludeMeta      bool `json:"include_meta"`
}

type Counts struct {
	DBFiles    int   `json:"db_files"`
	MetaFiles  int   `json:"meta_files"`
	Avatars    int   `json:"avatars"`
	Derived    int   `json:"derived"`
	Originals  int   `json:"originals"`
	TotalFiles int   `json:"total_files"`
	TotalBytes int64 `json:"total_bytes"`
}

type SnapshotResult struct {
	Manifest   SnapshotManifest
	OutputPath string
}

func Snapshot(mediaRoot, sqlitePath, outputPath string, opts SnapshotOptions) (*SnapshotResult, error) {
	// Set defaults
	if !opts.IncludeDerived && !opts.IncludeOriginals && !opts.IncludeMedia {
		// Default: include DB, meta, avatars
		opts.IncludeDB = true
		opts.IncludeMeta = true
		opts.IncludeAvatars = true
	}

	if opts.IncludeMedia {
		opts.IncludeDerived = true
		opts.IncludeOriginals = true
	}

	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("create output file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	// Create gzip writer
	gzw := gzip.NewWriter(outFile)
	defer func() { _ = gzw.Close() }()

	// Create tar writer
	tw := tar.NewWriter(gzw)
	defer func() { _ = tw.Close() }()

	// Track counts
	counts := Counts{}

	// Get base paths
	dbDir := filepath.Dir(sqlitePath)
	dbBase := filepath.Base(sqlitePath)

	// Add DB files (include WAL and SHM if present)
	if opts.IncludeDB {
		dbFiles := []string{dbBase}
		// Check for WAL/SHM files
		if _, err := os.Stat(sqlitePath + "-wal"); err == nil {
			dbFiles = append(dbFiles, dbBase+"-wal")
		}
		if _, err := os.Stat(sqlitePath + "-shm"); err == nil {
			dbFiles = append(dbFiles, dbBase+"-shm")
		}

		for _, f := range dbFiles {
			fullPath := filepath.Join(dbDir, f)
			if _, err := os.Stat(fullPath); err == nil {
				if err := addFileToTar(tw, fullPath, filepath.Join("db", f), &counts); err != nil {
					return nil, fmt.Errorf("add db file %s: %w", f, err)
				}
				counts.DBFiles++
			}
		}
	}

	// Add meta files
	if opts.IncludeMeta {
		itemsDir := filepath.Join(mediaRoot, "items")
		entries, err := os.ReadDir(itemsDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read items dir: %w", err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			itemID := entry.Name()
			metaDir := filepath.Join(itemsDir, itemID, "meta")

			// Add item.json
			itemMetaPath := filepath.Join(metaDir, "item.json")
			if _, err := os.Stat(itemMetaPath); err == nil {
				relPath := filepath.Join("items", itemID, "meta", "item.json")
				if err := addFileToTar(tw, itemMetaPath, relPath, &counts); err != nil {
					return nil, fmt.Errorf("add item meta %s: %w", itemID, err)
				}
				counts.MetaFiles++
			}

			// Add assets.json
			assetsMetaPath := filepath.Join(metaDir, "assets.json")
			if _, err := os.Stat(assetsMetaPath); err == nil {
				relPath := filepath.Join("items", itemID, "meta", "assets.json")
				if err := addFileToTar(tw, assetsMetaPath, relPath, &counts); err != nil {
					return nil, fmt.Errorf("add assets meta %s: %w", itemID, err)
				}
				counts.MetaFiles++
			}
		}
	}

	// Add avatars
	if opts.IncludeAvatars {
		avatarsDir := filepath.Join(mediaRoot, "avatars")
		if err := addDirToTar(tw, avatarsDir, "avatars", &counts); err != nil {
			return nil, fmt.Errorf("add avatars: %w", err)
		}
	}

	// Add derived files
	if opts.IncludeDerived {
		itemsDir := filepath.Join(mediaRoot, "items")
		entries, err := os.ReadDir(itemsDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read items dir: %w", err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			itemID := entry.Name()
			derivedDir := filepath.Join(itemsDir, itemID, "derived")
			thumbsDir := filepath.Join(itemsDir, itemID, "thumbs")

			// Add derived/*
			if err := addDirToTar(tw, derivedDir, filepath.Join("items", itemID, "derived"), &counts); err != nil {
				return nil, fmt.Errorf("add derived %s: %w", itemID, err)
			}

			// Add thumbs/*
			if err := addDirToTar(tw, thumbsDir, filepath.Join("items", itemID, "thumbs"), &counts); err != nil {
				return nil, fmt.Errorf("add thumbs %s: %w", itemID, err)
			}
		}
	}

	// Add original files
	if opts.IncludeOriginals {
		itemsDir := filepath.Join(mediaRoot, "items")
		entries, err := os.ReadDir(itemsDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read items dir: %w", err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			itemID := entry.Name()
			originalDir := filepath.Join(itemsDir, itemID, "original")

			if err := addDirToTar(tw, originalDir, filepath.Join("items", itemID, "original"), &counts); err != nil {
				return nil, fmt.Errorf("add originals %s: %w", itemID, err)
			}
		}
	}

	counts.TotalFiles = counts.DBFiles + counts.MetaFiles + counts.Avatars + counts.Derived + counts.Originals

	// Create and write manifest
	manifest := SnapshotManifest{
		Schema:             SchemaVersion,
		CreatedAt:          time.Now(),
		MediaRootLayoutVer: SchemaVersion,
		Options:            Options(opts),
		Counts:             counts,
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	// Add manifest to tar
	if err := tw.WriteHeader(&tar.Header{
		Name: ManifestFile,
		Mode: 0644,
		Size: int64(len(manifestData)),
	}); err != nil {
		return nil, fmt.Errorf("write manifest header: %w", err)
	}

	if _, err := tw.Write(manifestData); err != nil {
		return nil, fmt.Errorf("write manifest data: %w", err)
	}

	// Close tar to flush
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar: %w", err)
	}

	// Get final file size
	stat, err := outFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat output: %w", err)
	}
	counts.TotalBytes = stat.Size()

	return &SnapshotResult{
		Manifest:   manifest,
		OutputPath: outputPath,
	}, nil
}

// addFileToTar adds a single file to the tar archive
func addFileToTar(tw *tar.Writer, fullPath, relPath string, counts *Counts) error {
	f, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	counts.TotalBytes += stat.Size()

	header, err := tar.FileInfoHeader(stat, "")
	if err != nil {
		return err
	}
	header.Name = relPath

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tw, f)
	return err
}

// addDirToTar recursively adds all files in a directory to the tar archive
func addDirToTar(tw *tar.Writer, dirPath, prefix string, counts *Counts) error {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return nil
	}

	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}

		// Determine what category this file belongs to
		if strings.Contains(prefix, "derived") {
			counts.Derived++
		} else if strings.Contains(prefix, "original") {
			counts.Originals++
		} else if strings.Contains(prefix, "avatars") {
			counts.Avatars++
		}

		archivePath := filepath.Join(prefix, relPath)
		return addFileToTar(tw, path, archivePath, counts)
	})
}

// ReadManifest reads a snapshot manifest from an archive
func ReadManifest(snapshotPath string) (*SnapshotManifest, error) {
	f, err := os.Open(snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("open snapshot: %w", err)
	}
	defer func() { _ = f.Close() }()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}

		if header.Name == ManifestFile {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("read manifest: %w", err)
			}

			var manifest SnapshotManifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				return nil, fmt.Errorf("unmarshal manifest: %w", err)
			}

			return &manifest, nil
		}
	}

	return nil, fmt.Errorf("manifest not found in snapshot")
}
