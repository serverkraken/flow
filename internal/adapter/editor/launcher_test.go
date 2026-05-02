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

func TestOpen_EmptyID(t *testing.T) {
	r, _ := recorder(t)
	l := editor.NewWithRunner("kompendium", "glow", r)
	err := l.Open("")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestOpen_ShellsTmuxSplit(t *testing.T) {
	r, calls := recorder(t)
	l := editor.NewWithRunner("kompendium", "glow", r)
	if err := l.Open("daily/2026-04-30"); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0].name != "tmux" {
		t.Fatalf("calls: %+v", *calls)
	}
	want := []string{"split-window", "-h", "kompendium", "open", "daily/2026-04-30"}
	if !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("args: got %v, want %v", (*calls)[0].args, want)
	}
}

func TestOpen_PropagatesError(t *testing.T) {
	want := errors.New("split failed")
	r, _ := recorder(t, response{err: want})
	l := editor.NewWithRunner("kompendium", "glow", r)
	if err := l.Open("daily"); !errors.Is(err, want) {
		t.Errorf("got %v, want %v", err, want)
	}
}

func TestView_EmptyID(t *testing.T) {
	r, _ := recorder(t)
	l := editor.NewWithRunner("kompendium", "glow", r)
	if err := l.View("   "); err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestView_PathErrorWrapped(t *testing.T) {
	want := errors.New("not found")
	r, _ := recorder(t, response{err: want})
	l := editor.NewWithRunner("kompendium", "glow", r)
	err := l.View("missing")
	if err == nil || !errors.Is(err, want) {
		t.Errorf("got %v, want wrapped %v", err, want)
	}
}

func TestView_EmptyPath(t *testing.T) {
	r, _ := recorder(t, response{out: []byte("\n")})
	l := editor.NewWithRunner("kompendium", "glow", r)
	if err := l.View("note"); err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestView_OpensViewerOnSplit(t *testing.T) {
	r, calls := recorder(t,
		response{out: []byte("/notes/daily.md\n")}, // kompendium path
		response{}, // tmux split
	)
	l := editor.NewWithRunner("kompendium", "glow", r)
	if err := l.View("daily"); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 {
		t.Fatalf("want 2 calls, got %d: %+v", len(*calls), *calls)
	}
	pathCall := (*calls)[0]
	if pathCall.name != "kompendium" ||
		!reflect.DeepEqual(pathCall.args, []string{"path", "daily"}) {
		t.Errorf("path call: %+v", pathCall)
	}
	splitCall := (*calls)[1]
	want := []string{"split-window", "-h", "glow", "/notes/daily.md"}
	if splitCall.name != "tmux" || !reflect.DeepEqual(splitCall.args, want) {
		t.Errorf("split call: %+v", splitCall)
	}
}

func TestView_HonoursCustomViewer(t *testing.T) {
	r, calls := recorder(t,
		response{out: []byte("/p")},
		response{},
	)
	l := editor.NewWithRunner("kompendium", "bat --paging=always", r)
	if err := l.View("x"); err != nil {
		t.Fatal(err)
	}
	if (*calls)[1].args[2] != "bat --paging=always" {
		t.Errorf("viewer not propagated: %+v", (*calls)[1].args)
	}
}

func TestView_PropagatesSplitError(t *testing.T) {
	splitErr := errors.New("no tmux")
	r, _ := recorder(t,
		response{out: []byte("/p")},
		response{err: splitErr},
	)
	l := editor.NewWithRunner("kompendium", "glow", r)
	err := l.View("note")
	if !errors.Is(err, splitErr) {
		t.Errorf("got %v, want %v", err, splitErr)
	}
}

func TestNew_ProductionConstructor(t *testing.T) {
	l := editor.New("kompendium", "glow")
	if l == nil {
		t.Fatal("New returned nil")
	}
}
