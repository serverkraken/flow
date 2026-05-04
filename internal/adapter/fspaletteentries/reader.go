package fspaletteentries

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/textscan"
	"github.com/serverkraken/flow/internal/domain"
)

// Reader reads palette entries from a plugins directory.
type Reader struct {
	pluginsDir  string
	enabledFile string
}

// New constructs a Reader. pluginsDir is the root containing one
// directory per plugin (typically ~/.tmux/plugins); enabledFile is the
// list-of-plugin-names file (typically ~/.tmux/enabled-plugins). Pass
// enabledFile = "" to disable the enabled-plugins lookup and always
// fall back to listing pluginsDir's subdirectories.
func New(pluginsDir, enabledFile string) *Reader {
	return &Reader{pluginsDir: pluginsDir, enabledFile: enabledFile}
}

// List aggregates entries across all enabled plugins (or all subdirs
// of pluginsDir when enabledFile is missing). Order across plugins is
// preserved so the palette renders sections in the intended layout.
//
// A plugin without a menu.entries file is silently skipped (that's the
// expected shape for service plugins like clipboard/nav). Real read or
// scan errors on an EXISTING menu.entries file are joined into the
// returned error so the caller can surface "plugin X has a broken
// menu.entries" instead of seeing it silently disappear from the
// palette.
func (r *Reader) List() ([]domain.PaletteEntry, error) {
	plugins, err := r.enabledPlugins()
	if err != nil {
		return nil, err
	}
	if plugins == nil {
		fallback, ferr := allSubdirs(r.pluginsDir)
		if ferr == nil {
			plugins = fallback
		}
	}

	var (
		entries []domain.PaletteEntry
		errs    []error
		order   = 0
	)
	for _, plugin := range plugins {
		path := filepath.Join(r.pluginsDir, plugin, "menu.entries")
		ee, perr := parseEntriesFile(path, order)
		if perr != nil {
			if !errors.Is(perr, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("%s: %w", plugin, perr))
			}
			continue
		}
		entries = append(entries, ee...)
		order += len(ee)
	}
	return entries, errors.Join(errs...)
}

func (r *Reader) enabledPlugins() ([]string, error) {
	if r.enabledFile == "" {
		return nil, nil
	}
	f, err := os.Open(r.enabledFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var plugins []string
	sc := textscan.New(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		plugins = append(plugins, line)
	}
	return plugins, sc.Err()
}

func allSubdirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// parseEntriesFile reads one plugin's menu.entries file. baseOrder is
// added to each entry's Order so cross-plugin ordering is stable.
func parseEntriesFile(path string, baseOrder int) ([]domain.PaletteEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var entries []domain.PaletteEntry
	i := 0
	sc := textscan.New(f)
	for sc.Scan() {
		entry, ok := parseLine(sc.Text(), baseOrder+i)
		if !ok {
			continue
		}
		entries = append(entries, entry)
		i++
	}
	return entries, sc.Err()
}

func parseLine(raw string, order int) (domain.PaletteEntry, bool) {
	trim := strings.TrimSpace(raw)
	if trim == "" || strings.HasPrefix(trim, "#") {
		return domain.PaletteEntry{}, false
	}
	parts := strings.SplitN(raw, "\t", 5)
	if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
		return domain.PaletteEntry{}, false
	}
	section := "Misc"
	if len(parts) >= 4 && strings.TrimSpace(parts[3]) != "" {
		section = strings.TrimSpace(parts[3])
	}
	keybind := ""
	if len(parts) >= 5 {
		keybind = strings.TrimSpace(parts[4])
	}
	return domain.PaletteEntry{
		Icon:    strings.TrimSpace(parts[0]),
		Label:   strings.TrimSpace(parts[1]),
		Action:  strings.TrimSpace(parts[2]),
		Section: section,
		Keybind: keybind,
		Order:   order,
	}, true
}
