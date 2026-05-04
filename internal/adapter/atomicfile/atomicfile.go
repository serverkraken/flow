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
// end. When the append created the file (path did not exist before),
// the parent directory is also fsync'd so the new directory entry is
// durable. Subsequent appends skip the dir-sync since the entry already
// exists — this keeps the steady-state cost at one fsync per call.
func Append(path string, data []byte, perm os.FileMode) error {
	_, statErr := os.Stat(path)
	created := os.IsNotExist(statErr)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, perm)
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
