package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestStartStopBookingFlow(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	ids := &testutil.FakeIDGen{}
	sessions := testutil.NewFakeSessionStore()
	projects := testutil.NewFakeProjectStore()

	start := usecase.StartSession{Sessions: sessions, IDs: ids, Clock: clk}
	stop := usecase.StopSession{Sessions: sessions, Projects: projects, Clock: clk}
	createProj := usecase.CreateProject{Projects: projects, IDs: ids, Clock: clk}

	s, err := start.Execute(ctx, "u1", nil, "", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !s.Running() {
		t.Fatal("started session must be running")
	}
	if _, err := start.Execute(ctx, "u1", nil, "", ""); !errors.Is(err, domain.ErrAlreadyRunning) {
		t.Fatalf("want ErrAlreadyRunning, got %v", err)
	}
	if _, err := stop.Execute(ctx, "u1", s.ID, nil); !errors.Is(err, domain.ErrProjectRequired) {
		t.Fatalf("want ErrProjectRequired, got %v", err)
	}
	p, err := createProj.Execute(ctx, "u1", "Flow", "", "", "")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if p.Slug != "flow" {
		t.Fatalf("slug not derived from name: %q", p.Slug)
	}
	clk.T = clk.T.Add(time.Hour)
	stop.Clock = clk
	stopped, err := stop.Execute(ctx, "u1", s.ID, &p.ID)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if stopped.Stop == nil || stopped.ProjectID == nil || *stopped.ProjectID != p.ID {
		t.Fatalf("stop result wrong: %+v", stopped)
	}
	s2, _ := start.Execute(ctx, "u1", nil, "", "")
	bad := "ghost"
	if _, err := stop.Execute(ctx, "u1", s2.ID, &bad); err == nil {
		t.Fatal("stop with unknown project must error")
	}
}

func TestListSessionsSince(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)}
	sessions := testutil.NewFakeSessionStore()
	list := usecase.ListSessions{Sessions: sessions, Clock: clk}
	old, _ := domain.NewWorkSession("old", "u1", nil, clk.T.Add(-48*time.Hour))
	oldStop := old.Start.Add(time.Hour)
	old.Stop = &oldStop
	_, _ = sessions.Create(ctx, old)
	got, err := list.Execute(ctx, "u1", clk.T.Add(-24*time.Hour))
	if err != nil || len(got) != 0 {
		t.Fatalf("since-filter failed: %d err=%v", len(got), err)
	}
}
