package output

import (
	"fmt"
	"os"
	"strings"
)

// Pager opens content in a tmux horizontal split running viewer. The
// adapter writes content to a temp file under TMPDIR, builds a
// `bash -c '<viewer> <tmp>; rm <tmp>'` command-line and dispatches it
// through ports.Tmux.SplitWindowH so the temp file is removed once the
// viewer exits. ext fixes the temp file's suffix (e.g. "md" so glow
// auto-detects markdown). Empty ext falls back to "txt".
//
// On any failure path after the temp file is created, the adapter
// removes it before returning so a botched dispatch never leaves
// dangling /tmp/worktime-*.<ext> files behind.
func (t *Targets) Pager(content, viewer, ext string) error {
	if ext == "" {
		ext = "txt"
	}
	if strings.TrimSpace(viewer) == "" {
		return fmt.Errorf("pager: viewer command is empty")
	}
	f, err := os.CreateTemp("", "worktime-*."+ext)
	if err != nil {
		return fmt.Errorf("pager temp-file: %w", err)
	}
	tmpPath := f.Name()
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("pager temp-file write: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("pager temp-file close: %w", err)
	}
	cmdline := fmt.Sprintf("%s %s; rm %s",
		viewer, shellQuote(tmpPath), shellQuote(tmpPath))
	if err := t.tmux.SplitWindowH("bash", "-c", cmdline); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("pager tmux split: %w", err)
	}
	return nil
}

// shellQuote wraps s in single quotes, escaping embedded single quotes
// via the standard '\” bash idiom. Used by Pager to compose a safe
// `bash -c` command-line that survives spaces / quotes in TMPDIR.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
