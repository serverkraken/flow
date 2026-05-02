package tarsnapshot

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Export writes a tar.gz of every regular file under sourceRoot (except
// anything under .git/) to outPath.
func (Snapshot) Export(_ context.Context, sourceRoot, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %q: %w", outPath, err)
	}
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	defer func() { _ = gz.Close() }()

	tw := tar.NewWriter(gz)
	defer func() { _ = tw.Close() }()

	walkErr := filepath.WalkDir(sourceRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return fmt.Errorf("rel %q: %w", path, err)
		}
		if rel == "." {
			return nil
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			// Skip symlinks, devices, sockets, named pipes; archive only regular files.
			return nil
		}
		return writeFileEntry(tw, sourceRoot, path, rel, d)
	})
	if walkErr != nil {
		return fmt.Errorf("walk %q: %w", sourceRoot, walkErr)
	}
	return nil
}

func writeFileEntry(tw *tar.Writer, _ string, path, rel string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return fmt.Errorf("stat %q: %w", path, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}
	hdr := &tar.Header{
		Name:    filepath.ToSlash(rel),
		Mode:    0o644,
		Size:    int64(len(content)),
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write header %q: %w", rel, err)
	}
	if _, err := tw.Write(content); err != nil {
		return fmt.Errorf("write body %q: %w", rel, err)
	}
	return nil
}
