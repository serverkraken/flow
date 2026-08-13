package tokenstore

import (
	"os"
	"path/filepath"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/zalando/go-keyring"
)

var userConfigDir = os.UserConfigDir

// Open returns the keyring store when the OS keyring is usable, otherwise a
// 0600 file store (headless/CI/Linux-without-keyring).
func Open() ports.TokenStore {
	lockPath, err := defaultLockPath()
	if err != nil {
		lockPath = ".flow-token.lock"
	}
	if keyringUsable() {
		return newLockedStore(lockPath, newKeyringStore())
	}
	path, err := defaultFilePath()
	if err != nil {
		// Last resort: a file in the working dir; better than a nil store.
		path = ".flow-token.json"
	}
	return newLockedStore(lockPath, newFileStore(path))
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
	dir, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "flow", "token.json"), nil
}

func defaultLockPath() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "flow", "token.lock"), nil
}

var _ ports.TokenStore = (*lockedStore)(nil)
var _ ports.TokenStoreSession = (*fileStore)(nil)
var _ ports.TokenStoreSession = (*keyringStore)(nil)
