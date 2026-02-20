package restore

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type RestoreOptions struct {
	Force        bool
	MediaRoot    string
	SQLitePath   string
	RunRebuildDB bool
}

type RestoreResult struct {
	DBFilesRestored   int
	MetaFilesRestored int
	AvatarsRestored   int
	DerivedRestored   int
	OriginalsRestored int
	TotalFiles        int
	TotalBytes        int64
	Warnings          []string
}

func Restore(snapshotPath string, opts RestoreOptions) (*RestoreResult, error) {
	result := &RestoreResult{
		Warnings: []string{},
	}

	targetRoot := opts.MediaRoot
	if targetRoot == "" {
		return nil, fmt.Errorf("media-root is required")
	}

	if !opts.Force {
		if err := checkTargetEmpty(targetRoot); err != nil {
			return nil, fmt.Errorf("target directory not empty: %w\nUse --force to restore anyway", err)
		}
	}

	snapshotFile, err := os.Open(snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("open snapshot: %w", err)
	}
	defer snapshotFile.Close()

	gzr, err := gzip.NewReader(snapshotFile)
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}

		if header.Name == "snapshot.json" {
			continue
		}

		destPath := filepath.Join(targetRoot, header.Name)

		if err := ensureParentDir(destPath); err != nil {
			return nil, fmt.Errorf("ensure parent dir for %s: %w", header.Name, err)
		}

		if fileExists(destPath) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("skipping existing file: %s", header.Name))
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return nil, fmt.Errorf("create dir %s: %w", header.Name, err)
			}
		case tar.TypeReg:
			if err := extractFile(tr, destPath, header.Size); err != nil {
				return nil, fmt.Errorf("extract file %s: %w", header.Name, err)
			}
			result.TotalBytes += header.Size

			if strings.HasPrefix(header.Name, "db/") {
				result.DBFilesRestored++
			} else if strings.HasPrefix(header.Name, "items/") && strings.Contains(header.Name, "/meta/") {
				result.MetaFilesRestored++
			} else if strings.HasPrefix(header.Name, "avatars/") {
				result.AvatarsRestored++
			} else if strings.HasPrefix(header.Name, "items/") && strings.Contains(header.Name, "/derived/") {
				result.DerivedRestored++
			} else if strings.HasPrefix(header.Name, "items/") && strings.Contains(header.Name, "/original/") {
				result.OriginalsRestored++
			}
		}
	}

	result.TotalFiles = result.DBFilesRestored + result.MetaFilesRestored + result.AvatarsRestored + result.DerivedRestored + result.OriginalsRestored

	return result, nil
}

func checkTargetEmpty(targetRoot string) error {
	entries, err := os.ReadDir(targetRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	nonEmptyDirs := []string{}
	for _, entry := range entries {
		if entry.Name() == "tmp" || entry.Name() == ".git" {
			continue
		}
		nonEmptyDirs = append(nonEmptyDirs, entry.Name())
	}

	if len(nonEmptyDirs) > 0 {
		return fmt.Errorf("directory contains: %s", strings.Join(nonEmptyDirs, ", "))
	}

	return nil
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func extractFile(tr *tar.Reader, destPath string, size int64) error {
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.CopyN(f, tr, size)
	if err != nil && err != io.EOF {
		return err
	}

	return f.Sync()
}
