package nvimeditor_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/adapter/nvimeditor"
)

// External tests exercise realRun against POSIX no-op binaries — true exits
// 0, false exits 1 — so coverage hits the actual exec path without
// requiring nvim.

// t.Setenv cannot run alongside t.Parallel — these two cases mutate process
// env so they stay serial.

func TestEdit_RealRunSucceeds(t *testing.T) {
	if _, err := exec.LookPath("true"); err != nil {
		t.Skipf("/bin/true not on PATH: %v", err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "true")

	if err := nvimeditor.New().Edit(context.Background(), "/some/path"); err != nil {
		t.Errorf("Edit with /bin/true should succeed, got %v", err)
	}
}

func TestEdit_RealRunFails(t *testing.T) {
	if _, err := exec.LookPath("false"); err != nil {
		t.Skipf("/bin/false not on PATH: %v", err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "false")

	err := nvimeditor.New().Edit(context.Background(), "/some/path")
	if err == nil {
		t.Error("expected error from failing editor")
	}
}
