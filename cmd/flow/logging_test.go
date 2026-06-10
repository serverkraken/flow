package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupLoggingWritesToFile(t *testing.T) {
	dir := t.TempDir()
	closeFn := setupLogging(dir, "warn")
	defer closeFn()

	slog.Warn("test-marker-warn")
	slog.Debug("test-marker-debug")
	closeFn()

	data, err := os.ReadFile(filepath.Join(dir, "flow.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "test-marker-warn") {
		t.Errorf("warn line missing in log file: %q", data)
	}
	if strings.Contains(string(data), "test-marker-debug") {
		t.Errorf("debug line should be filtered at warn level: %q", data)
	}
}

func TestSetupLoggingDebugLevel(t *testing.T) {
	dir := t.TempDir()
	closeFn := setupLogging(dir, "debug")
	defer closeFn()
	slog.Debug("dbg-marker")
	closeFn()
	data, _ := os.ReadFile(filepath.Join(dir, "flow.log"))
	if !strings.Contains(string(data), "dbg-marker") {
		t.Errorf("debug line missing with FLOW_LOG_LEVEL=debug: %q", data)
	}
}
