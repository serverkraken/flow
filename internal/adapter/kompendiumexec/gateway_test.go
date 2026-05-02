package kompendiumexec_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/kompendiumexec"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.KompendiumGateway = (*kompendiumexec.Gateway)(nil)

type call struct {
	name string
	args []string
}

func recorder(t *testing.T, responses ...response) (kompendiumexec.Runner, *[]call) {
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

// realStat returns os.Stat — used for tests that pass real file paths.
func realStat(path string) (os.FileInfo, error) { return os.Stat(path) }

func TestList_ParsesJSON(t *testing.T) {
	body := `[
		{"id":"daily/2026-04-30","type":"daily","date":"2026-04-30"},
		{"id":"proj/flow","type":"project","project":"flow"}
	]`
	r, calls := recorder(t, response{out: []byte(body)})
	g := kompendiumexec.NewWithRunner("kompendium", r, realStat)

	got, err := g.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.KompendiumNote{
		{ID: "daily/2026-04-30", Type: "daily", Date: "2026-04-30"},
		{ID: "proj/flow", Type: "project", Project: "flow"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if len(*calls) != 1 || (*calls)[0].name != "kompendium" ||
		!reflect.DeepEqual((*calls)[0].args, []string{"ls", "--json"}) {
		t.Errorf("calls: %+v", *calls)
	}
}

func TestList_EmptyOutput(t *testing.T) {
	r, _ := recorder(t, response{out: []byte("")})
	g := kompendiumexec.NewWithRunner("kompendium", r, realStat)
	got, err := g.List()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("want nil, got %v", got)
	}

	r, _ = recorder(t, response{out: []byte("   \n\n")})
	g = kompendiumexec.NewWithRunner("kompendium", r, realStat)
	got, _ = g.List()
	if got != nil {
		t.Errorf("whitespace-only: want nil, got %v", got)
	}
}

func TestList_PropagatesError(t *testing.T) {
	r, _ := recorder(t, response{err: errors.New("boom")})
	g := kompendiumexec.NewWithRunner("kompendium", r, realStat)
	_, err := g.List()
	if err == nil || !contains(err.Error(), "kompendium ls:") {
		t.Errorf("expected wrapped error, got %v", err)
	}
}

func TestList_BadJSON(t *testing.T) {
	r, _ := recorder(t, response{out: []byte("{not json")})
	g := kompendiumexec.NewWithRunner("kompendium", r, realStat)
	_, err := g.List()
	if err == nil || !contains(err.Error(), "parse") {
		t.Errorf("expected parse error, got %v", err)
	}
}

func TestResolvePath_TrimsAndReturns(t *testing.T) {
	r, calls := recorder(t, response{out: []byte("/notes/daily/2026-04-30.md\n")})
	g := kompendiumexec.NewWithRunner("kompendium", r, realStat)

	got, err := g.ResolvePath("daily/2026-04-30")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/notes/daily/2026-04-30.md" {
		t.Errorf("got %q", got)
	}
	if !reflect.DeepEqual((*calls)[0].args, []string{"path", "daily/2026-04-30"}) {
		t.Errorf("call: %+v", (*calls)[0])
	}
}

func TestResolvePath_PropagatesError(t *testing.T) {
	want := errors.New("not found")
	r, _ := recorder(t, response{err: want})
	g := kompendiumexec.NewWithRunner("kompendium", r, realStat)

	_, err := g.ResolvePath("missing")
	if !errors.Is(err, want) {
		t.Errorf("got %v, want %v", err, want)
	}
}

func TestResolvePath_HonoursBinName(t *testing.T) {
	r, calls := recorder(t, response{out: []byte("/some/path")})
	g := kompendiumexec.NewWithRunner("/usr/local/bin/komp-dev", r, realStat)
	if _, err := g.ResolvePath("foo"); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].name != "/usr/local/bin/komp-dev" {
		t.Errorf("bin name: %q", (*calls)[0].name)
	}
}

func TestDailyExists_True(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "note.md")
	if err := os.WriteFile(notePath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, _ := recorder(t, response{out: []byte(notePath + "\n")})
	g := kompendiumexec.NewWithRunner("kompendium", r, realStat)

	if !g.DailyExists(time.Date(2026, 4, 30, 0, 0, 0, 0, time.Local)) {
		t.Error("want true")
	}
}

func TestDailyExists_FalseOnPathError(t *testing.T) {
	r, _ := recorder(t, response{err: errors.New("no")})
	g := kompendiumexec.NewWithRunner("kompendium", r, realStat)
	if g.DailyExists(time.Now()) {
		t.Error("path error: want false")
	}
}

func TestDailyExists_FalseOnEmptyPath(t *testing.T) {
	r, _ := recorder(t, response{out: []byte("\n")})
	g := kompendiumexec.NewWithRunner("kompendium", r, realStat)
	if g.DailyExists(time.Now()) {
		t.Error("empty path: want false")
	}
}

func TestDailyExists_FalseOnStatError(t *testing.T) {
	r, _ := recorder(t, response{out: []byte("/missing/path\n")})
	stat := func(string) (os.FileInfo, error) { return nil, fs.ErrNotExist }
	g := kompendiumexec.NewWithRunner("kompendium", r, stat)
	if g.DailyExists(time.Now()) {
		t.Error("stat error: want false")
	}
}

func TestNew_DefaultStat(t *testing.T) {
	g := kompendiumexec.New("kompendium")
	if g == nil {
		t.Fatal("New returned nil")
	}
}

func TestNewWithRunner_NilStatFallsBackToOSStat(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "note.md")
	if err := os.WriteFile(notePath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	r, _ := recorder(t, response{out: []byte(notePath + "\n")})
	g := kompendiumexec.NewWithRunner("kompendium", r, nil)
	if !g.DailyExists(time.Now()) {
		t.Error("default stat fallback should report existing file")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
