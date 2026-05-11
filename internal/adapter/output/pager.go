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
	// viewer is interpolated as-is: in production every caller passes a
	// hardcoded constant ("glow", "less -S"), so the tokens after the
	// command name are part of that string. Quoting the whole thing
	// would break those args. We instead reject viewer strings that
	// could escape the bash -c context — review finding S2.
	if !isSafeViewer(viewer) {
		return fmt.Errorf("pager: viewer command %q contains shell metacharacters", viewer)
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

// isSafeViewer guards the unquoted viewer interpolation inside Pager's
// `bash -c` command-line. Production callers pass hardcoded constants
// like "glow" or "less -S"; the same set of metacharacters that would
// let a malicious value escape the bash context is rejected here.
//
// Why: review finding S2 — the viewer parameter is an exported part of
// the ports.Output interface, so a future caller wiring it to env or
// config could otherwise re-introduce shell injection.
func isSafeViewer(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	if strings.ContainsAny(s, ";|&`\n\r<>'\"") {
		return false
	}
	if strings.Contains(s, "$(") || strings.Contains(s, "${") {
		return false
	}
	return true
}
