package iniconfig_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/iniconfig"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.ConfigReader = (*iniconfig.Reader)(nil)

func TestLoad_MissingFile_Default8h(t *testing.T) {
	// Pass 0 → adapter falls back to its own 8h baseline. This is the
	// shape the composition root uses when $WORKTIME_TARGET_HOURS is
	// unset (review finding A1 — env-resolution moved to main.go).
	r := iniconfig.New(filepath.Join(t.TempDir(), "missing.conf"), 0)
	cfg, err := r.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultTarget != 8*time.Hour {
		t.Errorf("DefaultTarget: want 8h, got %v", cfg.DefaultTarget)
	}
	if len(cfg.PerWeekday) != 0 {
		t.Errorf("PerWeekday: want empty, got %v", cfg.PerWeekday)
	}
	if len(cfg.TagTargets) != 0 {
		t.Errorf("TagTargets: want empty, got %v", cfg.TagTargets)
	}
}

func TestLoad_MissingFile_HonorsExplicitDefault(t *testing.T) {
	// Composition root resolved $WORKTIME_TARGET_HOURS=6 into 6h and
	// passed it in. The reader uses it because the file has no
	// `target_hours` key.
	r := iniconfig.New(filepath.Join(t.TempDir(), "missing.conf"), 6*time.Hour)
	cfg, err := r.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultTarget != 6*time.Hour {
		t.Errorf("DefaultTarget: want 6h, got %v", cfg.DefaultTarget)
	}
}

func TestLoad_MissingFile_NegativeDefaultFallsBackTo8h(t *testing.T) {
	// Defensive: a negative or zero default is treated as "use the
	// hardcoded baseline" — the adapter shouldn't echo back nonsense.
	r := iniconfig.New(filepath.Join(t.TempDir(), "missing.conf"), -3*time.Hour)
	cfg, _ := r.Load()
	if cfg.DefaultTarget != 8*time.Hour {
		t.Errorf("negative default: want 8h, got %v", cfg.DefaultTarget)
	}
}

func TestLoad_FullFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "worktime.conf")
	body := "" +
		"# leading comment\n" +
		"\n" +
		"target_hours = 7.5\n" +
		"target_mon = 8\n" +
		"target_tue = 8\n" +
		"target_fri = 6\n" +
		"target_sat = 0\n" +
		"target_sun = 0\n" +
		"tag_target_deep = 4\n" +
		"tag_target_admin = 2.5\n" +
		"# inline-style note\n" +
		"malformed line without equals\n" +
		"target_unknown = 10\n" + // unknown key, ignored
		"target_mon = -3\n" + // negative, ignored — keeps prior 8h
		"tag_target_ = 99\n" + // empty tag, ignored
		"target_wed = bogus\n" // bad value, ignored
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := iniconfig.New(path, 0).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultTarget != 7*time.Hour+30*time.Minute {
		t.Errorf("DefaultTarget: want 7h30m, got %v", cfg.DefaultTarget)
	}
	wantPerDay := map[time.Weekday]time.Duration{
		time.Monday:   8 * time.Hour,
		time.Tuesday:  8 * time.Hour,
		time.Friday:   6 * time.Hour,
		time.Saturday: 0,
		time.Sunday:   0,
	}
	if len(cfg.PerWeekday) != len(wantPerDay) {
		t.Errorf("PerWeekday: want %d keys, got %d (%v)", len(wantPerDay), len(cfg.PerWeekday), cfg.PerWeekday)
	}
	for wd, want := range wantPerDay {
		if got := cfg.PerWeekday[wd]; got != want {
			t.Errorf("PerWeekday[%v]: want %v, got %v", wd, want, got)
		}
	}
	if cfg.TagTargets["deep"] != 4*time.Hour {
		t.Errorf("tag deep: want 4h, got %v", cfg.TagTargets["deep"])
	}
	if cfg.TagTargets["admin"] != 2*time.Hour+30*time.Minute {
		t.Errorf("tag admin: want 2h30m, got %v", cfg.TagTargets["admin"])
	}
	if _, ok := cfg.TagTargets[""]; ok {
		t.Errorf("empty tag should be skipped")
	}
}

func TestLoad_FilePrecedenceOverDefault(t *testing.T) {
	// Composition root passed 12h in (e.g. from $WORKTIME_TARGET_HOURS),
	// but the file's `target_hours = 6` wins.
	dir := t.TempDir()
	path := filepath.Join(dir, "conf")
	if err := os.WriteFile(path, []byte("target_hours = 6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := iniconfig.New(path, 12*time.Hour).Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultTarget != 6*time.Hour {
		t.Errorf("file precedence: want 6h, got %v", cfg.DefaultTarget)
	}
}

func TestLoad_OpenError(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	r := iniconfig.New(filepath.Join(regular, "child"), 0)
	_, err := r.Load()
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("want non-NotExist error, got %v", err)
	}
}
