// Package clientcheckout is a per-machine registry mapping a project slug to its
// git checkout root on THIS machine. It is inherently device-local (nothing
// crosses devices). Mirrors internal/clientmachine's file pattern.
package clientcheckout

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Checkouts is the on-disk registry. Roots maps projectSlug → checkoutRoot.
type Checkouts struct {
	Roots map[string]string `json:"roots"`
}

// Get returns the recorded checkout root for slug on this machine.
func (c Checkouts) Get(slug string) (string, bool) {
	r, ok := c.Roots[slug]
	return r, ok
}

// Load reads the registry from os.UserConfigDir()/flow/checkouts.json.
func Load() (Checkouts, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Checkouts{Roots: map[string]string{}}, err
	}
	return LoadFrom(filepath.Join(dir, "flow"))
}

// LoadFrom reads <dir>/checkouts.json. A missing or corrupt file yields an empty
// (non-nil) registry, not an error.
func LoadFrom(dir string) (Checkouts, error) {
	path := filepath.Join(dir, "checkouts.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Checkouts{Roots: map[string]string{}}, nil
		}
		return Checkouts{Roots: map[string]string{}}, err
	}
	var c Checkouts
	if json.Unmarshal(data, &c) != nil || c.Roots == nil {
		return Checkouts{Roots: map[string]string{}}, nil
	}
	return c, nil
}

// Record upserts slug→root in the real config dir. Non-fatal callers ignore the
// error.
func Record(slug, root string) error {
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	return RecordIn(filepath.Join(dir, "flow"), slug, root)
}

// RecordIn upserts slug→root in <dir>/checkouts.json (testable variant).
func RecordIn(dir, slug, root string) error {
	c, err := LoadFrom(dir)
	if err != nil {
		return err
	}
	c.Roots[slug] = root
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	out, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "checkouts.json"), out, 0o600)
}
