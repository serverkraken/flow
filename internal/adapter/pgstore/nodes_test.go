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

	projects := pgstore.NewNodeStore(pool)
	now := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	proj, _ := domain.NewNode("p-del", "u-del", "ToDelete", "to-delete", now)
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
		NodeID: &docPID,
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
INSERT INTO project_bindings (id, owner_id, node_id, kind, remote_slug, created_at, updated_at)
VALUES ($1, $2, $3, 'remote', $4, $5, $6)`
	if _, err := pool.Exec(ctx, bindQ, "bind-del", "u-del", "p-del", "github.com/del/repo", now, now); err != nil {
		t.Fatal(err)
	}

	// --- TDD RED gate: Delete the project ---
	if err := projects.Delete(ctx, "u-del", "p-del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Project must be gone.
	if _, err := projects.Get(ctx, "u-del", "p-del"); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("after Delete, Get must return ErrNodeNotFound, got %v", err)
	}

	// work_sessions row must STILL EXIST with node_id IS NULL (SET NULL).
	var wsProjectID *string
	if err := pool.QueryRow(ctx, `SELECT node_id FROM work_sessions WHERE id=$1`, "ws-del").
		Scan(&wsProjectID); err != nil {
		t.Fatalf("work_sessions row missing after project delete: %v", err)
	}
	if wsProjectID != nil {
		t.Errorf("work_sessions.node_id should be NULL after project delete, got %q", *wsProjectID)
	}

	// documents row must STILL EXIST with node_id IS NULL (SET NULL).
	var docProjectID *string
	if err := pool.QueryRow(ctx, `SELECT node_id FROM documents WHERE id=$1`, "doc-del").
		Scan(&docProjectID); err != nil {
		t.Fatalf("documents row missing after project delete: %v", err)
	}
	if docProjectID != nil {
		t.Errorf("documents.node_id should be NULL after project delete, got %q", *docProjectID)
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

	// Delete of a missing id → ErrNodeNotFound.
	if err := projects.Delete(ctx, "u-del", "p-del"); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Errorf("double Delete: want ErrNodeNotFound, got %v", err)
	}
}

func TestProjectStore_UpdateRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-upd", "sub-upd", "upduser", "upd@x.de", "Upd User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}

	st := pgstore.NewNodeStore(pool)
	now := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	proj, _ := domain.NewNode("p-upd", "u-upd", "Acme", "acme", now)
	if _, err := st.Create(ctx, proj); err != nil {
		t.Fatal(err)
	}

	// Set a rate so we can prove Update preserves it.
	if err := st.SetRate(ctx, "u-upd", "p-upd", &domain.Money{Amount: 9000, Currency: "EUR"}); err != nil {
		t.Fatal(err)
	}

	upd := proj
	upd.Name = "Acme Reloaded"
	upd.Description = "# Notes\nhello"
	upd.UpstreamGit = "git@github.com:acme/reloaded.git"
	upd.Status = domain.NodePaused
	upd.UpdatedAt = now.Add(time.Hour)
	got, err := st.Update(ctx, "u-upd", upd)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name != "Acme Reloaded" || got.Description != "# Notes\nhello" ||
		got.UpstreamGit != "git@github.com:acme/reloaded.git" || got.Status != domain.NodePaused {
		t.Errorf("Update returned %+v", got)
	}
	if got.Rate == nil || got.Rate.Amount != 9000 {
		t.Errorf("Update must preserve rate, got %+v", got.Rate)
	}

	// Re-read confirms persistence.
	re, err := st.Get(ctx, "u-upd", "p-upd")
	if err != nil {
		t.Fatalf("Get after Update: %v", err)
	}
	if re.Status != domain.NodePaused || re.Description != "# Notes\nhello" {
		t.Errorf("Get after Update: %+v", re)
	}

	// Unknown id → ErrNodeNotFound.
	miss := upd
	miss.ID = "nope"
	if _, err := st.Update(ctx, "u-upd", miss); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Errorf("unknown id: want ErrNodeNotFound, got %v", err)
	}

	// Foreign owner (real id, wrong owner) → ErrNodeNotFound.
	if _, err := st.Update(ctx, "someone-else", upd); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Errorf("foreign owner: want ErrNodeNotFound, got %v", err)
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

	st := pgstore.NewNodeStore(pool)
	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	proj, _ := domain.NewNode("p1", "u-rate", "Acme", "acme", now)
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

	if err := st.SetRate(ctx, "u-rate", "nope", &domain.Money{Amount: 1, Currency: "EUR"}); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Errorf("unknown id: want ErrNodeNotFound, got %v", err)
	}
}
