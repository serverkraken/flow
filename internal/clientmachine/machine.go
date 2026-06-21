// Package clientmachine provides a stable per-device machine identity
// (uuid + hostname) persisted in UserConfigDir/flow/machine.json.
package clientmachine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// Machine holds the stable identity of the local device.
type Machine struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Load returns the machine identity from the default config directory,
// creating and persisting it on first call.
func Load() (Machine, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Machine{}, err
	}
	return LoadFrom(filepath.Join(dir, "flow"))
}

// LoadFrom reads dir/machine.json and returns the Machine stored there.
// If the file is missing, corrupt, or cannot be unmarshalled, a new Machine
// is generated (uuid + hostname), written to dir/machine.json with 0o600
// permissions, and returned. A second call to the same dir returns the same id.
//
// This unexported-by-convention function is package-level exported only so that
// tests can call it with t.TempDir() without touching the real home directory.
func LoadFrom(dir string) (Machine, error) {
	path := filepath.Join(dir, "machine.json")

	data, err := os.ReadFile(path)
	if err == nil {
		var m Machine
		if jsonErr := json.Unmarshal(data, &m); jsonErr == nil && m.ID != "" {
			return m, nil
		}
		// fall through: corrupt or missing id — regenerate
	} else if !errors.Is(err, os.ErrNotExist) {
		return Machine{}, err
	}

	// Generate a new identity.
	m := Machine{
		ID:    uuid.NewString(),
		Label: hostname(),
	}

	if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
		return Machine{}, mkErr
	}
	out, _ := json.Marshal(m)
	if writeErr := os.WriteFile(path, out, 0o600); writeErr != nil {
		return Machine{}, writeErr
	}
	return m, nil
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	return h
}
