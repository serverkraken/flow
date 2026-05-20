package cli_test

// Drives the --watch path of `flow worktime status`. The production
// loop ticks every 60s; we cancel the context immediately so the loop's
// `<-ctx.Done()` branch fires on the very next iteration. Without this
// the watch branch in newStatusCmd stayed uncovered.

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/frontend/cli"
)

func TestStatus_WatchExitsOnContextCancel(t *testing.T) {
	f := newFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := cli.NewWorktimeCmd(f.deps())
	cmd.SetArgs([]string{"status", "--watch"})
	cmd.SetContext(ctx)
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("status --watch with cancelled ctx should exit cleanly, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("status --watch did not honour context cancellation within 2s")
	}
}
