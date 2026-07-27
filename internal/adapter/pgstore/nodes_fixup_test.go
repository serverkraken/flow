package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
)

func TestMigration0017_DataFixup(t *testing.T) {
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	// Stage the schema through 0016 (columns present, NO CHECKs yet) so we can
	// insert legacy-shaped rows (repos at the root) that the fixup will repair.
	if err := pgstore.MigrateUpTo(ctx, pool, 16); err != nil {
		t.Fatal(err)
	}

	users := pgstore.NewUserStore(pool)
	u, _ := domain.NewUser("u-fix", "sub-fix", "fixuser", "fix@x.de", "Fix")
	if _, err := users.UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 27, 9, 0, 0, 0, time.UTC)

	// Legacy repos at the root: 'flow' (rate set), 'gitlab-acme' (rate set).
	mustExec(t, pool, `INSERT INTO nodes (id, owner_id, name, slug, kind, rate_amount, rate_currency, created_at, updated_at)
		VALUES ('n-flow','u-fix','flow','flow','repo', 9500,'EUR',$1,$1)`, now)
	mustExec(t, pool, `INSERT INTO nodes (id, owner_id, name, slug, kind, rate_amount, rate_currency, created_at, updated_at)
		VALUES ('n-gl','u-fix','acme on gitlab','gitlab-acme','repo', 8000,'EUR',$1,$1)`, now)

	// Docs: daily, free, project — node_id starts pointing at the legacy repo.
	mustExec(t, pool, `INSERT INTO documents (id, owner_id, node_id, type, path, doc_date, created_at, updated_at)
		VALUES ('d-daily','u-fix','n-flow','daily','daily/2026-06-27',$1::date,$1,$1)`, now)
	mustExec(t, pool, `INSERT INTO documents (id, owner_id, node_id, type, path, created_at, updated_at)
		VALUES ('d-free','u-fix','n-flow','free','scratch/idea',$1,$1)`, now)
	mustExec(t, pool, `INSERT INTO documents (id, owner_id, node_id, type, path, created_at, updated_at)
		VALUES ('d-proj','u-fix','n-flow','project','flow/arch',$1,$1)`, now)

	// A session booked against the legacy repo.
	mustExec(t, pool, `INSERT INTO work_sessions (id, owner_id, node_id, start_at, stop_at, created_at)
		VALUES ('ws-1','u-fix','n-flow',$1,$2,$1)`, now, now.Add(time.Hour))

	// Run the data fixup + CHECKs.
	if err := pgstore.MigrateUpTo(ctx, pool, 17); err != nil {
		t.Fatalf("apply 0017: %v", err)
	}

	// --- engagements exist, are roots ---
	assertScalar(t, pool, `SELECT count(*) FROM nodes WHERE owner_id='u-fix' AND kind='engagement' AND parent_id IS NULL AND slug IN ('privat','rtl-extern')`, int64(2))

	// --- repos parented by rule: flow→privat, gitlab-acme→rtl-extern ---
	assertText(t, pool, `SELECT parent_id FROM nodes WHERE id='n-flow'`, "eng-privat-u-fix")
	assertText(t, pool, `SELECT parent_id FROM nodes WHERE id='n-gl'`, "eng-rtl-extern-u-fix")

	// --- rate audited into extra.legacy_rate and cleared ---
	assertScalar(t, pool, `SELECT count(*) FROM nodes WHERE id='n-flow' AND rate_amount IS NULL`, int64(1))
	assertText(t, pool, `SELECT extra->'legacy_rate'->>'amount' FROM nodes WHERE id='n-flow'`, "9500")

	// --- docs re-scoped ---
	assertText(t, pool, `SELECT node_id FROM documents WHERE id='d-daily'`, "eng-rtl-extern-u-fix")
	assertScalar(t, pool, `SELECT count(*) FROM documents WHERE id='d-free' AND node_id IS NULL`, int64(1))
	assertText(t, pool, `SELECT node_id FROM documents WHERE id='d-proj'`, "n-flow") // unchanged

	// --- session re-pointed to the engagement (flow's parent = privat) ---
	assertText(t, pool, `SELECT node_id FROM work_sessions WHERE id='ws-1'`, "eng-privat-u-fix")

	// --- CHECKs are live: a rootless repo is rejected ---
	if _, err := pool.Exec(ctx, `INSERT INTO nodes (id, owner_id, name, slug, kind, created_at, updated_at)
		VALUES ('bad','u-fix','bad','bad','repo',now(),now())`); err == nil {
		t.Error("nodes_root_is_engagement CHECK should reject a rootless repo")
	}
	// --- idempotency: re-running the data ops is a no-op (apply 0017 again is a goose no-op;
	//     re-running the UPDATEs by hand must not change parents) ---
	mustExec(t, pool, `UPDATE nodes r SET parent_id = e.id FROM nodes e
		WHERE r.kind='repo' AND r.parent_id IS NULL AND r.slug NOT IN ('privat','rtl-extern')
		  AND e.owner_id=r.owner_id AND e.kind='engagement'
		  AND e.slug = CASE WHEN r.slug ILIKE '%gitlab%' THEN 'rtl-extern' ELSE 'privat' END`)
	assertText(t, pool, `SELECT parent_id FROM nodes WHERE id='n-flow'`, "eng-privat-u-fix")
}

func mustExec(t *testing.T, pool *pgxpool.Pool, q string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func assertScalar(t *testing.T, pool *pgxpool.Pool, q string, want int64) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(context.Background(), q).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	if got != want {
		t.Errorf("%q = %d, want %d", q, got, want)
	}
}

func assertText(t *testing.T, pool *pgxpool.Pool, q, want string) {
	t.Helper()
	var got *string
	if err := pool.QueryRow(context.Background(), q).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	if got == nil || *got != want {
		t.Errorf("%q = %v, want %q", q, got, want)
	}
}
