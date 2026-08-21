package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// NodeAggregateStore commits a node and its dependent rate, tags and logo in
// one PostgreSQL transaction. It deliberately shares the same pool as the
// read-oriented NodeStore/TagStore/NodeLogoStore adapters.
type NodeAggregateStore struct {
	pool *pgxpool.Pool
	ids  ports.IDGen
}

func NewNodeAggregateStore(pool *pgxpool.Pool, ids ports.IDGen) *NodeAggregateStore {
	return &NodeAggregateStore{pool: pool, ids: ids}
}

func (s *NodeAggregateStore) CreateAggregate(ctx context.Context, n domain.Node, changes ports.NodeAggregateChanges) (domain.Node, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Node{}, fmt.Errorf("pgstore: begin node create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err := s.createAggregateTx(ctx, tx, n, changes)
	if err != nil {
		return domain.Node{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Node{}, fmt.Errorf("pgstore: commit node create: %w", err)
	}
	return created, nil
}

func (s *NodeAggregateStore) CreateBoundAggregate(ctx context.Context, n domain.Node, changes ports.NodeAggregateChanges, binding domain.ProjectBinding) (domain.Node, domain.ProjectBinding, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Node{}, domain.ProjectBinding{}, fmt.Errorf("pgstore: begin bound node create: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if binding.OwnerID != n.OwnerID || binding.NodeID != n.ID {
		return domain.Node{}, domain.ProjectBinding{}, errors.New("pgstore: bound node identity mismatch")
	}
	created, err := s.createAggregateTx(ctx, tx, n, changes)
	if err != nil {
		return domain.Node{}, domain.ProjectBinding{}, err
	}
	bound, err := upsertProjectBinding(ctx, tx, binding)
	if err != nil {
		return domain.Node{}, domain.ProjectBinding{}, fmt.Errorf("pgstore: create node binding: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Node{}, domain.ProjectBinding{}, fmt.Errorf("pgstore: commit bound node create: %w", err)
	}
	return created, bound, nil
}

func (s *NodeAggregateStore) createAggregateTx(ctx context.Context, tx pgx.Tx, n domain.Node, changes ports.NodeAggregateChanges) (domain.Node, error) {
	if changes.SetRate {
		n.Rate = changes.Rate
	}
	if changes.Logo == ports.NodeLogoPut {
		if changes.LogoValue.OwnerID != n.OwnerID || changes.LogoValue.NodeID != n.ID {
			return domain.Node{}, errors.New("pgstore: node create logo identity mismatch")
		}
		n.LogoRef = changes.LogoValue.Ref
	}
	if changes.Banner == ports.NodeBannerPut {
		if changes.BannerValue.OwnerID != n.OwnerID || changes.BannerValue.NodeID != n.ID {
			return domain.Node{}, errors.New("pgstore: node create banner identity mismatch")
		}
		n.BannerRef = changes.BannerValue.Ref
	}
	created, err := createNodeTx(ctx, tx, n)
	if err != nil {
		return domain.Node{}, err
	}
	if changes.SetTags {
		if err := s.setNodeTagsTx(ctx, tx, n.OwnerID, n.ID, changes.Tags); err != nil {
			return domain.Node{}, err
		}
	}
	if changes.Logo == ports.NodeLogoPut {
		if err := putNodeLogoTx(ctx, tx, changes.LogoValue); err != nil {
			return domain.Node{}, err
		}
	}
	if changes.Banner == ports.NodeBannerPut {
		if err := putNodeBannerTx(ctx, tx, changes.BannerValue); err != nil {
			return domain.Node{}, err
		}
	}
	return created, nil
}

func (s *NodeAggregateStore) UpdateAggregate(ctx context.Context, ownerID, nodeID string, mutate func(domain.Node) (domain.Node, ports.NodeAggregateChanges, error)) (domain.Node, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Node{}, fmt.Errorf("pgstore: begin node update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := getNodeForUpdate(ctx, tx, ownerID, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	n, changes, err := mutate(current)
	if err != nil {
		return domain.Node{}, err
	}
	if n.ID != current.ID || n.OwnerID != current.OwnerID {
		return domain.Node{}, errors.New("pgstore: node aggregate mutation changed identity")
	}
	switch changes.Logo {
	case ports.NodeLogoKeep:
		n.LogoRef = current.LogoRef
	case ports.NodeLogoPut:
		if changes.LogoValue.OwnerID != current.OwnerID || changes.LogoValue.NodeID != current.ID {
			return domain.Node{}, errors.New("pgstore: node update logo identity mismatch")
		}
		n.LogoRef = changes.LogoValue.Ref
	case ports.NodeLogoDelete:
		n.LogoRef = ""
	default:
		return domain.Node{}, errors.New("pgstore: invalid node logo mutation")
	}
	switch changes.Banner {
	case ports.NodeBannerKeep:
		n.BannerRef = current.BannerRef
	case ports.NodeBannerPut:
		if changes.BannerValue.OwnerID != current.OwnerID || changes.BannerValue.NodeID != current.ID {
			return domain.Node{}, errors.New("pgstore: node update banner identity mismatch")
		}
		n.BannerRef = changes.BannerValue.Ref
	case ports.NodeBannerDelete:
		n.BannerRef = ""
	default:
		return domain.Node{}, errors.New("pgstore: invalid node banner mutation")
	}
	if _, err := updateNodeTx(ctx, tx, ownerID, n); err != nil {
		return domain.Node{}, err
	}
	if changes.SetRate {
		if err := setNodeRateTx(ctx, tx, ownerID, nodeID, changes.Rate); err != nil {
			return domain.Node{}, err
		}
	}
	if changes.SetTags {
		if err := s.setNodeTagsTx(ctx, tx, ownerID, nodeID, changes.Tags); err != nil {
			return domain.Node{}, err
		}
	}
	switch changes.Logo {
	case ports.NodeLogoPut:
		if err := putNodeLogoTx(ctx, tx, changes.LogoValue); err != nil {
			return domain.Node{}, err
		}
	case ports.NodeLogoDelete:
		if err := deleteNodeLogoTx(ctx, tx, ownerID, nodeID); err != nil {
			return domain.Node{}, err
		}
	}
	switch changes.Banner {
	case ports.NodeBannerPut:
		if err := putNodeBannerTx(ctx, tx, changes.BannerValue); err != nil {
			return domain.Node{}, err
		}
	case ports.NodeBannerDelete:
		if err := deleteNodeBannerTx(ctx, tx, ownerID, nodeID); err != nil {
			return domain.Node{}, err
		}
	}
	final, err := getNodeTx(ctx, tx, ownerID, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Node{}, fmt.Errorf("pgstore: commit node update: %w", err)
	}
	return final, nil
}

func createNodeTx(ctx context.Context, tx pgx.Tx, n domain.Node) (domain.Node, error) {
	const q = `INSERT INTO nodes (` + nodeCols + `)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
RETURNING ` + nodeCols
	ra, rc := rateCols(n.Rate)
	ex := n.Extra
	if ex == nil {
		ex = map[string]any{}
	}
	got, err := scanNode(tx.QueryRow(ctx, q,
		n.ID, n.OwnerID, n.ParentID, string(n.Kind), n.Name, n.Slug, n.Color, n.Glyph,
		n.Description, n.UpstreamGit, nullStr(n.OriginSlug), string(n.Status), ra, rc, ex,
		n.CreatedAt, n.UpdatedAt, n.CountsTowardTarget, n.Icon, n.LogoRef, n.BannerRef,
		weeklyTargetMin(n.WeeklyTarget)))
	if err != nil {
		return domain.Node{}, mapSlugConflict(err)
	}
	return got, nil
}

func getNodeForUpdate(ctx context.Context, tx pgx.Tx, ownerID, nodeID string) (domain.Node, error) {
	const q = `SELECT ` + nodeCols + ` FROM nodes WHERE owner_id=$1 AND id=$2 FOR UPDATE`
	n, err := scanNode(tx.QueryRow(ctx, q, ownerID, nodeID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	return n, err
}

func getNodeTx(ctx context.Context, tx pgx.Tx, ownerID, nodeID string) (domain.Node, error) {
	const q = `SELECT ` + nodeCols + ` FROM nodes WHERE owner_id=$1 AND id=$2`
	n, err := scanNode(tx.QueryRow(ctx, q, ownerID, nodeID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	return n, err
}

func updateNodeTx(ctx context.Context, tx pgx.Tx, ownerID string, n domain.Node) (domain.Node, error) {
	const q = `UPDATE nodes SET name=$1, slug=$2, color=$3, glyph=$4, description=$5,
upstream_git=$6, origin_slug=$7, status=$8, extra=$9, counts_toward_target=$10,
icon=$11, logo_ref=$12, banner_ref=$13, updated_at=$14
WHERE owner_id=$15 AND id=$16 RETURNING ` + nodeCols
	ex := n.Extra
	if ex == nil {
		ex = map[string]any{}
	}
	got, err := scanNode(tx.QueryRow(ctx, q,
		n.Name, n.Slug, n.Color, n.Glyph, n.Description, n.UpstreamGit, nullStr(n.OriginSlug),
		string(n.Status), ex, n.CountsTowardTarget, n.Icon, n.LogoRef, n.BannerRef, n.UpdatedAt, ownerID, n.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	if err != nil {
		return domain.Node{}, mapSlugConflict(err)
	}
	return got, nil
}

func setNodeRateTx(ctx context.Context, tx pgx.Tx, ownerID, nodeID string, rate *domain.Money) error {
	ra, rc := rateCols(rate)
	tag, err := tx.Exec(ctx, `UPDATE nodes SET rate_amount=$1, rate_currency=$2 WHERE owner_id=$3 AND id=$4`, ra, rc, ownerID, nodeID)
	if err != nil {
		return fmt.Errorf("pgstore: set aggregate node rate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrNodeNotFound
	}
	return nil
}

func (s *NodeAggregateStore) setNodeTagsTx(ctx context.Context, tx pgx.Tx, ownerID, nodeID string, raw []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM taggings WHERE taggable_type=$1 AND taggable_id=$2
AND tag_id IN (SELECT id FROM tags WHERE owner_id=$3)`, string(domain.TaggableNode), nodeID, ownerID); err != nil {
		return fmt.Errorf("pgstore: clear aggregate node tags: %w", err)
	}
	seen := map[string]bool{}
	for _, display := range raw {
		slug, ok := domain.NormalizeTag(display)
		if !ok || seen[slug] {
			continue
		}
		seen[slug] = true
		var tagID string
		if err := tx.QueryRow(ctx, `INSERT INTO tags (id, owner_id, slug, display)
VALUES ($1,$2,$3,$4) ON CONFLICT (owner_id, slug) DO UPDATE SET slug=EXCLUDED.slug RETURNING id`,
			"tag-"+s.ids.NewID(), ownerID, slug, display).Scan(&tagID); err != nil {
			return fmt.Errorf("pgstore: upsert aggregate node tag: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO taggings (tag_id, taggable_type, taggable_id)
VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, tagID, string(domain.TaggableNode), nodeID); err != nil {
			return fmt.Errorf("pgstore: insert aggregate node tagging: %w", err)
		}
	}
	return nil
}

func putNodeLogoTx(ctx context.Context, tx pgx.Tx, l domain.NodeLogo) error {
	_, err := tx.Exec(ctx, `INSERT INTO node_logos (node_id, owner_id, mime, ref, bytes, updated_at, width, height)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (node_id) DO UPDATE SET mime=$3, ref=$4, bytes=$5, updated_at=$6, width=$7, height=$8
WHERE node_logos.owner_id=$2`,
		l.NodeID, l.OwnerID, l.Mime, l.Ref, l.Bytes, l.UpdatedAt, l.Width, l.Height)
	if err != nil {
		return fmt.Errorf("pgstore: put aggregate node logo: %w", err)
	}
	return nil
}

func deleteNodeLogoTx(ctx context.Context, tx pgx.Tx, ownerID, nodeID string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM node_logos WHERE owner_id=$1 AND node_id=$2`, ownerID, nodeID); err != nil {
		return fmt.Errorf("pgstore: delete aggregate node logo: %w", err)
	}
	return nil
}

func putNodeBannerTx(ctx context.Context, tx pgx.Tx, b domain.NodeBanner) error {
	_, err := tx.Exec(ctx, `INSERT INTO node_banners (node_id, owner_id, mime, ref, bytes, updated_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (node_id) DO UPDATE SET mime=$3, ref=$4, bytes=$5, updated_at=$6
WHERE node_banners.owner_id=$2`,
		b.NodeID, b.OwnerID, b.Mime, b.Ref, b.Bytes, b.UpdatedAt)
	if err != nil {
		return fmt.Errorf("pgstore: put aggregate node banner: %w", err)
	}
	return nil
}

func deleteNodeBannerTx(ctx context.Context, tx pgx.Tx, ownerID, nodeID string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM node_banners WHERE owner_id=$1 AND node_id=$2`, ownerID, nodeID); err != nil {
		return fmt.Errorf("pgstore: delete aggregate node banner: %w", err)
	}
	return nil
}

var _ ports.NodeAggregateStore = (*NodeAggregateStore)(nil)
var _ ports.NodeBindingAggregateStore = (*NodeAggregateStore)(nil)
