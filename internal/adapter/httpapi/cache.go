package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// resourceCache hält die letzte gute Server-Antwort einer Resource und
// persistiert sie als Teil des Offline-Snapshots (Spec §8). Invalidate()
// erzwingt beim nächsten Read einen Refetch.
type resourceCache[T any] struct {
	mu    sync.Mutex
	val   T
	ok    bool
	stale bool
}

func (r *resourceCache[T]) get() (T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.ok || r.stale {
		var zero T
		return zero, false
	}
	return r.val, true
}

func (r *resourceCache[T]) put(v T) {
	r.mu.Lock()
	r.val, r.ok, r.stale = v, true, false
	r.mu.Unlock()
}

func (r *resourceCache[T]) invalidate() { r.mu.Lock(); r.stale = true; r.mu.Unlock() }

// fallback liefert den letzten bekannten Wert auch wenn stale — für
// Offline-Reads (Server weg => lieber alt als nichts, UI zeigt das Banner).
func (r *resourceCache[T]) fallback() (T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.val, r.ok
}

// Snapshot ist die Platte-Repräsentation unter $XDG_STATE_HOME/flow/snapshot.json.
type Snapshot struct {
	FetchedAt time.Time         `json:"fetched_at"`
	Sessions  []sessionDTO      `json:"sessions"`
	Active    []activeDTO       `json:"active"`
	Projects  []projectDTO      `json:"projects"`
	DayOffs   []dayOffDTO       `json:"day_offs"`
	Settings  map[string]string `json:"settings"`
}

func snapshotPath() string {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "flow", "snapshot.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "flow", "snapshot.json")
}

func saveSnapshot(s Snapshot) error {
	p := snapshotPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func loadSnapshot() (Snapshot, bool) {
	b, err := os.ReadFile(snapshotPath())
	if err != nil {
		return Snapshot{}, false
	}
	var s Snapshot
	if json.Unmarshal(b, &s) != nil {
		return Snapshot{}, false
	}
	return s, true
}
