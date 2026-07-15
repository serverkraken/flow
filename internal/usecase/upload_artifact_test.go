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

// newArtifactTestNode seeds a single owner-scoped node ("n1"/"u1") in a fresh
// FakeNodeStore, for the artifact usecase tests.
func newArtifactTestNode(t *testing.T, clk testutil.FakeClock) *testutil.FakeNodeStore {
	t.Helper()
	ns := testutil.NewFakeNodeStore()
	n, err := domain.NewNode("n1", "u1", "flow", "flow", clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	n.Kind = domain.KindEngagement
	if _, err := ns.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	return ns
}

func pdfBytes() []byte { return []byte("%PDF-1.4\n%mock pdf body for artifact tests\n%%EOF") }

func svgBytes() []byte {
	return []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`)
}

func TestUploadArtifact_Image_SlugFromName(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := newArtifactTestNode(t, clk)
	as := testutil.NewFakeArtifactStore()
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	got, err := uc.Execute(ctx, "u1", "n1", "My Photo.png", "application/octet-stream", pngPixel(t), "", "human", "soenne")
	if err != nil {
		t.Fatal(err)
	}
	if got.Mime != "image/png" {
		t.Errorf("mime = %q, want image/png (sniff authoritative over declared)", got.Mime)
	}
	if got.Width != 1 || got.Height != 1 {
		t.Errorf("dims = %dx%d, want 1x1", got.Width, got.Height)
	}
	if got.Slug != "my-photo" {
		t.Errorf("slug = %q, want my-photo", got.Slug)
	}
	if len(em.events) != 1 || em.events[0].Type != domain.EventArtifactCreated {
		t.Fatalf("want exactly one artifact.created, got %+v", em.events)
	}
	if em.events[0].Data["node"] != "n1" || em.events[0].Data["id"] != "my-photo" || em.events[0].Data["name"] != "My Photo.png" {
		t.Errorf("event data = %+v, want node=n1 id=my-photo name=\"My Photo.png\"", em.events[0].Data)
	}
}

func TestUploadArtifact_PDF_DownloadNoDims(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := newArtifactTestNode(t, clk)
	as := testutil.NewFakeArtifactStore()
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	got, err := uc.Execute(ctx, "u1", "n1", "report.pdf", "application/pdf", pdfBytes(), "", "human", "soenne")
	if err != nil {
		t.Fatal(err)
	}
	if got.Mime != "application/pdf" {
		t.Errorf("mime = %q, want application/pdf", got.Mime)
	}
	if got.Width != 0 || got.Height != 0 {
		t.Errorf("dims = %dx%d, want 0x0 for a non-image", got.Width, got.Height)
	}
	if got.IsImage() {
		t.Error("IsImage() true for a PDF")
	}
}

func TestUploadArtifact_SVG_Rejected(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := newArtifactTestNode(t, clk)
	as := testutil.NewFakeArtifactStore()
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	_, err := uc.Execute(ctx, "u1", "n1", "icon.svg", "image/svg+xml", svgBytes(), "", "human", "soenne")
	if !errors.Is(err, usecase.ErrArtifactBadType) {
		t.Errorf("err = %v, want ErrArtifactBadType", err)
	}
	if len(em.events) != 0 {
		t.Errorf("want no event on rejected SVG, got %+v", em.events)
	}
}

func TestUploadArtifact_TooLarge(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := newArtifactTestNode(t, clk)
	as := testutil.NewFakeArtifactStore()
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	oversized := make([]byte, domain.MaxArtifactBytes+1)
	_, err := uc.Execute(ctx, "u1", "n1", "huge.bin", "application/octet-stream", oversized, "", "human", "soenne")
	if !errors.Is(err, usecase.ErrArtifactTooLarge) {
		t.Errorf("err = %v, want ErrArtifactTooLarge", err)
	}
}

func TestUploadArtifact_SlugCollisionGetsSuffix(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := newArtifactTestNode(t, clk)
	as := testutil.NewFakeArtifactStore()
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	first, err := uc.Execute(ctx, "u1", "n1", "diagram.png", "application/octet-stream", pngPixel(t), "", "human", "soenne")
	if err != nil {
		t.Fatal(err)
	}
	if first.Slug != "diagram" {
		t.Fatalf("first slug = %q, want diagram", first.Slug)
	}
	second, err := uc.Execute(ctx, "u1", "n1", "diagram.png", "application/octet-stream", pngPixel(t), "", "human", "soenne")
	if err != nil {
		t.Fatal(err)
	}
	if second.Slug != "diagram-1" {
		t.Errorf("second slug = %q, want diagram-1", second.Slug)
	}
	if len(em.events) != 2 || em.events[1].Type != domain.EventArtifactCreated {
		t.Fatalf("want two artifact.created events, got %+v", em.events)
	}
	if em.events[1].Data["id"] != "diagram-1" {
		t.Fatalf("collision event id = %v, want diagram-1", em.events[1].Data["id"])
	}
}

func TestUploadArtifact_ReplaceSlugOverwritesAndUpdates(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := newArtifactTestNode(t, clk)
	as := testutil.NewFakeArtifactStore()
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	orig, err := uc.Execute(ctx, "u1", "n1", "diagram.png", "application/octet-stream", pngPixel(t), "", "human", "soenne")
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := uc.Execute(ctx, "u1", "n1", "diagram-v2.png", "application/pdf", pdfBytes(), orig.Slug, "human", "soenne")
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Slug != orig.Slug {
		t.Errorf("slug changed on replace: got %q, want %q", replaced.Slug, orig.Slug)
	}
	if replaced.Ref == orig.Ref {
		t.Error("ref did not change after replacing with different content")
	}
	if replaced.Name != "diagram-v2.png" {
		t.Errorf("name = %q, want updated diagram-v2.png", replaced.Name)
	}
	if replaced.ID != orig.ID || !replaced.CreatedAt.Equal(orig.CreatedAt) {
		t.Errorf("replace changed creation identity: original=%+v replaced=%+v", orig, replaced)
	}
	if replaced.CreatedByKind != orig.CreatedByKind || replaced.CreatedByRef != orig.CreatedByRef {
		t.Errorf("replace changed creator: original=%s/%s replaced=%s/%s",
			orig.CreatedByKind, orig.CreatedByRef, replaced.CreatedByKind, replaced.CreatedByRef)
	}
	if len(em.events) != 2 || em.events[1].Type != domain.EventArtifactUpdated {
		t.Fatalf("want [created, updated], got %+v", em.events)
	}
	// Only one row persists for the slug (upsert, not a second artifact).
	all, err := as.List(ctx, "u1", "n1")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("want 1 stored artifact after replace, got %d", len(all))
	}
}

func TestUploadArtifact_ReplaceRequiresExistingValidSlug(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := newArtifactTestNode(t, clk)
	as := testutil.NewFakeArtifactStore()
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	if _, err := uc.Execute(ctx, "u1", "n1", "replacement.pdf", "application/pdf", pdfBytes(), "missing", "human", "soenne"); !errors.Is(err, ports.ErrArtifactNotFound) {
		t.Fatalf("missing replace error = %v, want ErrArtifactNotFound", err)
	}
	if _, err := uc.Execute(ctx, "u1", "n1", "replacement.pdf", "application/pdf", pdfBytes(), "../invalid", "human", "soenne"); !errors.Is(err, domain.ErrInvalidArtifact) {
		t.Fatalf("invalid replace slug error = %v, want ErrInvalidArtifact", err)
	}
	if len(em.events) != 0 {
		t.Fatalf("failed replaces emitted events: %+v", em.events)
	}
}

func TestUploadArtifact_ReplaceQuotaSubtractsOldSize(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := newArtifactTestNode(t, clk)
	as := testutil.NewFakeArtifactStore()
	original := domain.Artifact{
		ID: "original", OwnerID: "u1", NodeID: "n1", Slug: "report", Name: "report.pdf",
		Mime: "application/pdf", SizeBytes: usecase.MaxArtifactBytesPerOwner - 1,
		CreatedByKind: "human", CreatedByRef: "soenne", CreatedAt: clk.Now(), UpdatedAt: clk.Now(),
	}
	if err := as.Put(ctx, original); err != nil {
		t.Fatal(err)
	}
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	got, err := uc.Execute(ctx, "u1", "n1", "report-v2.pdf", "application/pdf", pdfBytes(), "report", "agent", "codex")
	if err != nil {
		t.Fatalf("smaller replacement must fit after subtracting old size: %v", err)
	}
	if got.ID != original.ID || got.CreatedByRef != original.CreatedByRef {
		t.Fatalf("replace did not preserve creation metadata: %+v", got)
	}
}

func TestUploadArtifact_QuotaExceeded(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := newArtifactTestNode(t, clk)
	as := testutil.NewFakeArtifactStore()
	// Pre-load the owner to just under the cap so any further upload tips it over.
	if err := as.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "n1", Slug: "existing", Name: "existing.pdf",
		Mime: "application/pdf", SizeBytes: usecase.MaxArtifactBytesPerOwner - 10,
	}); err != nil {
		t.Fatal(err)
	}
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	_, err := uc.Execute(ctx, "u1", "n1", "report.pdf", "application/pdf", pdfBytes(), "", "human", "soenne")
	if !errors.Is(err, usecase.ErrArtifactQuotaExceeded) {
		t.Errorf("err = %v, want ErrArtifactQuotaExceeded", err)
	}
	if len(em.events) != 0 {
		t.Errorf("want no event on quota rejection, got %+v", em.events)
	}
}

func TestUploadArtifact_UnknownNode(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := newArtifactTestNode(t, clk)
	as := testutil.NewFakeArtifactStore()
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	_, err := uc.Execute(ctx, "u1", "ghost", "x.pdf", "application/pdf", pdfBytes(), "", "human", "soenne")
	if !errors.Is(err, ports.ErrNodeNotFound) {
		t.Errorf("err = %v, want ErrNodeNotFound", err)
	}
}

// --- Free (node-less) artifacts (free-artifacts Task 3) --------------------

// TestUploadArtifact_Free_NoNodeGetCalled is the free-upload skip-guard
// mandatory test: nodeID=="" must never call Nodes.Get (the fake NodeStore
// here has ZERO nodes seeded — any Get call would return ErrNodeNotFound and
// fail the upload, so a successful free upload IS the proof the guard held).
func TestUploadArtifact_Free_NoNodeGetCalled(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := testutil.NewFakeNodeStore() // deliberately empty — no "" node exists
	as := testutil.NewFakeArtifactStore()
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	got, err := uc.Execute(ctx, "u1", "", "Brand.png", "application/octet-stream", pngPixel(t), "", "human", "soenne")
	if err != nil {
		t.Fatalf("free upload must not require a node: %v", err)
	}
	if got.NodeID != "" {
		t.Errorf("NodeID = %q, want empty (free)", got.NodeID)
	}
	if got.Slug != "brand" {
		t.Errorf("slug = %q, want brand", got.Slug)
	}
	if len(em.events) != 1 || em.events[0].Type != domain.EventArtifactCreated {
		t.Fatalf("want exactly one artifact.created, got %+v", em.events)
	}
	if em.events[0].Data["node"] != "" {
		t.Errorf("event data node = %v, want empty string (free)", em.events[0].Data["node"])
	}
}

// TestUploadArtifact_Free_SlugCollisionGetsSuffix mirrors
// TestUploadArtifact_SlugCollisionGetsSuffix for the free tier — the
// collision check goes through ExistingSlugs(owner, "") + nextArtifactSlug,
// the real usecase path (not just the pgstore Put-overwrite Task 1 covers).
func TestUploadArtifact_Free_SlugCollisionGetsSuffix(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := testutil.NewFakeNodeStore()
	as := testutil.NewFakeArtifactStore()
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	first, err := uc.Execute(ctx, "u1", "", "diagram.png", "application/octet-stream", pngPixel(t), "", "human", "soenne")
	if err != nil {
		t.Fatal(err)
	}
	if first.Slug != "diagram" {
		t.Fatalf("first slug = %q, want diagram", first.Slug)
	}
	second, err := uc.Execute(ctx, "u1", "", "diagram.png", "application/octet-stream", pngPixel(t), "", "human", "soenne")
	if err != nil {
		t.Fatal(err)
	}
	if second.Slug != "diagram-1" {
		t.Errorf("second slug = %q, want diagram-1", second.Slug)
	}
	if len(em.events) != 2 || em.events[1].Type != domain.EventArtifactCreated {
		t.Fatalf("want two artifact.created events, got %+v", em.events)
	}
}

// TestUploadArtifact_Free_ReplaceSlugOverwritesAndUpdates mirrors the node
// replace test for the free tier.
func TestUploadArtifact_Free_ReplaceSlugOverwritesAndUpdates(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := testutil.NewFakeNodeStore()
	as := testutil.NewFakeArtifactStore()
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	orig, err := uc.Execute(ctx, "u1", "", "diagram.png", "application/octet-stream", pngPixel(t), "", "human", "soenne")
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := uc.Execute(ctx, "u1", "", "diagram-v2.png", "application/pdf", pdfBytes(), orig.Slug, "human", "soenne")
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Slug != orig.Slug {
		t.Errorf("slug changed on replace: got %q, want %q", replaced.Slug, orig.Slug)
	}
	if replaced.Ref == orig.Ref {
		t.Error("ref did not change after replacing with different content")
	}
	if len(em.events) != 2 || em.events[1].Type != domain.EventArtifactUpdated {
		t.Fatalf("want [created, updated], got %+v", em.events)
	}
	all, err := as.ListFree(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("want 1 stored free artifact after replace, got %d", len(all))
	}
}

// TestUploadArtifact_Free_QuotaExceeded proves the owner quota applies to
// free uploads too (TotalBytes sums all owner artifacts including free ones —
// no code change needed, but it must be test-covered).
func TestUploadArtifact_Free_QuotaExceeded(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := testutil.NewFakeNodeStore()
	as := testutil.NewFakeArtifactStore()
	if err := as.Put(ctx, domain.Artifact{
		OwnerID: "u1", NodeID: "", Slug: "existing", Name: "existing.pdf",
		Mime: "application/pdf", SizeBytes: usecase.MaxArtifactBytesPerOwner - 10,
	}); err != nil {
		t.Fatal(err)
	}
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	_, err := uc.Execute(ctx, "u1", "", "report.pdf", "application/pdf", pdfBytes(), "", "human", "soenne")
	if !errors.Is(err, usecase.ErrArtifactQuotaExceeded) {
		t.Errorf("err = %v, want ErrArtifactQuotaExceeded", err)
	}
	if len(em.events) != 0 {
		t.Errorf("want no event on quota rejection, got %+v", em.events)
	}
}

func TestUploadArtifact_ActorKindPassedThrough(t *testing.T) {
	ctx := context.Background()
	clk := testutil.FakeClock{T: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	ns := newArtifactTestNode(t, clk)
	as := testutil.NewFakeArtifactStore()
	em := &recEmitter{}
	uc := usecase.UploadArtifact{Nodes: ns, Artifacts: as, IDs: &testutil.FakeIDGen{}, Clock: clk, Emitter: em}

	got, err := uc.Execute(ctx, "u1", "n1", "x.pdf", "application/pdf", pdfBytes(), "", "agent", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatedByKind != "agent" || got.CreatedByRef != "claude-code" {
		t.Errorf("CreatedByKind/Ref = %q/%q, want agent/claude-code", got.CreatedByKind, got.CreatedByRef)
	}
}
