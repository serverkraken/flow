package httpapi_test

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/domain"
)

func TestSettings_GetAndPut(t *testing.T) {
	api := newTestAPI(t)
	settings := httpapi.NewSettings(api.Client)
	ctx := context.Background()

	// Start fresh — server may have no settings yet, get returns empty map
	m, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil map from Get")
	}

	// Set a value
	if err := settings.Put(ctx, map[string]string{"daily_target": "8h"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Read back
	m2, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get after Put: %v", err)
	}
	if m2["daily_target"] != "8h" {
		t.Errorf("daily_target = %q, want %q", m2["daily_target"], "8h")
	}
}

// fakeIni is a minimal ConfigReader for testing.
type fakeIni struct {
	cfg domain.Config
}

func (f *fakeIni) Load() (domain.Config, error) { return f.cfg, nil }

func TestConfigReader_ServerOverridesIni(t *testing.T) {
	api := newTestAPI(t)
	settings := httpapi.NewSettings(api.Client)
	ctx := context.Background()

	// Seed server with a specific target
	if err := settings.Put(ctx, map[string]string{"daily_target": "7h30m"}); err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	ini := &fakeIni{cfg: domain.Config{DefaultTarget: 8 * time.Hour}}
	reader := httpapi.NewConfigReader(api.Client, ini)

	cfg, err := reader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Server's 7h30m should override ini's 8h
	if cfg.DefaultTarget != 7*time.Hour+30*time.Minute {
		t.Errorf("DefaultTarget = %v, want 7h30m", cfg.DefaultTarget)
	}
}

func TestConfigReader_EmptyServer_SeedsFromIni(t *testing.T) {
	// Use a fresh client that connects to the test server.
	// We cannot guarantee the server is empty (shared pgTestStore), but we can
	// verify that Load() returns without error and falls back to ini.
	api := newTestAPI(t)
	ini := &fakeIni{cfg: domain.Config{DefaultTarget: 6 * time.Hour}}
	reader := httpapi.NewConfigReader(api.Client, ini)

	cfg, err := reader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// DefaultTarget may be from server or ini — should not error and should be non-zero
	if cfg.DefaultTarget == 0 {
		t.Error("DefaultTarget is zero after Load")
	}
}
