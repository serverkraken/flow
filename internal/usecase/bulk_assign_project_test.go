package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func seedSess(t *testing.T, ss *testutil.FakeSessionStore, id, owner string) {
	t.Helper()
	st := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	sp := st.Add(time.Hour)
	if _, err := ss.Create(context.Background(), domain.WorkSession{ID: id, OwnerID: owner, Start: st, Stop: &sp}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func TestBulkAssignProject_AssignsOwnedSkipsForeign(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeProjectStore()
	seedSess(t, ss, "a", "u1")
	seedSess(t, ss, "b", "u1")
	seedSess(t, ss, "c", "u2") // foreign
	if _, err := ps.Create(ctx, domain.Project{ID: "p1", OwnerID: "u1", Name: "flow"}); err != nil {
		t.Fatalf("seed proj: %v", err)
	}
	uc := usecase.BulkAssignProject{Sessions: ss, Projects: ps}
	n, err := uc.Execute(ctx, "u1", []string{"a", "b", "c", "missing"}, "p1")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if n != 2 {
		t.Fatalf("updated = %d, want 2 (a,b; c foreign + missing skipped)", n)
	}
	got, _ := ss.Get(ctx, "u1", "a")
	if got.ProjectID == nil || *got.ProjectID != "p1" {
		t.Fatalf("a not assigned: %+v", got)
	}
	// foreign session untouched
	if c, _ := ss.Get(ctx, "u2", "c"); c.ProjectID != nil {
		t.Fatalf("foreign c was mutated: %+v", c)
	}
}

func TestBulkAssignProject_EmptyIDs(t *testing.T) {
	uc := usecase.BulkAssignProject{Sessions: testutil.NewFakeSessionStore(), Projects: testutil.NewFakeProjectStore()}
	if _, err := uc.Execute(context.Background(), "u1", nil, "p1"); !errors.Is(err, usecase.ErrNoSessions) {
		t.Fatalf("err = %v, want ErrNoSessions", err)
	}
}

func TestBulkAssignProject_ForeignProject(t *testing.T) {
	ctx := context.Background()
	ss := testutil.NewFakeSessionStore()
	ps := testutil.NewFakeProjectStore()
	seedSess(t, ss, "a", "u1")
	if _, err := ps.Create(ctx, domain.Project{ID: "p2", OwnerID: "other", Name: "x"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	uc := usecase.BulkAssignProject{Sessions: ss, Projects: ps}
	if _, err := uc.Execute(ctx, "u1", []string{"a"}, "p2"); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Fatalf("err = %v, want ErrProjectNotFound", err)
	}
}
