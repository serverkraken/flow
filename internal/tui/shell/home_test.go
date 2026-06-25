package shell_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

type fakeDash struct{}

func (fakeDash) GetToday(context.Context) (apiclient.Today, error) {
	return apiclient.Today{Date: "2026-06-18", LoggedMin: 300, TargetMin: 480, Running: true}, nil
}

func (fakeDash) GetWeek(context.Context, string) ([]apiclient.WeekDay, error) {
	return []apiclient.WeekDay{{Date: "2026-06-18", LoggedMin: 300, TargetMin: 480, IsToday: true, Workday: true}}, nil
}

func (fakeDash) ListDocuments(context.Context, ...string) ([]domain.Document, error) {
	return []domain.Document{{ID: "d1", Path: "notes/x", Title: "Mein Dokument", UpdatedAt: time.Now()}}, nil
}

func (fakeDash) ListProjects(context.Context) ([]domain.Project, error) {
	return []domain.Project{{ID: "p1", Name: "ProjektA"}}, nil
}

func drainHome(r shell.Route, cmd tea.Cmd) shell.Route {
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		if msg == nil {
			break
		}
		r, cmd = r.Update(msg)
	}
	return r
}

func TestHomeRoute_rendersDashboard(t *testing.T) {
	var r shell.Route = shell.NewHomeRoute(fakeDash{}, theme.Default, "alice")
	r = drainHome(r, r.Init())
	body := r.View(shell.Frame{Width: 100, Height: 30, Pal: theme.Default})
	for _, want := range []string{"Arbeit", "Wissen", "Mein Dokument", "ProjektA"} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard missing %q:\n%s", want, body)
		}
	}
	if r.Title() != "Home" {
		t.Fatalf("title = %q, want Home", r.Title())
	}
}

func TestHomeRoute_wDrillsToWorktime(t *testing.T) {
	r := shell.NewHomeRoute(fakeDash{}, theme.Default, "alice")
	_, cmd := r.Update(tea.KeyPressMsg{Text: "w"})
	if cmd == nil {
		t.Fatal("w should emit a drill cmd")
	}
	msg, ok := cmd().(shell.SwitchTabMsg)
	if !ok || msg.Title != "Worktime" {
		t.Fatalf("w should emit SwitchTabMsg{Worktime}, got %#v", cmd())
	}
}

func TestHomeRoute_reloadsOnSessionEvent(t *testing.T) {
	r := shell.NewHomeRoute(fakeDash{}, theme.Default, "alice")
	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventSessionStarted)}})
	if cmd == nil {
		t.Fatal("a session event should trigger a reload cmd")
	}
}

func TestHomeRoute_reloadsOnProjectEvent(t *testing.T) {
	r := shell.NewHomeRoute(fakeDash{}, theme.Default, "alice")
	_, cmd := r.Update(shell.EventMsg{Ev: apiclient.ClientEvent{Type: string(domain.EventProjectCreated)}})
	if cmd == nil {
		t.Fatal("a project.created event should trigger a reload cmd")
	}
}
