package output

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SaveFile writes content to <home>/Downloads/<basename>-<ts>.<ext>
// and returns the absolute path. The timestamp (yyyy-mm-dd-HHMMSS,
// resolution to the second) makes overwrites of an earlier export
// effectively impossible — calling SaveFile twice in the same second
// is the only collision case and we let os.WriteFile overwrite there
// since the user just dispatched the same export twice in a row and
// would expect the latest.
//
// The Downloads directory is created with 0o755 if it doesn't exist,
// so a fresh checkout / clean test environment doesn't fail on the
// first export.
func (t *Targets) SaveFile(basename, ext string, content []byte) (string, error) {
	if ext == "" {
		ext = "txt"
	}
	dir := filepath.Join(t.home, "Downloads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("savefile mkdir %s: %w", dir, err)
	}
	ts := time.Now().Format("2006-01-02-150405")
	name := fmt.Sprintf("%s-%s.%s", basename, ts, ext)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", fmt.Errorf("savefile write %s: %w", path, err)
	}
	return path, nil
}
