package main

import (
	"bytes"
	"context"
	"errors"
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
	fetched := false
	out := renderStatus(context.Background(), now, p, func(context.Context) (apiclient.WorktimeStatus, error) {
		fetched = true
		return apiclient.WorktimeStatus{}, nil
	}, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if fetched {
		t.Error("fresh cache must not fetch")
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
	out := renderStatus(context.Background(), now, p, func(context.Context) (apiclient.WorktimeStatus, error) {
		return apiclient.WorktimeStatus{}, errors.New("offline")
	}, statusRenderOpts{Palette: base})
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
	out := renderStatus(context.Background(), now, p, func(context.Context) (apiclient.WorktimeStatus, error) {
		return apiclient.WorktimeStatus{}, errors.New("offline")
	}, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if out != "" {
		t.Errorf("expired cache should render empty, got %q", out)
	}
}

// Finding #8: a corrupt on-disk cache reads as "no cache"; offline → empty.
func TestRenderStatus_CorruptCacheOfflineEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "worktime-status.json")
	_ = os.WriteFile(p, []byte("{garbage"), 0o644)
	out := renderStatus(context.Background(), time.Now(), p, func(context.Context) (apiclient.WorktimeStatus, error) {
		return apiclient.WorktimeStatus{}, errors.New("offline")
	}, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if out != "" {
		t.Errorf("corrupt cache + offline → empty, got %q", out)
	}
}

// Finding C10: cold cache-miss → successful fetch writes the cache AND renders.
func TestRenderStatus_ColdFetchWritesCacheAndRenders(t *testing.T) {
	p := filepath.Join(t.TempDir(), "worktime-status.json")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.Local)
	out := renderStatus(context.Background(), now, p, func(context.Context) (apiclient.WorktimeStatus, error) {
		return apiclient.WorktimeStatus{Date: "2026-07-08", LoggedMin: 480, TargetMin: 480}, nil
	}, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if !strings.Contains(out, "08:00") {
		t.Errorf("cold fetch should render, got %q", out)
	}
	if e, ok := statuscache.Read(p); !ok || e.Status.LoggedMin != 480 {
		t.Errorf("cold fetch must persist the cache, got ok=%v %+v", ok, e)
	}
}

// Finding C10: no cache at all + offline → empty (no panic).
func TestRenderStatus_NoCacheOfflineEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "absent.json")
	out := renderStatus(context.Background(), time.Now(), p, func(context.Context) (apiclient.WorktimeStatus, error) {
		return apiclient.WorktimeStatus{}, errors.New("offline")
	}, statusRenderOpts{Palette: statusline.DefaultStatusPalette()})
	if out != "" {
		t.Errorf("no cache + offline → empty, got %q", out)
	}
}

// Finding #2: exercise the REAL command (clientFromStore + GetWorktimeStatus +
// offline path) end-to-end — deterministically offline via an unreachable
// server. Must exit 0 (RunE returns nil) and render empty; NEVER prompt/hang.
func TestWorktimeStatusCmd_OfflineExitZeroEmpty(t *testing.T) {
	t.Setenv("FLOW_TOKEN", "dummy")                   // static bearer → no keychain touched
	t.Setenv("FLOW_SERVER_URL", "http://127.0.0.1:1") // refused port → 2s fetch fails fast
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("TMUX", "")
	cmd := worktimeStatusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status must exit 0 offline, got %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("no cache + offline → empty, got %q", buf.String())
	}
}
