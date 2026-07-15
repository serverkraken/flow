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

func newNodeAggregateFixture(t *testing.T) (context.Context, *pgstore.NodeStore, *pgstore.NodeLogoStore, *pgstore.TagStore, *pgstore.NodeAggregateStore, *pgstore.ProjectBindingStore, func(string, ...any)) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgstore.NewPool(ctx, startPG(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pgstore.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	u, _ := domain.NewUser("u-agg", "sub-agg", "agg", "agg@example.test", "Aggregate")
	if _, err := pgstore.NewUserStore(pool).UpsertBySub(ctx, u); err != nil {
		t.Fatal(err)
	}
	ids := &testutil.FakeIDGen{}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	return ctx, pgstore.NewNodeStore(pool), pgstore.NewNodeLogoStore(pool), pgstore.NewTagStore(pool, ids), pgstore.NewNodeAggregateStore(pool, ids), pgstore.NewProjectBindingStore(pool), exec
}

func aggregateNode(id string, now time.Time) domain.Node {
	n, _ := domain.NewNode(id, "u-agg", id, id, now)
	n.Kind = domain.KindEngagement
	return n
}

func aggregateLogo(nodeID, ref string, now time.Time) domain.NodeLogo {
	return domain.NodeLogo{
		NodeID: nodeID, OwnerID: "u-agg", Mime: "image/png", Ref: ref,
		Bytes: []byte(ref), UpdatedAt: now, Width: 1, Height: 1,
	}
}

func TestNodeAggregateStore_RollsBackCreateAndUpdateFollowFailures(t *testing.T) {
	ctx, nodes, logos, tags, agg, _, exec := newNodeAggregateFixture(t)
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	for _, id := range []string{"agg-rate", "agg-tags", "agg-logo"} {
		n := aggregateNode(id, now)
		logo := aggregateLogo(id, "old-"+id, now)
		if _, err := agg.CreateAggregate(ctx, n, ports.NodeAggregateChanges{
			SetRate: true, Rate: &domain.Money{Amount: 7000, Currency: "EUR"},
			SetTags: true, Tags: []string{"old"}, Logo: ports.NodeLogoPut, LogoValue: logo,
		}); err != nil {
			t.Fatal(err)
		}
	}

	exec(`CREATE FUNCTION test_fail_node_rate() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF NEW.id='agg-rate' THEN RAISE EXCEPTION 'rate failure'; END IF; RETURN NEW; END $$`)
	exec(`CREATE TRIGGER test_fail_node_rate BEFORE UPDATE OF rate_amount ON nodes
FOR EACH ROW EXECUTE FUNCTION test_fail_node_rate()`)
	exec(`CREATE FUNCTION test_fail_node_tags() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF NEW.taggable_id IN ('agg-tags','agg-create-tags') THEN RAISE EXCEPTION 'tags failure'; END IF; RETURN NEW; END $$`)
	exec(`CREATE TRIGGER test_fail_node_tags BEFORE INSERT ON taggings
FOR EACH ROW EXECUTE FUNCTION test_fail_node_tags()`)
	exec(`CREATE FUNCTION test_fail_node_logo() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (TG_OP='DELETE' AND OLD.node_id='agg-logo') OR (TG_OP='INSERT' AND NEW.node_id='agg-create-logo') THEN
    RAISE EXCEPTION 'logo failure';
  END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$`)
	exec(`CREATE TRIGGER test_fail_node_logo BEFORE INSERT OR DELETE ON node_logos
FOR EACH ROW EXECUTE FUNCTION test_fail_node_logo()`)

	newName := func(n domain.Node) domain.Node { n.Name = "changed"; n.UpdatedAt = now.Add(time.Hour); return n }
	_, err := agg.UpdateAggregate(ctx, "u-agg", "agg-rate", func(n domain.Node) (domain.Node, ports.NodeAggregateChanges, error) {
		return newName(n), ports.NodeAggregateChanges{SetRate: true, Rate: &domain.Money{Amount: 9000, Currency: "EUR"}}, nil
	})
	if err == nil {
		t.Fatal("rate follow failure did not abort")
	}
	_, err = agg.UpdateAggregate(ctx, "u-agg", "agg-tags", func(n domain.Node) (domain.Node, ports.NodeAggregateChanges, error) {
		return newName(n), ports.NodeAggregateChanges{SetTags: true, Tags: []string{"new"}}, nil
	})
	if err == nil {
		t.Fatal("tag follow failure did not abort")
	}
	_, err = agg.UpdateAggregate(ctx, "u-agg", "agg-logo", func(n domain.Node) (domain.Node, ports.NodeAggregateChanges, error) {
		return newName(n), ports.NodeAggregateChanges{Logo: ports.NodeLogoDelete}, nil
	})
	if err == nil {
		t.Fatal("logo follow failure did not abort")
	}

	for _, id := range []string{"agg-rate", "agg-tags", "agg-logo"} {
		got, err := nodes.Get(ctx, "u-agg", id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != id || got.Rate == nil || got.Rate.Amount != 7000 || got.LogoRef != "old-"+id {
			t.Fatalf("%s partially changed: %+v", id, got)
		}
		gotTags, err := tags.TagsFor(ctx, "u-agg", domain.TaggableNode, id)
		if err != nil || len(gotTags) != 1 || gotTags[0].Slug != "old" {
			t.Fatalf("%s tags changed: tags=%+v err=%v", id, gotTags, err)
		}
		gotLogo, err := logos.Get(ctx, "u-agg", id)
		if err != nil || gotLogo.Ref != "old-"+id {
			t.Fatalf("%s logo changed: logo=%+v err=%v", id, gotLogo, err)
		}
	}
	if _, err := agg.UpdateAggregate(ctx, "foreign-owner", "agg-rate", func(n domain.Node) (domain.Node, ports.NodeAggregateChanges, error) {
		n.Name = "stolen"
		return n, ports.NodeAggregateChanges{}, nil
	}); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("foreign aggregate update: want ErrNodeNotFound, got %v", err)
	}

	_, err = agg.CreateAggregate(ctx, aggregateNode("agg-create-tags", now), ports.NodeAggregateChanges{
		SetTags: true, Tags: []string{"new"},
	})
	if err == nil {
		t.Fatal("create survived tag failure")
	}
	_, err = agg.CreateAggregate(ctx, aggregateNode("agg-create-logo", now), ports.NodeAggregateChanges{
		Logo: ports.NodeLogoPut, LogoValue: aggregateLogo("agg-create-logo", "new-logo", now),
	})
	if err == nil {
		t.Fatal("create survived logo failure")
	}
	for _, id := range []string{"agg-create-tags", "agg-create-logo"} {
		if _, err := nodes.Get(ctx, "u-agg", id); !errors.Is(err, ports.ErrNodeNotFound) {
			t.Fatalf("partial create %s survived: %v", id, err)
		}
	}
}

func TestNodeAggregateStore_ConcurrentLogoUploadsStayConsistent(t *testing.T) {
	ctx, nodes, logos, _, agg, _, _ := newNodeAggregateFixture(t)
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	if _, err := agg.CreateAggregate(ctx, aggregateNode("agg-upload", now), ports.NodeAggregateChanges{}); err != nil {
		t.Fatal(err)
	}
	concurrentNodeAggregateMutations(t, func(i int) error {
		ref := []string{"ref-a", "ref-b"}[i]
		_, err := agg.UpdateAggregate(ctx, "u-agg", "agg-upload", func(n domain.Node) (domain.Node, ports.NodeAggregateChanges, error) {
			n.UpdatedAt = now.Add(time.Duration(i+1) * time.Minute)
			logo := aggregateLogo(n.ID, ref, n.UpdatedAt)
			return n, ports.NodeAggregateChanges{Logo: ports.NodeLogoPut, LogoValue: logo}, nil
		})
		return err
	})
	got, err := nodes.Get(ctx, "u-agg", "agg-upload")
	if err != nil {
		t.Fatal(err)
	}
	logo, err := logos.Get(ctx, "u-agg", "agg-upload")
	if err != nil || got.LogoRef != logo.Ref || (logo.Ref != "ref-a" && logo.Ref != "ref-b") {
		t.Fatalf("node/blob diverged: node=%+v logo=%+v err=%v", got, logo, err)
	}
}

func TestNodeAggregateStore_ConcurrentLogoDeleteAndUploadStayConsistent(t *testing.T) {
	ctx, nodes, logos, _, agg, _, _ := newNodeAggregateFixture(t)
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	if _, err := agg.CreateAggregate(ctx, aggregateNode("agg-delete-upload", now), ports.NodeAggregateChanges{
		Logo: ports.NodeLogoPut, LogoValue: aggregateLogo("agg-delete-upload", "old", now),
	}); err != nil {
		t.Fatal(err)
	}
	concurrentNodeAggregateMutations(t, func(i int) error {
		_, err := agg.UpdateAggregate(ctx, "u-agg", "agg-delete-upload", func(n domain.Node) (domain.Node, ports.NodeAggregateChanges, error) {
			n.UpdatedAt = now.Add(time.Duration(i+1) * time.Minute)
			if i == 0 {
				return n, ports.NodeAggregateChanges{Logo: ports.NodeLogoDelete}, nil
			}
			return n, ports.NodeAggregateChanges{Logo: ports.NodeLogoPut, LogoValue: aggregateLogo(n.ID, "new", n.UpdatedAt)}, nil
		})
		return err
	})
	got, err := nodes.Get(ctx, "u-agg", "agg-delete-upload")
	if err != nil {
		t.Fatal(err)
	}
	logo, logoErr := logos.Get(ctx, "u-agg", "agg-delete-upload")
	if got.LogoRef == "" {
		if !errors.Is(logoErr, ports.ErrNodeLogoNotFound) {
			t.Fatalf("empty ref retained blob: %+v err=%v", logo, logoErr)
		}
	} else if logoErr != nil || got.LogoRef != logo.Ref || logo.Ref != "new" {
		t.Fatalf("non-empty ref/blob diverged: node=%+v logo=%+v err=%v", got, logo, logoErr)
	}
}

func TestNodeAggregateStore_ConcurrentMetadataAndLogoUpdatesDoNotLoseEither(t *testing.T) {
	ctx, nodes, logos, _, agg, _, _ := newNodeAggregateFixture(t)
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	if _, err := agg.CreateAggregate(ctx, aggregateNode("agg-meta-logo", now), ports.NodeAggregateChanges{}); err != nil {
		t.Fatal(err)
	}
	concurrentNodeAggregateMutations(t, func(i int) error {
		_, err := agg.UpdateAggregate(ctx, "u-agg", "agg-meta-logo", func(n domain.Node) (domain.Node, ports.NodeAggregateChanges, error) {
			n.UpdatedAt = now.Add(time.Duration(i+1) * time.Minute)
			if i == 0 {
				n.Name = "renamed"
				return n, ports.NodeAggregateChanges{}, nil
			}
			return n, ports.NodeAggregateChanges{Logo: ports.NodeLogoPut, LogoValue: aggregateLogo(n.ID, "logo", n.UpdatedAt)}, nil
		})
		return err
	})
	got, err := nodes.Get(ctx, "u-agg", "agg-meta-logo")
	if err != nil {
		t.Fatal(err)
	}
	logo, err := logos.Get(ctx, "u-agg", "agg-meta-logo")
	if err != nil || got.Name != "renamed" || got.LogoRef != "logo" || logo.Ref != "logo" {
		t.Fatalf("concurrent updates lost state: node=%+v logo=%+v err=%v", got, logo, err)
	}
}

func TestNodeAggregateStore_CreateBoundAggregateRollsBackNodeWhenBindingFails(t *testing.T) {
	ctx, nodes, _, _, agg, bindings, exec := newNodeAggregateFixture(t)
	now := time.Date(2026, 7, 15, 13, 0, 0, 0, time.UTC)
	parent := aggregateNode("bound-parent", now)
	if _, err := agg.CreateAggregate(ctx, parent, ports.NodeAggregateChanges{}); err != nil {
		t.Fatal(err)
	}
	exec(`CREATE FUNCTION test_fail_bound_node_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF NEW.remote_slug='github.com/serverkraken/fail' THEN RAISE EXCEPTION 'binding failure'; END IF; RETURN NEW; END $$`)
	exec(`CREATE TRIGGER test_fail_bound_node_binding BEFORE INSERT ON project_bindings
FOR EACH ROW EXECUTE FUNCTION test_fail_bound_node_binding()`)

	failed, _ := domain.NewNode("bound-failed", "u-agg", "Failed", "failed", now)
	failed.Kind = domain.KindRepo
	failed.ParentID = &parent.ID
	failed.OriginSlug = "github.com/serverkraken/fail"
	failedBinding := domain.ProjectBinding{
		ID: "binding-failed", OwnerID: failed.OwnerID, NodeID: failed.ID,
		Kind: domain.BindingRemote, RemoteSlug: failed.OriginSlug,
		CreatedAt: now, UpdatedAt: now,
	}
	if _, _, err := agg.CreateBoundAggregate(ctx, failed, ports.NodeAggregateChanges{}, failedBinding); err == nil {
		t.Fatal("binding failure did not abort aggregate create")
	}
	if _, err := nodes.Get(ctx, failed.OwnerID, failed.ID); !errors.Is(err, ports.ErrNodeNotFound) {
		t.Fatalf("node survived failed binding write: %v", err)
	}
	gotBindings, err := bindings.List(ctx, failed.OwnerID)
	if err != nil || len(gotBindings) != 0 {
		t.Fatalf("binding survived failed aggregate: %+v err=%v", gotBindings, err)
	}

	created, _ := domain.NewNode("bound-created", "u-agg", "Created", "created", now)
	created.Kind = domain.KindRepo
	created.ParentID = &parent.ID
	created.OriginSlug = "github.com/serverkraken/created"
	wantBinding := domain.ProjectBinding{
		ID: "binding-created", OwnerID: created.OwnerID, NodeID: created.ID,
		Kind: domain.BindingRemote, RemoteSlug: created.OriginSlug,
		CreatedAt: now, UpdatedAt: now,
	}
	gotNode, gotBinding, err := agg.CreateBoundAggregate(ctx, created, ports.NodeAggregateChanges{}, wantBinding)
	if err != nil {
		t.Fatalf("successful aggregate: %v", err)
	}
	if gotNode.ID != created.ID || gotBinding.NodeID != created.ID || gotBinding.RemoteSlug != created.OriginSlug {
		t.Fatalf("node=%+v binding=%+v", gotNode, gotBinding)
	}
	ownerBindings, err := bindings.List(ctx, "u-agg")
	if err != nil || len(ownerBindings) != 1 || ownerBindings[0].NodeID != created.ID {
		t.Fatalf("owner bindings=%+v err=%v", ownerBindings, err)
	}
	foreignBindings, err := bindings.List(ctx, "foreign-owner")
	if err != nil || len(foreignBindings) != 0 {
		t.Fatalf("foreign owner can see binding: %+v err=%v", foreignBindings, err)
	}
}

func TestNodeAggregateStore_ConcurrentCreateBoundKeepsNodeAndBindingCardinalityEqual(t *testing.T) {
	ctx, nodes, _, _, agg, bindings, _ := newNodeAggregateFixture(t)
	now := time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)
	parent := aggregateNode("bound-race-parent", now)
	if _, err := agg.CreateAggregate(ctx, parent, ports.NodeAggregateChanges{}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			id := []string{"bound-race-a", "bound-race-b"}[i]
			n, _ := domain.NewNode(id, "u-agg", "Same", "same", now)
			n.Kind = domain.KindRepo
			n.ParentID = &parent.ID
			n.OriginSlug = "github.com/serverkraken/" + id
			_, _, err := agg.CreateBoundAggregate(ctx, n, ports.NodeAggregateChanges{}, domain.ProjectBinding{
				ID: "binding-" + id, OwnerID: n.OwnerID, NodeID: n.ID,
				Kind: domain.BindingRemote, RemoteSlug: n.OriginSlug,
				CreatedAt: now, UpdatedAt: now,
			})
			errCh <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	successes := 0
	conflicts := 0
	for err := range errCh {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ports.ErrNodeSlugTaken):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	children, err := nodes.Children(ctx, "u-agg", &parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	ownerBindings, err := bindings.List(ctx, "u-agg")
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 || len(ownerBindings) != 1 || ownerBindings[0].NodeID != children[0].ID {
		t.Fatalf("children=%+v bindings=%+v", children, ownerBindings)
	}
}

func concurrentNodeAggregateMutations(t *testing.T, run func(int) error) {
	t.Helper()
	start := make(chan struct{})
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errCh <- run(i)
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}
