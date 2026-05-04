package editor_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/editor"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.NoteLauncher = (*editor.Launcher)(nil)

type call struct {
	name string
	args []string
}

func recorder(t *testing.T, responses ...response) (editor.Runner, *[]call) {
	t.Helper()
	var calls []call
	idx := 0
	r := func(name string, args ...string) ([]byte, error) {
		calls = append(calls, call{name: name, args: append([]string(nil), args...)})
		if idx >= len(responses) {
			return nil, nil
		}
		resp := responses[idx]
		idx++
		return resp.out, resp.err
	}
	return r, &calls
}

type response struct {
	out []byte
	err error
}

// staticPath returns a canned path for a known ID and "" for everything else.
// pathOf "" signals the launcher to refuse rather than spawn the editor on a
// blank path, so most tests pre-load a path for the ID they pass.
func staticPath(idToPath map[string]string) editor.PathFunc {
	return func(id string) string { return idToPath[id] }
}

// nvimArgs is the production-shape EditorArgsFunc most tests use: returns
// argv {nvim, path} so the launcher's tmux split picks up nvim + path
// just like the kompendium nvimeditor adapter would.
func nvimArgs(path string) ([]string, error) {
	return []string{"nvim", path}, nil
}

func TestOpen_EmptyID(t *testing.T) {
	r, _ := recorder(t)
	l := editor.NewWithRunner(staticPath(nil), nvimArgs, "glow", r)
	if err := l.Open("   "); err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestOpen_UnresolvablePath(t *testing.T) {
	r, _ := recorder(t)
	l := editor.NewWithRunner(staticPath(nil), nvimArgs, "glow", r)
	if err := l.Open("daily/missing"); err == nil {
		t.Fatal("want error when pathOf returns empty path")
	}
}

func TestOpen_SpawnsTmuxSplit(t *testing.T) {
	r, calls := recorder(t)
	l := editor.NewWithRunner(
		staticPath(map[string]string{"daily/2026-04-30": "/notes/daily/2026-04-30.md"}),
		nvimArgs, "glow", r,
	)
	if err := l.Open("daily/2026-04-30"); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0].name != "tmux" {
		t.Fatalf("calls: %+v", *calls)
	}
	// argv tokens are single-quote-escaped and joined into one shell
	// command — see joinShellArgv. tmux runs split-window's trailing
	// arg through /bin/sh -c, so quoting is required to keep paths
	// with spaces / shell metachars from re-splitting.
	want := []string{"split-window", "-h", "'nvim' '/notes/daily/2026-04-30.md'"}
	if !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("args: got %v, want %v", (*calls)[0].args, want)
	}
}

// TestOpen_HonoursEditorArgsFlags covers the case where editorArgs
// returns extra flags (e.g. "code -w" or "nvim -O"). The launcher
// must pass them through to tmux split-window verbatim — without
// flag passthrough, the user's editor preferences are silently
// ignored.
func TestOpen_HonoursEditorArgsFlags(t *testing.T) {
	r, calls := recorder(t)
	editorArgs := func(path string) ([]string, error) {
		return []string{"nvim", "-O", path}, nil
	}
	l := editor.NewWithRunner(
		staticPath(map[string]string{"daily/2026-04-30": "/p.md"}),
		editorArgs, "glow", r,
	)
	if err := l.Open("daily/2026-04-30"); err != nil {
		t.Fatal(err)
	}
	want := []string{"split-window", "-h", "'nvim' '-O' '/p.md'"}
	if !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("args: got %v, want %v", (*calls)[0].args, want)
	}
}

// TestOpen_PathWithSpaces_GetsQuoted verifies the regression that
// motivated the quoting refactor: a path containing a space used to
// reach /bin/sh as `nvim /notes/My Daily.md` and split into three args.
func TestOpen_PathWithSpaces_GetsQuoted(t *testing.T) {
	r, calls := recorder(t)
	l := editor.NewWithRunner(
		staticPath(map[string]string{"x": "/notes/My Daily.md"}),
		nvimArgs, "glow", r,
	)
	if err := l.Open("x"); err != nil {
		t.Fatal(err)
	}
	want := []string{"split-window", "-h", "'nvim' '/notes/My Daily.md'"}
	if !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("args: got %v, want %v", (*calls)[0].args, want)
	}
}

// TestOpen_PathWithSingleQuote escapes inner quotes via the POSIX
// '\\” sequence so the shell still reads it as one argument.
func TestOpen_PathWithSingleQuote(t *testing.T) {
	r, calls := recorder(t)
	l := editor.NewWithRunner(
		staticPath(map[string]string{"x": "/n/it's-fine.md"}),
		nvimArgs, "glow", r,
	)
	if err := l.Open("x"); err != nil {
		t.Fatal(err)
	}
	want := []string{"split-window", "-h", `'nvim' '/n/it'\''s-fine.md'`}
	if !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("args: got %v, want %v", (*calls)[0].args, want)
	}
}

func TestOpen_ResolveEditorErrorWrapped(t *testing.T) {
	r, _ := recorder(t)
	want := errors.New("env parse")
	editorArgs := func(string) ([]string, error) { return nil, want }
	l := editor.NewWithRunner(
		staticPath(map[string]string{"x": "/p"}),
		editorArgs, "glow", r,
	)
	err := l.Open("x")
	if err == nil || !errors.Is(err, want) {
		t.Errorf("got %v, want wrapped %v", err, want)
	}
}

func TestOpen_EmptyEditorArgs(t *testing.T) {
	r, _ := recorder(t)
	l := editor.NewWithRunner(
		staticPath(map[string]string{"x": "/p"}),
		func(string) ([]string, error) { return nil, nil },
		"glow", r,
	)
	if err := l.Open("x"); err == nil {
		t.Fatal("expected error on empty editor argv")
	}
}

func TestOpen_PropagatesTmuxError(t *testing.T) {
	want := errors.New("split failed")
	r, _ := recorder(t, response{err: want})
	l := editor.NewWithRunner(
		staticPath(map[string]string{"daily": "/p"}),
		nvimArgs, "glow", r,
	)
	if err := l.Open("daily"); !errors.Is(err, want) {
		t.Errorf("got %v, want %v", err, want)
	}
}

func TestView_EmptyID(t *testing.T) {
	r, _ := recorder(t)
	l := editor.NewWithRunner(staticPath(nil), nvimArgs, "glow", r)
	if err := l.View("   "); err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestView_UnresolvablePath(t *testing.T) {
	r, _ := recorder(t)
	l := editor.NewWithRunner(staticPath(nil), nvimArgs, "glow", r)
	if err := l.View("missing"); err == nil {
		t.Fatal("want error when pathOf returns empty path")
	}
}

func TestView_OpensViewerOnSplit(t *testing.T) {
	r, calls := recorder(t)
	l := editor.NewWithRunner(
		staticPath(map[string]string{"daily": "/notes/daily.md"}),
		nvimArgs, "glow", r,
	)
	if err := l.View("daily"); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatalf("want 1 call, got %d: %+v", len(*calls), *calls)
	}
	// noteViewer is pre-tokenised at construction; each argv token is
	// single-quote-escaped at exec time so a hostile viewer cannot
	// inject extra commands via tmux's sh -c.
	want := []string{"split-window", "-h", "'glow' '/notes/daily.md'"}
	if (*calls)[0].name != "tmux" || !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("split call: %+v", (*calls)[0])
	}
}

func TestView_HonoursCustomViewer(t *testing.T) {
	r, calls := recorder(t)
	l := editor.NewWithRunner(
		staticPath(map[string]string{"x": "/p"}),
		nvimArgs, "bat --paging=always", r,
	)
	if err := l.View("x"); err != nil {
		t.Fatal(err)
	}
	want := []string{"split-window", "-h", "'bat' '--paging=always' '/p'"}
	if (*calls)[0].name != "tmux" || !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("split call: %+v", (*calls)[0])
	}
}

// TestView_PathWithSpaces ensures the path is quoted alongside the
// pre-tokenised viewer argv.
func TestView_PathWithSpaces(t *testing.T) {
	r, calls := recorder(t)
	l := editor.NewWithRunner(
		staticPath(map[string]string{"x": "/notes/My Daily.md"}),
		nvimArgs, "glow", r,
	)
	if err := l.View("x"); err != nil {
		t.Fatal(err)
	}
	want := []string{"split-window", "-h", "'glow' '/notes/My Daily.md'"}
	if (*calls)[0].name != "tmux" || !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("split call: %+v", (*calls)[0])
	}
}

// TestView_RejectsShellInjectionInViewer verifies that a hostile
// $FLOW_NOTE_VIEWER like `glow; rm -rf $HOME` is tokenised, not
// interpreted by sh — the `;` becomes part of the first argv token,
// the `rm` and `$HOME` become literal arguments to that bogus binary.
func TestView_RejectsShellInjectionInViewer(t *testing.T) {
	r, calls := recorder(t)
	l := editor.NewWithRunner(
		staticPath(map[string]string{"x": "/p"}),
		nvimArgs, "glow; rm -rf $HOME", r,
	)
	if err := l.View("x"); err != nil {
		t.Fatal(err)
	}
	want := []string{"split-window", "-h", `'glow;' 'rm' '-rf' '$HOME' '/p'`}
	if (*calls)[0].name != "tmux" || !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("split call: got %v, want %v", (*calls)[0].args, want)
	}
}

func TestView_PropagatesSplitError(t *testing.T) {
	splitErr := errors.New("no tmux")
	r, _ := recorder(t, response{err: splitErr})
	l := editor.NewWithRunner(
		staticPath(map[string]string{"note": "/p"}),
		nvimArgs, "glow", r,
	)
	if err := l.View("note"); !errors.Is(err, splitErr) {
		t.Errorf("got %v, want %v", err, splitErr)
	}
}

func TestNew_ProductionConstructor(t *testing.T) {
	l := editor.New(staticPath(nil), nvimArgs, "glow")
	if l == nil {
		t.Fatal("New returned nil")
	}
}
