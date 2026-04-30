// Package palette implements the palette screen: a searchable, grouped list of
// all actions aggregated from enabled plugins' menu.entries files.
package palette

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/serverkraken/tui-kit/tmuxbridge"
)

// Entry is a single palette action loaded from a plugin's menu.entries file.
type Entry struct {
	Icon    string
	Label   string
	Action  string
	Section string
	Keybind string
	order   int
}

// Section display priority — lower index = appears first.
// Unknown sections land after all known ones.
var sectionRank = func() map[string]int {
	order := []string{
		"Favoriten", "Sidekick", "Kompendium", "Worktime", "Git", "Navigation", "System", "Misc",
	}
	m := make(map[string]int, len(order))
	for i, s := range order {
		m[s] = i
	}
	return m
}()

func rankOf(section string) int {
	if r, ok := sectionRank[section]; ok {
		return r
	}
	return len(sectionRank)
}

// LoadEntries reads all menu.entries from enabled plugins and returns entries
// alongside the persisted stats, sorted by effective section priority,
// recency-weighted score, and input order. Stats are loaded best-effort —
// missing or unreadable files yield a zero-state Stats and a stable order.
func LoadEntries() ([]Entry, *Stats, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, err
	}
	pluginsDir := filepath.Join(home, ".tmux", "plugins")

	plugins, err := tmuxbridge.EnabledPlugins()
	if err != nil {
		return nil, nil, err
	}
	// Fallback: scan all subdirs when enabled-plugins file is absent.
	if plugins == nil {
		plugins, err = subdirs(pluginsDir)
		if err != nil {
			plugins = nil
		}
	}

	var entries []Entry
	order := 0
	for _, plugin := range plugins {
		path := filepath.Join(pluginsDir, plugin, "menu.entries")
		ee, err := parseEntriesFile(path, order)
		if err != nil {
			continue
		}
		entries = append(entries, ee...)
		order += len(ee)
	}

	stats := LoadStats()
	SortEntries(entries, stats)
	return entries, stats, nil
}

// SortEntries sorts entries in-place by effective section rank (pin →
// Favoriten), then score descending (recents/freq boost), then by original
// input order as the stable tiebreaker.
func SortEntries(entries []Entry, stats *Stats) {
	sort.SliceStable(entries, func(i, j int) bool {
		si := stats.EffectiveSection(entries[i])
		sj := stats.EffectiveSection(entries[j])
		ri, rj := rankOf(si), rankOf(sj)
		if ri != rj {
			return ri < rj
		}
		scoreI, scoreJ := stats.score(entries[i]), stats.score(entries[j])
		if !nearlyEqual(scoreI, scoreJ) {
			return scoreI > scoreJ
		}
		return entries[i].order < entries[j].order
	})
}

// parseEntriesFile reads a menu.entries TSV file.
// Schema: icon\tlabel\taction[\tsection[\tkeybind]]
// Lines starting with '#' and blank lines are ignored.
func parseEntriesFile(path string, baseOrder int) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var entries []Entry
	i := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		raw := sc.Text()
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		parts := strings.SplitN(raw, "\t", 5)
		if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
			continue
		}
		section := "Misc"
		if len(parts) >= 4 && strings.TrimSpace(parts[3]) != "" {
			section = strings.TrimSpace(parts[3])
		}
		keybind := ""
		if len(parts) >= 5 {
			keybind = strings.TrimSpace(parts[4])
		}
		entries = append(entries, Entry{
			Icon:    strings.TrimSpace(parts[0]),
			Label:   strings.TrimSpace(parts[1]),
			Action:  strings.TrimSpace(parts[2]),
			Section: section,
			Keybind: keybind,
			order:   baseOrder + i,
		})
		i++
	}
	return entries, sc.Err()
}

func subdirs(dir string) ([]string, error) {
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
