// Package atomicfile centralises the temp+fsync+rename pattern used by every
// adapter that persists user state. The pattern is correct only when the
// rename is followed by an fsync of the parent directory — without it, POSIX
// allows the directory-entry update to roll back on crash even though the
// file's data was synced. This package fsyncs both file and directory.
package atomicfile

import (
	"os"
	"path/filepath"
)

// WriteFile atomically replaces path with data. The new content is written to
// a sibling temp file, fsync'd, then renamed over path; finally the parent
// directory is fsync'd so the rename itself becomes durable.
//
// Directory creation is the caller's responsibility — adapters typically
// MkdirAll first because they want to control mode bits.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return SyncDir(filepath.Dir(path))
}

// Append writes data to path in O_APPEND mode, fsync'ing the file at the
// end. The parent directory is fsync'd whenever path was created by
// THIS open — an O_EXCL probe distinguishes that from a concurrent
// process having created it just before us, closing the TOCTOU window
// where two parallel Stat-then-Open paths would each conclude "not
// created here" and both skip the dir-sync.
func Append(path string, data []byte, perm os.FileMode) error {
	created := false
	probe, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err == nil {
		created = true
		// We won the create race; close and reopen in append mode below.
		_ = probe.Close()
	} else if !os.IsExist(err) {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if created {
		return SyncDir(filepath.Dir(path))
	}
	return nil
}

// SyncDir fsync's the directory at dir. Errors from Open and Sync are
// returned; on filesystems that don't support directory fsync (rare; some
// network filesystems), Sync may return an error that callers can choose
// to log rather than fail on, but this package returns it verbatim — the
// safer default.
func SyncDir(dir string) error {
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
