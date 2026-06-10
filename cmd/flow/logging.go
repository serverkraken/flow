package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// setupLogging routes the process-global slog default handler into
// <stateDir>/flow.log. The flow TUI owns the terminal via bubbletea —
// any stderr write from a background goroutine (httpsync worker, keyring
// warnings) would corrupt the alternate-screen render, so NOTHING may log
// to stderr after this call. Falls back to io.Discard when the state dir
// is not writable. Returns a close func for the log file (call via defer).
func setupLogging(stateDir, level string) func() {
	var w io.Writer = io.Discard
	closeFn := func() {}
	if err := os.MkdirAll(stateDir, 0o755); err == nil {
		f, ferr := os.OpenFile(filepath.Join(stateDir, "flow.log"),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if ferr == nil {
			w = f
			closeFn = func() { _ = f.Close() }
		}
	}
	lvl := slog.LevelWarn
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl})))
	return closeFn
}
