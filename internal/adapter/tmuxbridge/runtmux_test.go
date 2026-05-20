package tmuxbridge_test

import (
	"reflect"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/tmuxbridge"
)

func TestRunTmuxAction_PrefixedDispatch(t *testing.T) {
	r, calls := recorder(t)
	if err := tmuxbridge.NewWithRunner(r).RunTmuxAction("split-window -h"); err != nil {
		t.Fatalf("RunTmuxAction: %v", err)
	}
	want := []string{"run-shell", "-b", "tmux split-window -h"}
	if len(*calls) != 1 || (*calls)[0].name != "tmux" || !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("calls: %+v want args=%v", *calls, want)
	}
}
