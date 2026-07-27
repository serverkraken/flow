package shell_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/shell"
)

func TestEventMsgExported(t *testing.T) {
	m := shell.EventMsg{Ev: apiclient.ClientEvent{Type: "session.started"}}
	if m.Ev.Type != "session.started" {
		t.Fatalf("EventMsg field not accessible: %+v", m)
	}
}
