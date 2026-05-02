package cli_test

import (
	"errors"
	"strings"
	"testing"
)

func TestExport_HappyPath(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	stdout, _, err := runCmd(t, env.deps, "export", "/tmp/snap.tar.gz")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(stdout, "/tmp/snap.tar.gz") {
		t.Errorf("output got %q", stdout)
	}
	if len(env.tar.Exports) != 1 {
		t.Errorf("FakeTarSnapshot.Exports got %+v", env.tar.Exports)
	}
}

func TestExport_AdapterError(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	forced := errors.New("forced export err")
	env.tar.ExportErr = forced

	_, _, err := runCmd(t, env.deps, "export", "/tmp/x")
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}

func TestExport_BundleMode(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	stdout, _, err := runCmd(t, env.deps, "export", "/tmp/snap.bundle", "--bundle")
	if err != nil {
		t.Fatalf("export --bundle: %v", err)
	}
	if !strings.Contains(stdout, "(bundle)") {
		t.Errorf("output should mark bundle mode, got %q", stdout)
	}
	if len(env.bundle.Exports) != 1 || len(env.tar.Exports) != 0 {
		t.Errorf("bundle export not routed to bundle adapter: tar=%+v bundle=%+v", env.tar.Exports, env.bundle.Exports)
	}
}

func TestExport_BundleAdapterError(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	forced := errors.New("forced bundle err")
	env.bundle.ExportErr = forced

	_, _, err := runCmd(t, env.deps, "export", "/tmp/snap.bundle", "--bundle")
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}
