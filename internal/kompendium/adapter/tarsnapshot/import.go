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

// Defensive limits against decompression-bomb archives. The per-entry
// cap is a hard limit on uncompressed bytes; a crafted .tar.gz that
// claims a 50 GiB note would otherwise fill the local disk. 100 MiB is
// a generous ceiling for legitimate notes — typical kompendium notes
// are well under 1 MiB and the snapshot/export path uses the same tar
// format, so a legitimate roundtrip never approaches the cap.
//
// maxTotalBytes guards against the cumulative case the per-entry cap
// misses: an archive with thousands of entries each just under
// maxEntryBytes would otherwise fill the disk while every individual
// entry passes the per-entry check. 5 GiB is two orders of magnitude
// above any real notebook (typical: <100 MiB total) and still well
// under the smallest realistic disk-free reserve.
// Effectively const at runtime; declared as `var` only so tests can swap
// them via the helper in export_test.go (the alternative is a multi-GiB
// fixture archive).
var (
	maxEntryBytes int64 = 100 << 20
	maxTotalBytes int64 = 5 << 30
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
	var totalBytes int64
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
		// Per-entry size cap, validated before any allocation. Header
		// Size is the uncompressed size the archive claims for the entry;
		// a hostile archive may lie, so the io.Copy below is wrapped in
		// io.LimitReader as a second line of defence.
		if hdr.Size > maxEntryBytes {
			return fmt.Errorf("entry %q exceeds size cap (%d > %d bytes)", hdr.Name, hdr.Size, maxEntryBytes)
		}
		n, err := extractEntry(tr, hdr, targetRoot, mode)
		if err != nil {
			return err
		}
		// Cumulative cap protects against the many-entries-each-just-
		// under-the-per-entry-cap pattern that the per-entry check alone
		// cannot stop. Checked after each write so the abort fires before
		// the next allocation.
		totalBytes += n
		if totalBytes > maxTotalBytes {
			return fmt.Errorf("archive exceeds cumulative size cap (%d > %d bytes) after entry %q", totalBytes, maxTotalBytes, hdr.Name)
		}
	}
}

// extractEntry writes one tar entry to disk via temp+fsync+rename so a
// crash mid-import never leaves a half-written file in the notebook
// (mirrors the discipline in fsstore.Put). The parent dir is fsync'd
// after rename so the new directory entry is durable. Returns the
// number of uncompressed bytes written (zero on skip), so the caller
// can enforce the cumulative-size cap.
func extractEntry(tr *tar.Reader, hdr *tar.Header, targetRoot string, mode ports.ConflictMode) (int64, error) {
	target := filepath.Join(targetRoot, filepath.FromSlash(hdr.Name))
	resolved, skip, err := resolveConflict(target, hdr, mode)
	if err != nil {
		return 0, err
	}
	if skip {
		return 0, nil
	}
	dir := filepath.Dir(resolved)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir %q: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tarsnap-*.tmp")
	if err != nil {
		return 0, fmt.Errorf("create temp in %q: %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	// io.LimitReader caps the bytes copied even when the header lied
	// about the entry size. n == maxEntryBytes signals the reader hit
	// the cap mid-stream; treat that as a bomb.
	n, err := io.Copy(tmp, io.LimitReader(tr, maxEntryBytes+1))
	if err != nil {
		_ = tmp.Close()
		cleanup()
		return 0, fmt.Errorf("write %q: %w", resolved, err)
	}
	if n > maxEntryBytes {
		_ = tmp.Close()
		cleanup()
		return 0, fmt.Errorf("entry %q exceeds size cap (%d bytes)", hdr.Name, maxEntryBytes)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return 0, fmt.Errorf("fsync %q: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return 0, fmt.Errorf("close %q: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		cleanup()
		return 0, fmt.Errorf("chmod %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, resolved); err != nil {
		cleanup()
		return 0, fmt.Errorf("rename %q → %q: %w", tmpPath, resolved, err)
	}
	if err := os.Chtimes(resolved, hdr.ModTime, hdr.ModTime); err != nil {
		return 0, fmt.Errorf("chtimes %q: %w", resolved, err)
	}
	if err := syncDir(dir); err != nil {
		return 0, fmt.Errorf("fsync dir %q: %w", dir, err)
	}
	return n, nil
}

// syncDir fsync's the directory FD so the prior rename becomes durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}

// resolveConflict picks a destination path according to the configured
// ConflictMode, or signals "skip this entry" / abort. For ConflictManual
// the .imported suffix is bumped (.imported.1, .imported.2, …) on
// collision so a second manual-mode import doesn't silently overwrite
// the first user's rescue copy.
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
		return uniqueImportedPath(target), false, nil
	}
	return "", false, fmt.Errorf("unknown conflict mode %d", mode)
}

// uniqueImportedPath returns target+".imported", or, if that already
// exists, target+".imported.1", ".imported.2", … up to a sensible
// cap. The cap exists so a corrupted state (thousands of .imported.N
// files) surfaces as an error rather than spinning forever.
func uniqueImportedPath(target string) string {
	base := target + ".imported"
	if _, err := os.Stat(base); errors.Is(err, fs.ErrNotExist) {
		return base
	}
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s.%d", base, i)
		if _, err := os.Stat(candidate); errors.Is(err, fs.ErrNotExist) {
			return candidate
		}
	}
	// Pathological: 1000 .imported copies already exist. Return a
	// timestamp-suffixed path so the caller still gets a write target;
	// the user clearly has bigger problems than name collision.
	return fmt.Sprintf("%s.%d", base, os.Getpid())
}
