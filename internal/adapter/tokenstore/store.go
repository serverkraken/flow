package tokenstore

import (
	"os"
	"path/filepath"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/zalando/go-keyring"
)

// Open returns the keyring store when the OS keyring is usable, otherwise a
// 0600 file store (headless/CI/Linux-without-keyring).
func Open() ports.TokenStore {
	if keyringUsable() {
		return newKeyringStore()
	}
	path, err := defaultFilePath()
	if err != nil {
		// Last resort: a file in the working dir; better than a nil store.
		path = ".flow-token.json"
	}
	return newFileStore(path)
}

// keyringUsable probes the keyring with a throwaway item.
func keyringUsable() bool {
	const probe = "__probe__"
	if err := keyring.Set(keyringService, probe, "1"); err != nil {
		return false
	}
	_ = keyring.Delete(keyringService, probe)
	return true
}

func defaultFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "flow", "token.json"), nil
}
