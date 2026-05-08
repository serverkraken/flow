package tmuxbridge_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/tmuxbridge"
	"github.com/serverkraken/flow/internal/ports"
)

var _ ports.Tmux = (*tmuxbridge.Bridge)(nil)

// recorder builds a Runner that captures every call and returns canned
// responses keyed by arg-prefix match. The matcher is the simplest form
// that fits these tests; expand if you need richer fakes.
type call struct {
	name string
	args []string
}

func recorder(t *testing.T, responses ...response) (tmuxbridge.Runner, *[]call) {
	t.Helper()
	var calls []call
	idx := 0
	r := func(name string, args ...string) ([]byte, error) {
		argsCopy := append([]string(nil), args...)
		calls = append(calls, call{name: name, args: argsCopy})
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

func TestRefreshClient(t *testing.T) {
	r, calls := recorder(t)
	if err := tmuxbridge.NewWithRunner(r).RefreshClient(); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || (*calls)[0].name != "tmux" ||
		!reflect.DeepEqual((*calls)[0].args, []string{"refresh-client", "-S"}) {
		t.Errorf("calls: %+v", *calls)
	}
}

func TestRefreshClient_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	r, _ := recorder(t, response{err: want})
	got := tmuxbridge.NewWithRunner(r).RefreshClient()
	if !errors.Is(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestShowOption_TrimsAndAddsAt(t *testing.T) {
	r, calls := recorder(t, response{out: []byte("  red  \n")})
	got := tmuxbridge.NewWithRunner(r).ShowOption("tn_red")
	if got != "red" {
		t.Errorf("ShowOption: got %q", got)
	}
	if !reflect.DeepEqual((*calls)[0].args, []string{"show-options", "-gqv", "@tn_red"}) {
		t.Errorf("call: %+v", (*calls)[0])
	}
}

func TestShowOption_ErrorYieldsEmpty(t *testing.T) {
	r, _ := recorder(t, response{err: errors.New("no server")})
	if got := tmuxbridge.NewWithRunner(r).ShowOption("tn_red"); got != "" {
		t.Errorf("want empty on error, got %q", got)
	}
}

func TestCurrentSessionName(t *testing.T) {
	r, calls := recorder(t, response{out: []byte("flow\n")})
	if got := tmuxbridge.NewWithRunner(r).CurrentSessionName(); got != "flow" {
		t.Errorf("got %q", got)
	}
	if !reflect.DeepEqual((*calls)[0].args, []string{"display-message", "-p", "#{session_name}"}) {
		t.Errorf("call: %+v", (*calls)[0])
	}
}

func TestCurrentSessionName_ErrorYieldsEmpty(t *testing.T) {
	r, _ := recorder(t, response{err: errors.New("outside tmux")})
	if got := tmuxbridge.NewWithRunner(r).CurrentSessionName(); got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

func TestListSessions_MultiLine(t *testing.T) {
	r, _ := recorder(t, response{out: []byte("flow\nproj-a\nproj-b\n")})
	got, err := tmuxbridge.NewWithRunner(r).ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"flow", "proj-a", "proj-b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestListSessions_EmptyStdout(t *testing.T) {
	r, _ := recorder(t, response{out: []byte("")})
	got, err := tmuxbridge.NewWithRunner(r).ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("want nil on empty stdout, got %v", got)
	}
}

func TestListSessions_PropagatesError(t *testing.T) {
	want := errors.New("no server")
	r, _ := recorder(t, response{err: want})
	_, err := tmuxbridge.NewWithRunner(r).ListSessions()
	if !errors.Is(err, want) {
		t.Errorf("got %v, want %v", err, want)
	}
}

func TestHasSession(t *testing.T) {
	// HasSession is now ListSessions-based — searches the list for the name.
	r, _ := recorder(t, response{out: []byte("flow\nfoo\n")})
	if !tmuxbridge.NewWithRunner(r).HasSession("flow") {
		t.Error("want true (flow in list)")
	}

	r, _ = recorder(t, response{out: []byte("foo\nbar\n")})
	if tmuxbridge.NewWithRunner(r).HasSession("missing") {
		t.Error("want false (missing not in list)")
	}

	r, _ = recorder(t, response{err: errors.New("no server")})
	if tmuxbridge.NewWithRunner(r).HasSession("any") {
		t.Error("want false on list-sessions error")
	}
}

func TestNewSessionAt_AlreadyExists_NoOp(t *testing.T) {
	// list-sessions returns the existing session → HasSession=true → no-op.
	r, calls := recorder(t, response{out: []byte("flow\n")})
	if err := tmuxbridge.NewWithRunner(r).NewSessionAt("flow", "/tmp"); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Errorf("want 1 call (list-sessions only), got %d: %+v", len(*calls), *calls)
	}
}

func TestNewSessionAt_CreatesWhenAbsent(t *testing.T) {
	r, calls := recorder(
		t,
		response{out: []byte("other\n")}, // list-sessions: name not present
		response{},                       // new-session succeeds
	)
	if err := tmuxbridge.NewWithRunner(r).NewSessionAt("flow", "/tmp/proj"); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 {
		t.Fatalf("want 2 calls, got %d: %+v", len(*calls), *calls)
	}
	want := []string{"new-session", "-d", "-s", "flow", "-c", "/tmp/proj"}
	if !reflect.DeepEqual((*calls)[1].args, want) {
		t.Errorf("new-session args: got %+v, want %v", (*calls)[1].args, want)
	}
}

func TestNewSessionAt_PropagatesCreateError(t *testing.T) {
	want := errors.New("permission denied")
	r, _ := recorder(
		t,
		response{err: errors.New("no")},
		response{err: want},
	)
	got := tmuxbridge.NewWithRunner(r).NewSessionAt("flow", "/tmp")
	if !errors.Is(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSwitchClient(t *testing.T) {
	r, calls := recorder(t)
	if err := tmuxbridge.NewWithRunner(r).SwitchClient("flow"); err != nil {
		t.Fatal(err)
	}
	want := []string{"switch-client", "-t", "flow"}
	if !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("got %+v", (*calls)[0])
	}
}

func TestSplitWindowH(t *testing.T) {
	r, calls := recorder(t)
	if err := tmuxbridge.NewWithRunner(r).SplitWindowH("nvim", "/tmp/file"); err != nil {
		t.Fatal(err)
	}
	want := []string{"split-window", "-h", "nvim", "/tmp/file"}
	if !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("got %+v", (*calls)[0])
	}
}

func TestSplitWindowH_NoArgs(t *testing.T) {
	r, calls := recorder(t)
	if err := tmuxbridge.NewWithRunner(r).SplitWindowH("htop"); err != nil {
		t.Fatal(err)
	}
	want := []string{"split-window", "-h", "htop"}
	if !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("got %+v", (*calls)[0])
	}
}

func TestNew_DefaultRunner_ResolvesToExec(t *testing.T) {
	// We can't actually invoke tmux in CI, but we can verify the
	// constructor returns a non-nil bridge that satisfies the port.
	b := tmuxbridge.New()
	if b == nil {
		t.Fatal("New returned nil")
	}
}
