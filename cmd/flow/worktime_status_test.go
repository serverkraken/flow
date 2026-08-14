package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/statuscache"
	"github.com/serverkraken/flow/internal/statusline"
)

func TestRenderStatus_FreshCacheSkipsFetch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "worktime-status.json")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.Local)
	_ = statuscache.Write(p, statuscache.Entry{FetchedAt: now.Add(-10 * time.Second),
		Status: apiclient.WorktimeStatus{Date: "2026-07-08", LoggedMin: 480, TargetMin: 480}})
	triggered := false
	out := renderStatus(now, p, func() { triggered = true }, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if triggered {
		t.Error("fresh cache must not schedule refresh")
	}
	if !strings.Contains(out, "08:00") {
		t.Errorf("expected rendered segment, got %q", out)
	}
}

func TestRenderStatus_StaleRendersDim(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "worktime-status.json")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.Local)
	base := statusline.DefaultStatusPalette()
	_ = statuscache.Write(p, statuscache.Entry{FetchedAt: now.Add(-2 * time.Minute),
		Status: apiclient.WorktimeStatus{Date: "2026-07-08", LoggedMin: 480, TargetMin: 480}})
	triggered := false
	out := renderStatus(now, p, func() { triggered = true }, statusRenderOpts{Palette: base})
	if !triggered {
		t.Error("stale cache must schedule refresh")
	}
	if strings.Contains(out, base.Green) {
		t.Errorf("stale render must be dim, got %q", out)
	}
	if out == "" || !strings.Contains(out, base.Dim) {
		t.Errorf("stale render should still show a dim segment, got %q", out)
	}
}

func TestRenderStatus_ExpiredEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "worktime-status.json")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.Local)
	_ = statuscache.Write(p, statuscache.Entry{FetchedAt: now.Add(-40 * time.Minute), Status: apiclient.WorktimeStatus{Date: "2026-07-08", LoggedMin: 480}})
	triggered := false
	out := renderStatus(now, p, func() { triggered = true }, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if !triggered {
		t.Error("expired cache must schedule refresh")
	}
	if out != "" {
		t.Errorf("expired cache should render empty, got %q", out)
	}
}

// Finding #8: a corrupt on-disk cache reads as "no cache"; offline → empty.
func TestRenderStatus_CorruptCacheOfflineEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "worktime-status.json")
	_ = os.WriteFile(p, []byte("{garbage"), 0o644)
	triggered := false
	out := renderStatus(time.Now(), p, func() { triggered = true }, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if !triggered {
		t.Error("corrupt cache must schedule refresh")
	}
	if out != "" {
		t.Errorf("corrupt cache + offline → empty, got %q", out)
	}
}

func TestRenderStatus_ColdCacheSchedulesRefreshAndReturnsEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "worktime-status.json")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.Local)
	triggered := false
	out := renderStatus(now, p, func() { triggered = true }, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if !triggered {
		t.Error("cold cache must schedule refresh")
	}
	if out != "" {
		t.Errorf("cold cache must return immediately without rendering, got %q", out)
	}
}

func TestRefreshStatusCache_ColdFetchWritesCache(t *testing.T) {
	p := filepath.Join(t.TempDir(), "worktime-status.json")
	want := apiclient.WorktimeStatus{Date: "2026-07-08", LoggedMin: 480, TargetMin: 480}
	if err := refreshStatusCache(context.Background(), p, func(context.Context) (apiclient.WorktimeStatus, error) {
		return want, nil
	}); err != nil {
		t.Fatal(err)
	}
	if e, ok := statuscache.Read(p); !ok || e.Status.LoggedMin != want.LoggedMin {
		t.Errorf("background refresh must persist the cache, got ok=%v %+v", ok, e)
	}
}

func TestRefreshStatusCache_RechecksFreshCacheInsideWorkerLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "worktime-status.json")
	if err := statuscache.Write(p, statuscache.Entry{
		FetchedAt: time.Now(),
		Status:    apiclient.WorktimeStatus{LoggedMin: 42},
	}); err != nil {
		t.Fatal(err)
	}
	fetched := false
	if err := refreshStatusCache(context.Background(), p, func(context.Context) (apiclient.WorktimeStatus, error) {
		fetched = true
		return apiclient.WorktimeStatus{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if fetched {
		t.Fatal("worker must not fetch after another worker refreshed the cache")
	}
}

// The real render command must schedule background work, exit zero, and stay
// empty on a cold cache without touching keychain or network itself.
func TestWorktimeStatusCmd_ColdCacheExitZeroEmpty(t *testing.T) {
	t.Setenv("FLOW_TOKEN", "dummy")
	t.Setenv("FLOW_SERVER_URL", "http://127.0.0.1:1")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	originalSpawn := spawnWorktimeStatusRefresh
	spawned := false
	spawnWorktimeStatusRefresh = func() error {
		spawned = true
		return nil
	}
	t.Cleanup(func() { spawnWorktimeStatusRefresh = originalSpawn })
	cmd := worktimeStatusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status must exit 0 offline, got %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("no cache + offline → empty, got %q", buf.String())
	}
	if !spawned {
		t.Error("cold cache must schedule the detached refresh worker")
	}
}
