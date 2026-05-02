package tarsnapshot

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// Import extracts archive into targetRoot. Existing files trigger the
// configured ConflictMode.
func (Snapshot) Import(_ context.Context, archive, targetRoot string, mode ports.ConflictMode) error {
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", targetRoot, err)
	}
	f, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("open %q: %w", archive, err)
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip %q: %w", archive, err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if !filepath.IsLocal(hdr.Name) {
			return fmt.Errorf("rejected non-local archive path %q", hdr.Name)
		}
		if err := extractEntry(tr, hdr, targetRoot, mode); err != nil {
			return err
		}
	}
}

func extractEntry(tr *tar.Reader, hdr *tar.Header, targetRoot string, mode ports.ConflictMode) error {
	target := filepath.Join(targetRoot, filepath.FromSlash(hdr.Name))
	resolved, skip, err := resolveConflict(target, hdr, mode)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", filepath.Dir(resolved), err)
	}
	out, err := os.Create(resolved)
	if err != nil {
		return fmt.Errorf("create %q: %w", resolved, err)
	}
	if _, err := io.Copy(out, tr); err != nil {
		_ = out.Close()
		return fmt.Errorf("write %q: %w", resolved, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %q: %w", resolved, err)
	}
	if err := os.Chtimes(resolved, hdr.ModTime, hdr.ModTime); err != nil {
		return fmt.Errorf("chtimes %q: %w", resolved, err)
	}
	return nil
}

func resolveConflict(target string, hdr *tar.Header, mode ports.ConflictMode) (resolved string, skip bool, err error) {
	info, statErr := os.Stat(target)
	if errors.Is(statErr, fs.ErrNotExist) {
		return target, false, nil
	}
	if statErr != nil {
		return "", false, fmt.Errorf("stat %q: %w", target, statErr)
	}
	switch mode {
	case ports.ConflictAbort:
		return "", false, fmt.Errorf("conflict: %q already exists", target)
	case ports.ConflictNewer:
		if hdr.ModTime.After(info.ModTime()) {
			return target, false, nil
		}
		return "", true, nil
	case ports.ConflictManual:
		return target + ".imported", false, nil
	}
	return "", false, fmt.Errorf("unknown conflict mode %d", mode)
}
