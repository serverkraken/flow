package iniconfig

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/textscan"
	"github.com/serverkraken/flow/internal/domain"
)

// Reader loads worktime configuration from a key=value file plus the
// WORKTIME_TARGET_HOURS env var fallback.
type Reader struct {
	path string
}

// New constructs a Reader that reads from path on each Load. The file
// is not required to exist.
func New(path string) *Reader {
	return &Reader{path: path}
}

const defaultTargetHours = 8

var weekdayKeys = map[string]time.Weekday{
	"target_mon": time.Monday,
	"target_tue": time.Tuesday,
	"target_wed": time.Wednesday,
	"target_thu": time.Thursday,
	"target_fri": time.Friday,
	"target_sat": time.Saturday,
	"target_sun": time.Sunday,
}

// Load returns the merged configuration. Missing file → defaults; an
// existing-but-unreadable file is surfaced as an error so the caller can
// distinguish "first launch" from "broken file".
func (r *Reader) Load() (domain.Config, error) {
	cfg := domain.Config{
		PerWeekday: map[time.Weekday]time.Duration{},
		TagTargets: map[string]time.Duration{},
	}
	fileDef, err := r.readFile(&cfg)
	if err != nil {
		return cfg, err
	}
	cfg.DefaultTarget = resolveDefault(fileDef)
	return cfg, nil
}

// readFile populates per-weekday + tag overrides and returns the parsed
// `target_hours` value (0 if absent). Per-line parse failures are
// tolerated silently — historically the config has been hand-edited.
func (r *Reader) readFile(cfg *domain.Config) (time.Duration, error) {
	f, err := os.Open(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close() //nolint:errcheck

	var def time.Duration
	sc := textscan.New(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// max_streak_min is an integer (minutes), not hours-as-float
		// like the other keys — handle it before the float parse.
		if key == "max_streak_min" {
			if n, err := strconv.Atoi(val); err == nil && n >= 0 {
				cfg.MaxStreakMin = n
			}
			continue
		}
		h, err := strconv.ParseFloat(val, 64)
		if err != nil || h < 0 {
			continue
		}
		dur := time.Duration(h * float64(time.Hour))
		switch {
		case key == "target_hours":
			def = dur
		case strings.HasPrefix(key, "tag_target_"):
			if tag := strings.TrimPrefix(key, "tag_target_"); tag != "" {
				cfg.TagTargets[tag] = dur
			}
		default:
			if wd, ok := weekdayKeys[key]; ok {
				cfg.PerWeekday[wd] = dur
			}
		}
	}
	return def, sc.Err()
}

// resolveDefault picks the highest-priority source: file > env > 8h.
func resolveDefault(fileVal time.Duration) time.Duration {
	if fileVal > 0 {
		return fileVal
	}
	if v := os.Getenv("WORKTIME_TARGET_HOURS"); v != "" {
		if h, err := strconv.ParseFloat(v, 64); err == nil && h > 0 {
			return time.Duration(h * float64(time.Hour))
		}
	}
	return defaultTargetHours * time.Hour
}
