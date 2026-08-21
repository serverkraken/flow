package pgstore_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil"
)

func TestNodeStore_ReparentRejectsCycleAtomically(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	u, _ := domain.NewUser("u-cycle", "sub-cycle", "cycle", "cycle@x.de", "Cycle")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewNodeStore(pool)
	now := time.Now().UTC()
	mk := func(id string, parent *string) {
		n, _ := domain.NewNode(id, u.ID, id, id, now)
		n.Kind = domain.KindVorhaben
		n.ParentID = parent
		if _, err := st.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	root, _ := domain.NewNode("root-cycle", u.ID, "root-cycle", "root-cycle", now)
	root.Kind = domain.KindEngagement
	if _, err := st.Create(ctx, root); err != nil {
		t.Fatal(err)
	}
	mk("a-cycle", strptr(root.ID))
	mk("b-cycle", strptr(root.ID))

	start := make(chan struct{})
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for _, move := range []struct{ id, parent string }{{"a-cycle", "b-cycle"}, {"b-cycle", "a-cycle"}} {
		move := move
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := st.Reparent(ctx, u.ID, move.id, strptr(move.parent))
			errCh <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	cycleErrors := 0
	for err := range errCh {
		if errors.Is(err, ports.ErrNodeCycle) {
			cycleErrors++
		} else if err != nil {
			t.Fatalf("unexpected move error: %v", err)
		}
	}
	if cycleErrors != 1 {
		t.Fatalf("exactly one concurrent move must be rejected, cycle errors=%d", cycleErrors)
	}
	for _, id := range []string{"a-cycle", "b-cycle"} {
		chain, err := st.Ancestors(ctx, u.ID, id)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, n := range chain {
			if seen[n.ID] {
				t.Fatalf("cycle persisted in ancestor chain for %s: %v", id, chain)
			}
			seen[n.ID] = true
		}
	}
}

func TestNodeStore_RecursiveReadsStopAtCorruptCycle(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	u, _ := domain.NewUser("u-corrupt-cycle", "sub-corrupt-cycle", "corrupt", "corrupt@x.de", "Corrupt")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewNodeStore(pool)
	now := time.Now().UTC()
	root, _ := domain.NewNode("root-corrupt", u.ID, "root-corrupt", "root-corrupt", now)
	root.Kind = domain.KindEngagement
	if _, err := st.Create(ctx, root); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a-corrupt", "b-corrupt"} {
		n, _ := domain.NewNode(id, u.ID, id, id, now)
		n.Kind = domain.KindVorhaben
		n.ParentID = strptr(root.ID)
		if _, err := st.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE nodes SET parent_id=CASE id WHEN 'a-corrupt' THEN 'b-corrupt' ELSE 'a-corrupt' END WHERE owner_id=$1 AND id IN ('a-corrupt','b-corrupt')`, u.ID); err != nil {
		t.Fatal(err)
	}
	anc, err := st.Ancestors(ctx, u.ID, "a-corrupt")
	if err != nil || len(anc) != 2 {
		t.Fatalf("cycle-safe ancestors: len=%d err=%v", len(anc), err)
	}
	sub, err := st.Subtree(ctx, u.ID, "a-corrupt")
	if err != nil || len(sub) != 2 {
		t.Fatalf("cycle-safe subtree: len=%d err=%v", len(sub), err)
	}
}

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
	proj.Kind = domain.KindEngagement
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
	docs := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	docPID := "p-del"
	doc := domain.Document{
		ID:        "doc-del",
		OwnerID:   "u-del",
		Type:      domain.DocFree,
		NodeID:    &docPID,
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

func TestNodeStore_DeleteRejectsProjectDocuments(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	u, _ := domain.NewUser("u-del-doc", "sub-del-doc", "deldoc", "deldoc@x.de", "Delete Doc")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	nodes := pgstore.NewNodeStore(pool)
	node, _ := domain.NewNode("node-del-doc", u.ID, "Delete Doc", "delete-doc", time.Now().UTC())
	node.Kind = domain.KindEngagement
	if _, err := nodes.Create(ctx, node); err != nil {
		t.Fatal(err)
	}
	docs := pgstore.NewDocumentStore(pool, &testutil.FakeIDGen{})
	if _, err := docs.Create(ctx, domain.Document{
		ID: "project-doc", OwnerID: u.ID, NodeID: &node.ID, Type: domain.DocProject,
		Path: "readme", Title: "README", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	if err := nodes.Delete(ctx, u.ID, node.ID); !errors.Is(err, ports.ErrNodeHasProjectDocuments) {
		t.Fatalf("Delete error = %v, want ErrNodeHasProjectDocuments", err)
	}
	if _, err := nodes.Get(ctx, u.ID, node.ID); err != nil {
		t.Fatalf("node changed after blocked delete: %v", err)
	}
	got, err := docs.Get(ctx, u.ID, "project-doc")
	if err != nil || got.NodeID == nil || *got.NodeID != node.ID || got.Type != domain.DocProject {
		t.Fatalf("project document changed after blocked delete: doc=%+v err=%v", got, err)
	}
}

func TestNodeStore_DeleteWaitsForConcurrentProjectDocument(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	u, _ := domain.NewUser("u-del-race", "sub-del-race", "delrace", "delrace@x.de", "Delete Race")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	nodes := pgstore.NewNodeStore(pool)
	node, _ := domain.NewNode("node-del-race", u.ID, "Delete Race", "delete-race", time.Now().UTC())
	node.Kind = domain.KindEngagement
	if _, err := nodes.Create(ctx, node); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `INSERT INTO documents
		(id, owner_id, node_id, type, path, title, body, extra, created_at, updated_at)
		VALUES ($1,$2,$3,'project','readme','README','','{}',$4,$4)`,
		"project-doc-race", u.ID, node.ID, now); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- nodes.Delete(ctx, u.ID, node.ID) }()
	select {
	case err := <-errCh:
		t.Fatalf("delete did not wait for concurrent FK insert: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; !errors.Is(err, ports.ErrNodeHasProjectDocuments) {
		t.Fatalf("delete after concurrent insert = %v, want ErrNodeHasProjectDocuments", err)
	}
	if _, err := nodes.Get(ctx, u.ID, node.ID); err != nil {
		t.Fatalf("node changed after concurrent blocked delete: %v", err)
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
	proj.Kind = domain.KindEngagement
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

func TestNodeStore_HierarchyRoundTrip(t *testing.T) {
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
	u, _ := domain.NewUser("u-h", "sub-h", "huser", "h@x.de", "H User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewNodeStore(pool)
	now := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)

	eng, _ := domain.NewNode("eng", "u-h", "Privat", "privat", now)
	eng.Kind = domain.KindEngagement
	eng.Extra = map[string]any{"legacy_rate": map[string]any{"amount": float64(9000), "currency": "EUR"}}
	if _, err := st.Create(ctx, eng); err != nil {
		t.Fatalf("create engagement: %v", err)
	}

	repo, _ := domain.NewNode("repo", "u-h", "flow", "flow", now)
	repo.Kind = domain.KindRepo
	repo.ParentID = strptr("eng")
	repo.OriginSlug = "github.com/serverkraken/flow"
	got, err := st.Create(ctx, repo)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	if got.Kind != domain.KindRepo || got.ParentID == nil || *got.ParentID != "eng" || got.OriginSlug != "github.com/serverkraken/flow" {
		t.Fatalf("create returned %+v", got)
	}

	re, err := st.Get(ctx, "u-h", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if re.Kind != domain.KindRepo || re.ParentID == nil || *re.ParentID != "eng" {
		t.Errorf("get repo: %+v", re)
	}
	reEng, _ := st.Get(ctx, "u-h", "eng")
	if reEng.Extra["legacy_rate"] == nil {
		t.Errorf("engagement extra lost: %+v", reEng.Extra)
	}

	// Update persists origin_slug + extra, leaves parent_id + rate untouched.
	upd := re
	upd.OriginSlug = "github.com/serverkraken/flow2"
	upd.Extra = map[string]any{"note": "x"}
	upd.UpdatedAt = now.Add(time.Hour)
	if _, err := st.Update(ctx, "u-h", upd); err != nil {
		t.Fatalf("update: %v", err)
	}
	re2, _ := st.Get(ctx, "u-h", "repo")
	if re2.OriginSlug != "github.com/serverkraken/flow2" || re2.Extra["note"] != "x" {
		t.Errorf("update did not persist origin/extra: %+v", re2)
	}
	if re2.ParentID == nil || *re2.ParentID != "eng" {
		t.Errorf("update must not touch parent_id: %+v", re2.ParentID)
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
	proj.Kind = domain.KindEngagement
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

func TestNodeStore_TreeWalk(t *testing.T) {
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
	u, _ := domain.NewUser("u-t", "sub-t", "tuser", "t@x.de", "T")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewNodeStore(pool)
	now := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)

	mk := func(id, name, slug string, kind domain.NodeKind, parent *string) {
		n, _ := domain.NewNode(id, "u-t", name, slug, now)
		n.Kind = kind
		n.ParentID = parent
		if _, err := st.Create(ctx, n); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("eng", "Privat", "privat", domain.KindEngagement, nil)
	mk("vor", "Sub", "sub", domain.KindVorhaben, strptr("eng"))
	mk("repo", "flow", "flow", domain.KindRepo, strptr("vor"))

	// Children of root (nil) = the engagement.
	roots, err := st.Children(ctx, "u-t", nil)
	if err != nil || len(roots) != 1 || roots[0].ID != "eng" {
		t.Fatalf("children(nil)=%v err=%v", roots, err)
	}
	// Children of eng = vor.
	kids, _ := st.Children(ctx, "u-t", strptr("eng"))
	if len(kids) != 1 || kids[0].ID != "vor" {
		t.Fatalf("children(eng)=%v", kids)
	}
	// Ancestors of repo, leaf→root: repo, vor, eng.
	chain, err := st.Ancestors(ctx, "u-t", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 3 || chain[0].ID != "repo" || chain[1].ID != "vor" || chain[2].ID != "eng" {
		t.Fatalf("ancestors=%v", chain)
	}
	eng, ok := domain.ResolveEngagement(chain)
	if !ok || eng.ID != "eng" {
		t.Fatalf("resolveEngagement=%v ok=%v", eng, ok)
	}
	// Reparent repo onto eng directly.
	if _, err := st.Reparent(ctx, "u-t", "repo", strptr("eng")); err != nil {
		t.Fatalf("reparent: %v", err)
	}
	chain2, _ := st.Ancestors(ctx, "u-t", "repo")
	if len(chain2) != 2 || chain2[1].ID != "eng" {
		t.Fatalf("after reparent ancestors=%v", chain2)
	}
	// Delete with children → ErrNodeHasChildren; leaf delete → ok.
	if err := st.Delete(ctx, "u-t", "eng"); !errors.Is(err, ports.ErrNodeHasChildren) {
		t.Fatalf("delete eng with children: want ErrNodeHasChildren, got %v", err)
	}
	if err := st.Delete(ctx, "u-t", "repo"); err != nil {
		t.Fatalf("delete leaf repo: %v", err)
	}
	if err := st.Delete(ctx, "u-t", "missing"); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("delete missing: want ErrNodeNotFound, got %v", err)
	}
}

func TestNodeStore_CountsTowardTarget(t *testing.T) {
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
	u, _ := domain.NewUser("u-ctt", "sub-ctt", "cttuser", "ctt@x.de", "CTT User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}

	st := pgstore.NewNodeStore(pool)
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)

	// Case 1: NewNode leaves CountsTowardTarget nil (inherit); round-trip must
	// return nil (persisted as SQL NULL).
	n1, _ := domain.NewNode("ctt-1", "u-ctt", "Default", "ctt-default", now)
	n1.Kind = domain.KindEngagement
	// CountsTowardTarget is already nil from NewNode — do not override.
	created1, err := st.Create(ctx, n1)
	if err != nil {
		t.Fatalf("Create n1: %v", err)
	}
	if created1.CountsTowardTarget != nil {
		t.Errorf("Create returned CountsTowardTarget=%v, want nil", *created1.CountsTowardTarget)
	}
	got1, err := st.Get(ctx, "u-ctt", "ctt-1")
	if err != nil {
		t.Fatalf("Get n1: %v", err)
	}
	if got1.CountsTowardTarget != nil {
		t.Errorf("Get after Create: CountsTowardTarget=%v, want nil", *got1.CountsTowardTarget)
	}

	// Case 2: explicit CountsTowardTarget:false persists.
	n2, _ := domain.NewNode("ctt-2", "u-ctt", "Exclude", "ctt-exclude", now)
	n2.Kind = domain.KindEngagement
	falseVal := false
	n2.CountsTowardTarget = &falseVal
	created2, err := st.Create(ctx, n2)
	if err != nil {
		t.Fatalf("Create n2: %v", err)
	}
	if created2.CountsTowardTarget == nil || *created2.CountsTowardTarget {
		t.Errorf("Create returned CountsTowardTarget=%v, want *false", created2.CountsTowardTarget)
	}
	got2, err := st.Get(ctx, "u-ctt", "ctt-2")
	if err != nil {
		t.Fatalf("Get n2: %v", err)
	}
	if got2.CountsTowardTarget == nil || *got2.CountsTowardTarget {
		t.Errorf("Get after Create: CountsTowardTarget=%v, want *false", got2.CountsTowardTarget)
	}

	// Case 3: Update CountsTowardTarget false→true persists.
	upd := got2
	trueVal := true
	upd.CountsTowardTarget = &trueVal
	upd.UpdatedAt = now.Add(time.Hour)
	if _, err := st.Update(ctx, "u-ctt", upd); err != nil {
		t.Fatalf("Update n2: %v", err)
	}
	got3, err := st.Get(ctx, "u-ctt", "ctt-2")
	if err != nil {
		t.Fatalf("Get after Update: %v", err)
	}
	if got3.CountsTowardTarget == nil || !*got3.CountsTowardTarget {
		t.Errorf("Get after Update: CountsTowardTarget=%v, want *true", got3.CountsTowardTarget)
	}
}

// TestNodeStore_CountsTowardTargetNullable verifies the nullable-column
// round-trip added by migration 0025: nil persists as SQL NULL and an
// explicit true persists as *true.
func TestNodeStore_CountsTowardTargetNullable(t *testing.T) {
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
	u, _ := domain.NewUser("u-cttn", "sub-cttn", "cttnuser", "cttn@x.de", "CTTN User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}

	st := pgstore.NewNodeStore(pool)
	mk := func(id string, ctt *bool) domain.Node {
		n, _ := domain.NewNode(id, "u-cttn", id, id, time.Now())
		n.Kind = domain.KindEngagement
		n.CountsTowardTarget = ctt
		return n
	}
	tt := true
	got, err := st.Create(ctx, mk("n-inherit", nil))
	if err != nil {
		t.Fatal(err)
	}
	if got.CountsTowardTarget != nil {
		t.Errorf("nil must persist as NULL, got %v", *got.CountsTowardTarget)
	}
	got2, err := st.Create(ctx, mk("n-work", &tt))
	if err != nil {
		t.Fatal(err)
	}
	if got2.CountsTowardTarget == nil || !*got2.CountsTowardTarget {
		t.Errorf("explicit true lost")
	}
}

// TestNodeStore_IconLogoRefRoundTrip verifies the icon/logo_ref columns added
// by migration 0026 survive Create, Update and the recursive CTE reads.
func TestNodeStore_IconLogoRefRoundTrip(t *testing.T) {
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
	u, _ := domain.NewUser("u-icon", "sub-icon", "iconuser", "icon@x.de", "Icon User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}

	st := pgstore.NewNodeStore(pool)
	n, _ := domain.NewNode("n-icon", "u-icon", "n-icon", "n-icon", time.Now())
	n.Kind = domain.KindEngagement
	n.Icon = "rocket"
	got, err := st.Create(ctx, n)
	if err != nil {
		t.Fatal(err)
	}
	if got.Icon != "rocket" || got.LogoRef != "" {
		t.Errorf("create round-trip: icon=%q logoRef=%q", got.Icon, got.LogoRef)
	}
	got.Icon, got.LogoRef = "leaf", "abc123def456"
	got2, err := st.Update(ctx, "u-icon", got)
	if err != nil {
		t.Fatal(err)
	}
	if got2.Icon != "leaf" || got2.LogoRef != "abc123def456" {
		t.Errorf("update round-trip: icon=%q logoRef=%q", got2.Icon, got2.LogoRef)
	}
	// Subtree/Ancestors CTEs must carry the new columns too.
	sub, err := st.Subtree(ctx, "u-icon", got.ID)
	if err != nil || len(sub) == 0 {
		t.Fatalf("subtree: %v (len %d)", err, len(sub))
	}
	if sub[0].Icon != "leaf" {
		t.Errorf("subtree lost icon: %q", sub[0].Icon)
	}
	anc, err := st.Ancestors(ctx, "u-icon", got.ID)
	if err != nil || len(anc) == 0 {
		t.Fatalf("ancestors: %v (len %d)", err, len(anc))
	}
	if anc[0].LogoRef != "abc123def456" {
		t.Errorf("ancestors lost logoRef: %q", anc[0].LogoRef)
	}
}

// TestNodeLogoStore_PutGetDeleteCascade verifies the node_logos blob store:
// upsert-on-put (including the migration-0027 width/height columns),
// owner-scoped Get, and FK ON DELETE CASCADE when the owning node is deleted.
func TestNodeLogoStore_PutGetDeleteCascade(t *testing.T) {
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
	u, _ := domain.NewUser("u-logo", "sub-logo", "logouser", "logo@x.de", "Logo User")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}

	st := pgstore.NewNodeStore(pool)
	ls := pgstore.NewNodeLogoStore(pool)
	n, _ := domain.NewNode("n-logo", "u-logo", "n-logo", "n-logo", time.Now())
	n.Kind = domain.KindEngagement
	created, err := st.Create(ctx, n)
	if err != nil {
		t.Fatal(err)
	}
	l := domain.NodeLogo{NodeID: created.ID, OwnerID: "u-logo", Mime: "image/png",
		Ref: "aaaabbbbcccc", Bytes: []byte{1, 2, 3}, UpdatedAt: time.Now(),
		Width: 64, Height: 64}
	if err := ls.Put(ctx, l); err != nil {
		t.Fatal(err)
	}
	l.Bytes = []byte{9, 9, 9} // replace-on-put
	l.Width, l.Height = 300, 100
	if err := ls.Put(ctx, l); err != nil {
		t.Fatal(err)
	}
	got, err := ls.Get(ctx, "u-logo", created.ID)
	if err != nil || len(got.Bytes) != 3 || got.Bytes[0] != 9 {
		t.Fatalf("get after upsert: %v (bytes %v)", err, got.Bytes)
	}
	if got.Width != 300 || got.Height != 100 {
		t.Errorf("dimensions did not round-trip: got %dx%d, want 300x100", got.Width, got.Height)
	}
	if _, err := ls.Get(ctx, "intruder", created.ID); !errors.Is(err, ports.ErrNodeLogoNotFound) {
		t.Errorf("foreign owner must not see the logo: %v", err)
	}
	// Node delete cascades the blob (FK ON DELETE CASCADE).
	if err := st.Delete(ctx, "u-logo", created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ls.Get(ctx, "u-logo", created.ID); !errors.Is(err, ports.ErrNodeLogoNotFound) {
		t.Errorf("logo survived node delete: %v", err)
	}
}

func TestNodeStore_Subtree(t *testing.T) {
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
	u, _ := domain.NewUser("u-sub", "sub-sub", "subuser", "sub@x.de", "Sub")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewNodeStore(pool)
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)

	mk := func(id, name, slug string, kind domain.NodeKind, parent *string) {
		n, _ := domain.NewNode(id, "u-sub", name, slug, now)
		n.Kind = kind
		n.ParentID = parent
		if _, err := st.Create(ctx, n); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("eng", "Privat", "privat", domain.KindEngagement, nil)
	mk("vor", "Sub", "sub", domain.KindVorhaben, strptr("eng"))
	mk("repo", "flow", "flow", domain.KindRepo, strptr("vor"))
	mk("other", "Other", "other", domain.KindEngagement, nil)

	// Subtree(eng) should return eng + vor + repo (3 nodes, root→leaf).
	sub, err := st.Subtree(ctx, "u-sub", "eng")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, n := range sub {
		ids[n.ID] = true
	}
	if len(ids) != 3 || !ids["eng"] || !ids["vor"] || !ids["repo"] || ids["other"] {
		t.Fatalf("Subtree(eng): want {eng,vor,repo}, got %v", ids)
	}
	// First node must be the root (depth 0).
	if sub[0].ID != "eng" {
		t.Fatalf("Subtree(eng): first node must be eng (depth 0), got %q", sub[0].ID)
	}

	// Subtree(repo) = just the leaf.
	leaf, err := st.Subtree(ctx, "u-sub", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(leaf) != 1 || leaf[0].ID != "repo" {
		t.Fatalf("Subtree(repo): want [repo], got %v", leaf)
	}

	// Cross-owner isolation: a node owned by u-sub2 but parented to u-sub's "eng"
	// must be invisible from u-sub's subtree.
	u2, _ := domain.NewUser("u-sub2", "sub-sub2", "subuser2", "sub2@x.de", "Sub2")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u2); err != nil {
		t.Fatal(err)
	}
	intruder, _ := domain.NewNode("intruder", "u-sub2", "Intruder", "intruder", now)
	intruder.Kind = domain.KindRepo
	intruder.ParentID = strptr("eng")
	if _, err := st.Create(ctx, intruder); err != nil {
		t.Fatalf("create intruder: %v", err)
	}

	// u-sub's subtree must still be exactly {eng, vor, repo}.
	sub2, err := st.Subtree(ctx, "u-sub", "eng")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range sub2 {
		if n.ID == "intruder" {
			t.Fatalf("Subtree(u-sub, eng) must not include intruder owned by u-sub2")
		}
	}
	if len(sub2) != 3 {
		t.Fatalf("Subtree(u-sub, eng) after intruder: want 3 nodes, got %d", len(sub2))
	}

	// u-sub2 does not own "eng" → Subtree must return empty.
	sub3, err := st.Subtree(ctx, "u-sub2", "eng")
	if err != nil {
		t.Fatal(err)
	}
	if len(sub3) != 0 {
		t.Fatalf("Subtree(u-sub2, eng): want empty (u-sub2 doesn't own eng), got %v", sub3)
	}
}

// TestNodeStore_BannerRefAndWeeklyTargetRoundTrip pins the two Screen-02
// columns through every path that reads a node: Create/Get, the Subtree CTE
// (which spells its columns out by hand and would silently rot otherwise) and
// Update. Update deliberately does NOT touch the weekly target — it mirrors
// rate, which has its own setter for the same reason.
func TestNodeStore_BannerRefAndWeeklyTargetRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	u, _ := domain.NewUser("u-banner", "sub-banner", "banner", "banner@x.de", "Banner")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewNodeStore(pool)
	now := time.Now().UTC()

	n, _ := domain.NewNode("eng-banner", u.ID, "Engagement Banner", "eng-banner", now)
	n.Kind = domain.KindEngagement
	n.BannerRef = "a1b2c3d4e5f6"
	soll := 20 * time.Hour
	n.WeeklyTarget = &soll
	created, err := st.Create(ctx, n)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.BannerRef != "a1b2c3d4e5f6" {
		t.Errorf("create BannerRef=%q want a1b2c3d4e5f6", created.BannerRef)
	}
	if created.WeeklyTarget == nil || *created.WeeklyTarget != 20*time.Hour {
		t.Errorf("create WeeklyTarget=%v want 20h", created.WeeklyTarget)
	}

	got, err := st.Get(ctx, u.ID, n.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.BannerRef != "a1b2c3d4e5f6" {
		t.Errorf("get BannerRef=%q want a1b2c3d4e5f6", got.BannerRef)
	}
	if got.WeeklyTarget == nil || *got.WeeklyTarget != 20*time.Hour {
		t.Errorf("get WeeklyTarget=%v want 20h", got.WeeklyTarget)
	}

	sub, err := st.Subtree(ctx, u.ID, n.ID)
	if err != nil {
		t.Fatalf("subtree: %v", err)
	}
	if len(sub) != 1 {
		t.Fatalf("subtree len=%d want 1", len(sub))
	}
	if sub[0].BannerRef != "a1b2c3d4e5f6" || sub[0].WeeklyTarget == nil {
		t.Errorf("subtree dropped the Screen-02 columns: BannerRef=%q WeeklyTarget=%v", sub[0].BannerRef, sub[0].WeeklyTarget)
	}

	// Clearing the banner goes through Update; the weekly target must survive it.
	got.BannerRef = ""
	got.UpdatedAt = now.Add(time.Minute)
	upd, err := st.Update(ctx, u.ID, got)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if upd.BannerRef != "" {
		t.Errorf("update BannerRef=%q want cleared", upd.BannerRef)
	}
	if upd.WeeklyTarget == nil || *upd.WeeklyTarget != 20*time.Hour {
		t.Errorf("update must not touch WeeklyTarget, got %v", upd.WeeklyTarget)
	}
}

// TestNodeStore_SetWeeklyTarget covers the target's own setter. It mirrors
// SetRate: Update() leaves the column alone, so setting and clearing the
// weekly target is a separate, owner-scoped write.
func TestNodeStore_SetWeeklyTarget(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	u, _ := domain.NewUser("u-soll", "sub-soll", "soll", "soll@x.de", "Soll")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	st := pgstore.NewNodeStore(pool)
	n, _ := domain.NewNode("eng-soll", u.ID, "Engagement Soll", "eng-soll", time.Now().UTC())
	n.Kind = domain.KindEngagement
	if _, err := st.Create(ctx, n); err != nil {
		t.Fatalf("create: %v", err)
	}

	soll := 20 * time.Hour
	if err := st.SetWeeklyTarget(ctx, u.ID, n.ID, &soll); err != nil {
		t.Fatalf("set weekly target: %v", err)
	}
	got, err := st.Get(ctx, u.ID, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.WeeklyTarget == nil || *got.WeeklyTarget != 20*time.Hour {
		t.Errorf("WeeklyTarget=%v want 20h", got.WeeklyTarget)
	}

	if err := st.SetWeeklyTarget(ctx, u.ID, n.ID, nil); err != nil {
		t.Fatalf("clear weekly target: %v", err)
	}
	cleared, err := st.Get(ctx, u.ID, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.WeeklyTarget != nil {
		t.Errorf("WeeklyTarget=%v want nil after clearing", cleared.WeeklyTarget)
	}

	// Owner-scoped: a foreign owner must not reach this node, and the caller
	// learns only that it is not there.
	if err := st.SetWeeklyTarget(ctx, "u-fremd", n.ID, &soll); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Errorf("foreign owner: want ErrNodeNotFound, got %v", err)
	}
	if err := st.SetWeeklyTarget(ctx, u.ID, "ghost", &soll); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Errorf("unknown node: want ErrNodeNotFound, got %v", err)
	}
}
