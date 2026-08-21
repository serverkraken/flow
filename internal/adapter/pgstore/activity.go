package pgstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/serverkraken/flow/internal/domain"
)

// ActivityStore is the Postgres-backed ports.ActivityStore.
type ActivityStore struct{ pool *pgxpool.Pool }

// NewActivityStore wraps a pool. The store does NOT generate IDs — the Emitter
// sets entry.ID before calling Append (mirrors NewSessionStore).
func NewActivityStore(pool *pgxpool.Pool) *ActivityStore { return &ActivityStore{pool: pool} }

const activityCols = `id, owner_id, actor_kind, actor_ref, kind, target_ref, label, node_ref, at`

// Append inserts a pre-populated ActivityEntry. The caller (Emitter) must set
// entry.ID and entry.At before calling; if At is the zero value the DB default
// (now()) applies.
func (s *ActivityStore) Append(ctx context.Context, e domain.ActivityEntry) error {
	const q = `INSERT INTO activity (` + activityCols + `)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
	var at any = e.At
	if e.At.IsZero() {
		at = nil // let the DB DEFAULT apply
	}
	_, err := s.pool.Exec(ctx, q,
		e.ID, e.OwnerID, e.ActorKind, e.ActorRef, e.Kind,
		e.TargetRef, e.Label, e.NodeRef, at)
	if err != nil {
		return fmt.Errorf("pgstore: append activity: %w", err)
	}
	return nil
}

// ListPage returns one page of activity entries newest-first plus the total
// matching the owner/class/actor filter.
//
// classes matches kind prefixes (e.g. "session" matches "session.started");
// empty = all kinds. actorRef nil = any actor.
func (s *ActivityStore) ListPage(ctx context.Context, ownerID string, classes []string, actorRef *string, limit, offset int) ([]domain.ActivityEntry, int, error) {
	where := ` WHERE owner_id=$1`
	args := []any{ownerID}
	if len(classes) > 0 {
		// kind prefix match: kind LIKE 'session.%' OR kind LIKE 'document.%' ...
		ors := make([]string, 0, len(classes))
		for _, c := range classes {
			args = append(args, c+".%")
			ors = append(ors, fmt.Sprintf("kind LIKE $%d", len(args)))
		}
		where += " AND (" + strings.Join(ors, " OR ") + ")"
	}
	if actorRef != nil {
		args = append(args, *actorRef)
		where += fmt.Sprintf(" AND actor_ref=$%d", len(args))
	}

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM activity`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("pgstore: count activity: %w", err)
	}

	args = append(args, limit, offset)
	q := `SELECT ` + activityCols + ` FROM activity` + where +
		fmt.Sprintf(` ORDER BY at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("pgstore: list activity: %w", err)
	}
	defer rows.Close()
	out, err := scanActivities(rows)
	return out, total, err
}

// DistinctActors returns all distinct actor_refs for the given owner, sorted
// alphabetically. It always queries the full owner scope regardless of any
// class or actor filter, so dropdown options never shrink after filtering.
func (s *ActivityStore) DistinctActors(ctx context.Context, ownerID string) ([]string, error) {
	const q = `SELECT DISTINCT actor_ref FROM activity WHERE owner_id=$1 ORDER BY actor_ref`
	rows, err := s.pool.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("pgstore: distinct actors: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, fmt.Errorf("pgstore: scan actor_ref: %w", err)
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

func scanActivities(rows pgx.Rows) ([]domain.ActivityEntry, error) {
	var out []domain.ActivityEntry
	for rows.Next() {
		e, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanActivity(r rowScanner) (domain.ActivityEntry, error) {
	var e domain.ActivityEntry
	if err := r.Scan(
		&e.ID, &e.OwnerID, &e.ActorKind, &e.ActorRef, &e.Kind,
		&e.TargetRef, &e.Label, &e.NodeRef, &e.At,
	); err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("pgstore: scan activity: %w", err)
	}
	return e, nil
}

// ListPageForNodes returns one page of activity newest-first restricted to the
// given node set, plus the total inside that set. It exists so the register
// entry point's "last N" is TRUE: filtering an owner-wide page in the adapter
// silently drops a register's rows as soon as a louder neighbour fills the
// page, and no page size makes that reliable.
//
// An empty node set returns nothing rather than falling back to a broader
// read — a register with no subtree has no activity, and a silent widening
// would be a cross-register leak in disguise.
func (s *ActivityStore) ListPageForNodes(ctx context.Context, ownerID string, nodeIDs []string, limit, offset int) ([]domain.ActivityEntry, int, error) {
	if len(nodeIDs) == 0 {
		return nil, 0, nil
	}
	const where = ` WHERE owner_id=$1 AND node_ref = ANY($2)`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM activity`+where, ownerID, nodeIDs).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("pgstore: count subtree activity: %w", err)
	}
	q := `SELECT ` + activityCols + ` FROM activity` + where + ` ORDER BY at DESC LIMIT $3 OFFSET $4`
	rows, err := s.pool.Query(ctx, q, ownerID, nodeIDs, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("pgstore: list subtree activity: %w", err)
	}
	defer rows.Close()
	out, err := scanActivities(rows)
	return out, total, err
}

// DistinctAgentsSince counts the distinct AGENT actors that touched the given
// node set at or after since — the register head's "N Agenten heute aktiv".
// Humans are not counted: the line answers "who else worked here", and the
// viewer already knows about themselves.
func (s *ActivityStore) DistinctAgentsSince(ctx context.Context, ownerID string, nodeIDs []string, since time.Time) (int, error) {
	if len(nodeIDs) == 0 {
		return 0, nil
	}
	const q = `SELECT count(DISTINCT actor_ref) FROM activity
WHERE owner_id=$1 AND node_ref = ANY($2) AND actor_kind='agent' AND at >= $3`
	var n int
	if err := s.pool.QueryRow(ctx, q, ownerID, nodeIDs, since).Scan(&n); err != nil {
		return 0, fmt.Errorf("pgstore: count subtree agents: %w", err)
	}
	return n, nil
}
