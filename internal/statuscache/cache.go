// Package statuscache is the client-side tmux status snapshot cache: one atomic
// JSON file with a fetch timestamp so the 5s tick can render without a server
// round-trip, and fall back to the last snapshot (dimmed) when offline.
package statuscache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

const (
	TTL    = 30 * time.Second // fresher than this → render without fetching
	MaxAge = 30 * time.Minute // older than this → segment goes empty (Spec §2)
)

type Entry struct {
	FetchedAt time.Time                `json:"fetchedAt"`
	Status    apiclient.WorktimeStatus `json:"status"`
}

func (e Entry) Fresh(now time.Time) bool   { return now.Sub(e.FetchedAt) < TTL }
func (e Entry) Expired(now time.Time) bool { return now.Sub(e.FetchedAt) > MaxAge }

// Read returns the cached entry; ok=false on a missing OR corrupt file (corrupt
// is treated as "no cache" so a bad write never wedges the segment).
func Read(path string) (Entry, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, false
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		return Entry{}, false
	}
	return e, true
}

// Write atomically persists e (tmp in the same dir + rename); concurrent ticks
// are last-writer-wins, no lock needed.
func Write(path string, e Entry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".worktime-status-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
