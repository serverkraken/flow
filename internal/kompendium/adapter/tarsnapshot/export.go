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
//
// Close errors on the tar / gzip writers and the file are checked
// explicitly: a deferred-discard would silently produce a corrupt
// archive (incomplete tar trailer or gzip footer) that the matching
// Import would later reject as "unexpected EOF". The file is also
// fsync'd before close so a crash between the last write and the OS
// flush can't truncate the just-written archive.
func (Snapshot) Export(_ context.Context, sourceRoot, outPath string) (retErr error) {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %q: %w", outPath, err)
	}
	// Belt-and-suspenders: if we return early via error, still close the
	// file so the fd doesn't leak. The successful path closes it
	// explicitly below to inspect the error.
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

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
	if err := tw.Close(); err != nil {
		return fmt.Errorf("tar close: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("gzip close: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync %q: %w", outPath, err)
	}
	closed = true
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %q: %w", outPath, err)
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
