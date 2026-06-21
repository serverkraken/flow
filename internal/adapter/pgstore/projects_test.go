package pgstore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

func TestProjectStore_Delete(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	// Seed user so FK constraints on work_sessions / documents / project_bindings are satisfied.
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-del", "sub-del", "deluser", "del@x.de", "Del User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}

	projects := pgstore.NewProjectStore(pool)
	now := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	proj, _ := domain.NewProject("p-del", "u-del", "ToDelete", "to-delete", now)
	if _, err := projects.Create(ctx, proj); err != nil {
		t.Fatal(err)
	}

	// Insert a completed work_sessions row referencing the project.
	sessions := pgstore.NewSessionStore(pool)
	pid := "p-del"
	stopAt := now.Add(time.Hour)
	ws, _ := domain.NewWorkSession("ws-del", "u-del", nil, now)
	if _, err := sessions.Create(ctx, ws); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Stop(ctx, "u-del", "ws-del", &pid, stopAt); err != nil {
		t.Fatal(err)
	}

	// Insert a documents row referencing the project.
	docs := pgstore.NewDocumentStore(pool)
	docPID := "p-del"
	doc := domain.Document{
		ID:        "doc-del",
		OwnerID:   "u-del",
		Type:      domain.DocFree,
		ProjectID: &docPID,
		Path:      "del/arch",
		Title:     "Del",
		Body:      "# Del",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := docs.Create(ctx, doc); err != nil {
		t.Fatal(err)
	}

	// Insert a project_bindings row (kind=remote) — must cascade-delete.
	const bindQ = `
INSERT INTO project_bindings (id, owner_id, project_id, kind, remote_slug, created_at, updated_at)
VALUES ($1, $2, $3, 'remote', $4, $5, $6)`
	if _, err := pool.Exec(ctx, bindQ, "bind-del", "u-del", "p-del", "github.com/del/repo", now, now); err != nil {
		t.Fatal(err)
	}

	// --- TDD RED gate: Delete the project ---
	if err := projects.Delete(ctx, "u-del", "p-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Project must be gone.
	if _, err := projects.Get(ctx, "u-del", "p-del"); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Fatalf("after Delete, Get must return ErrProjectNotFound, got %v", err)
	}

	// work_sessions row must STILL EXIST with project_id IS NULL (SET NULL).
	var wsProjectID *string
	if err := pool.QueryRow(ctx, `SELECT project_id FROM work_sessions WHERE id=$1`, "ws-del").
		Scan(&wsProjectID); err != nil {
		t.Fatalf("work_sessions row missing after project delete: %v", err)
	}
	if wsProjectID != nil {
		t.Errorf("work_sessions.project_id should be NULL after project delete, got %q", *wsProjectID)
	}

	// documents row must STILL EXIST with project_id IS NULL (SET NULL).
	var docProjectID *string
	if err := pool.QueryRow(ctx, `SELECT project_id FROM documents WHERE id=$1`, "doc-del").
		Scan(&docProjectID); err != nil {
		t.Fatalf("documents row missing after project delete: %v", err)
	}
	if docProjectID != nil {
		t.Errorf("documents.project_id should be NULL after project delete, got %q", *docProjectID)
	}

	// project_bindings row must be GONE (cascade).
	var bindCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM project_bindings WHERE id=$1`, "bind-del").
		Scan(&bindCount); err != nil {
		t.Fatal(err)
	}
	if bindCount != 0 {
		t.Errorf("project_bindings should be cascade-deleted, but %d row(s) remain", bindCount)
	}

	// Delete of a missing id → ErrProjectNotFound.
	if err := projects.Delete(ctx, "u-del", "p-del"); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("double Delete: want ErrProjectNotFound, got %v", err)
	}
}

func TestProjectStore_RateRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	// seed a user so the project FK is satisfied
	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-rate", "sub-rate", "rateuser", "rate@x.de", "Rate User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}

	st := pgstore.NewProjectStore(pool)
	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	proj, _ := domain.NewProject("p1", "u-rate", "Acme", "acme", now)
	p, err := st.Create(ctx, proj)
	if err != nil {
		t.Fatal(err)
	}
	if p.Rate != nil {
		t.Fatalf("fresh project should have nil rate, got %+v", p.Rate)
	}

	if err := st.SetRate(ctx, "u-rate", "p1", &domain.Money{Amount: 8000, Currency: "EUR"}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Get(ctx, "u-rate", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Rate == nil || got.Rate.Amount != 8000 || got.Rate.Currency != "EUR" {
		t.Errorf("rate after set: got %+v", got.Rate)
	}

	if err := st.SetRate(ctx, "u-rate", "p1", nil); err != nil {
		t.Fatal(err)
	}
	got, err = st.Get(ctx, "u-rate", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Rate != nil {
		t.Errorf("rate after clear should be nil, got %+v", got.Rate)
	}

	if err := st.SetRate(ctx, "u-rate", "nope", &domain.Money{Amount: 1, Currency: "EUR"}); !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("unknown id: want ErrProjectNotFound, got %v", err)
	}
}
