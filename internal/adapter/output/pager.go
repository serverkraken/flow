package output

import (
	"fmt"
	"os"
	"strings"

	"github.com/serverkraken/flow/internal/shellsafe"
)

// Pager opens content in a tmux horizontal split running viewer. The
// adapter writes content to a temp file under TMPDIR, builds a
// `bash -c '<viewer> <tmp>; rm <tmp>'` command-line and dispatches it
// through ports.Tmux.SplitWindowH so the temp file is removed once the
// viewer exits. ext fixes the temp file's suffix (e.g. "csv" so the
// viewer picks its syntax mode). Empty ext falls back to "txt".
//
// Production callers: Export (CSV/JSON) and Stats via `less -S`. The
// Brief Markdown path used to run through here against an external
// `glow`; in the glow-migration it switched to an in-process viewer
// overlay so the adapter no longer needs to know about Markdown.
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
	// viewer is interpolated as-is: in production every caller passes a
	// hardcoded constant ("less -S"), so the tokens after the command
	// name are part of that string. Quoting the whole thing would break
	// those args. We instead reject viewer strings that could escape the
	// bash -c context — review finding S2.
	//
	// Check first, before any temp file is created: otherwise an
	// adversarial viewer leaks /tmp/worktime-*.<ext> entries on every
	// rejected call (review follow-up — pager.go leak).
	if !isSafeViewer(viewer) {
		return fmt.Errorf("pager: viewer command %q contains shell metacharacters", viewer)
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

// shellQuote delegates to shellsafe.Quote so the bash-c quoting rule
// lives in exactly one place. Previously duplicated in adapter/editor
// — kept the wrapper here for the rest of the file's call sites to
// stay terse.
func shellQuote(s string) string { return shellsafe.Quote(s) }

// isSafeViewer guards the unquoted viewer interpolation inside Pager's
// `bash -c` command-line. Production callers pass hardcoded constants
// like "less -S"; the same set of metacharacters that would let a
// malicious value escape the bash context is rejected here.
//
// Why: review finding S2 — the viewer parameter is an exported part of
// the ports.Output interface, so a future caller wiring it to env or
// config could otherwise re-introduce shell injection.
//
// The forbidden set is shared with the palette's action filter (see
// internal/shellsafe) — both feed values into bash -c and need the same
// chaining-metacharacter guard. The viewer is stricter because it gets
// interpolated unquoted; the palette wraps its action in quoted args.
func isSafeViewer(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	return shellsafe.UnquotedOK(s)
}
