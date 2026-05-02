package cli_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

func TestImport_HappyPath(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	stdout, _, err := runCmd(t, env.deps, "import", "/tmp/snap.tar.gz")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !strings.Contains(stdout, "/tmp/snap.tar.gz") {
		t.Errorf("output got %q", stdout)
	}
	if len(env.tar.Imports) != 1 || env.tar.Imports[0].Mode != ports.ConflictAbort {
		t.Errorf("Imports got %+v", env.tar.Imports)
	}
}

func TestImport_NewerMode(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	if _, _, err := runCmd(t, env.deps, "import", "/tmp/snap.tar.gz", "--on-conflict", "newer"); err != nil {
		t.Fatal(err)
	}
	if env.tar.Imports[0].Mode != ports.ConflictNewer {
		t.Errorf("Mode got %v", env.tar.Imports[0].Mode)
	}
}

func TestImport_ManualMode(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	if _, _, err := runCmd(t, env.deps, "import", "/tmp/snap.tar.gz", "--on-conflict", "manual"); err != nil {
		t.Fatal(err)
	}
	if env.tar.Imports[0].Mode != ports.ConflictManual {
		t.Errorf("Mode got %v", env.tar.Imports[0].Mode)
	}
}

func TestImport_BadMode(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	_, _, err := runCmd(t, env.deps, "import", "/tmp/x", "--on-conflict", "wrong")
	if err == nil {
		t.Fatal("expected error for bad --on-conflict")
	}
}

func TestImport_AdapterError(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	forced := errors.New("forced import err")
	env.tar.ImportErr = forced

	_, _, err := runCmd(t, env.deps, "import", "/tmp/snap.tar.gz")
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}

func TestImport_BundleMode(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	stdout, _, err := runCmd(t, env.deps, "import", "/tmp/snap.bundle", "--bundle")
	if err != nil {
		t.Fatalf("import --bundle: %v", err)
	}
	if !strings.Contains(stdout, "(bundle)") {
		t.Errorf("output should mark bundle mode, got %q", stdout)
	}
	if len(env.bundle.Imports) != 1 || len(env.tar.Imports) != 0 {
		t.Errorf("bundle import not routed correctly: tar=%+v bundle=%+v", env.tar.Imports, env.bundle.Imports)
	}
}

func TestImport_BundleAdapterError(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	forced := errors.New("forced import bundle err")
	env.bundle.ImportErr = forced

	_, _, err := runCmd(t, env.deps, "import", "/tmp/snap.bundle", "--bundle")
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}
