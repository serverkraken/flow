package testutil

// Sanity tests for the in-package fakes. They lift the kompendium/testutil
// package from 0% to ~95% so CI's per-package coverage gate stops being
// dragged down by test-infrastructure code. Each fake gets a happy-path
// test and an Err-injection test for the failure branches.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

var errInjected = errors.New("injected")

func mustNote(t *testing.T, id, title string, typ domain.NoteType, project, body string) domain.Note {
	t.Helper()
	fm := domain.Frontmatter{ID: id, Type: typ, Title: title, Project: project}
	n, err := domain.NewNote(domain.ID(id), fm, []byte(body))
	if err != nil {
		t.Fatalf("NewNote(%s): %v", id, err)
	}
	return n
}

// — fake_store.go —

func TestFakeNoteStore_PathRootAndCRUD(t *testing.T) {
	s := NewFakeNoteStore()
	if got := s.Root(); got != "/fake-notebook" {
		t.Errorf("Root: %q", got)
	}
	n := mustNote(t, "notes/x", "X", domain.TypeFree, "", "body")
	if got := s.Path(n.ID); got != "/fake-notebook/"+n.ID.Path() {
		t.Errorf("Path: %q", got)
	}
	ctx := context.Background()
	if _, err := s.Get(ctx, n.ID); !errors.Is(err, ports.ErrNoteNotFound) {
		t.Errorf("Get missing should be ErrNoteNotFound, got %v", err)
	}
	if err := s.Put(ctx, n); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get(ctx, n.ID)
	if err != nil || got.ID != n.ID {
		t.Errorf("Get: (%+v, %v)", got, err)
	}
	exists, err := s.Exists(ctx, n.ID)
	if err != nil || !exists {
		t.Errorf("Exists: (%v, %v)", exists, err)
	}
	if err := s.Delete(ctx, n.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete(ctx, n.ID); !errors.Is(err, ports.ErrNoteNotFound) {
		t.Errorf("Delete missing should be ErrNoteNotFound, got %v", err)
	}
	exists2, _ := s.Exists(ctx, n.ID)
	if exists2 {
		t.Errorf("Exists after delete should be false")
	}
}

func TestFakeNoteStore_SeedAndListFilter(t *testing.T) {
	s := NewFakeNoteStore()
	a := mustNote(t, "daily/2026-05-01", "a", domain.TypeDaily, "", "")
	b := mustNote(t, "projects/foo/bar/2026-05-02", "b", domain.TypeProject, "github.com/foo/bar", "")
	c := mustNote(t, "notes/c", "c", domain.TypeFree, "", "")
	s.Seed(a, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	s.Seed(b, time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC))
	s.Seed(c, time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC))
	ctx := context.Background()

	all, err := s.List(ctx, ports.ListFilter{})
	if err != nil || len(all) != 3 {
		t.Errorf("List all: (%+v, %v)", all, err)
	}
	// Sorted by mtime DESC → c, b, a
	if all[0].ID != c.ID || all[2].ID != a.ID {
		t.Errorf("List should sort by mtime DESC, got %+v", all)
	}
	// Filter by type
	dailies, _ := s.List(ctx, ports.ListFilter{Type: domain.TypeDaily})
	if len(dailies) != 1 || dailies[0].ID != a.ID {
		t.Errorf("Type filter: %+v", dailies)
	}
	// Filter by project
	projs, _ := s.List(ctx, ports.ListFilter{Project: "github.com/foo/bar"})
	if len(projs) != 1 || projs[0].ID != b.ID {
		t.Errorf("Project filter: %+v", projs)
	}
	// Limit
	limited, _ := s.List(ctx, ports.ListFilter{Limit: 2})
	if len(limited) != 2 {
		t.Errorf("Limit=2: got %d", len(limited))
	}
}

func TestFakeNoteStore_ErrPaths(t *testing.T) {
	s := NewFakeNoteStore()
	s.GetErr = errInjected
	s.PutErr = errInjected
	s.DeleteErr = errInjected
	s.ExistsErr = errInjected
	s.ListErr = errInjected
	ctx := context.Background()
	n := mustNote(t, "notes/x", "X", domain.TypeFree, "", "")
	if _, err := s.Get(ctx, n.ID); err == nil {
		t.Errorf("Get with err should fail")
	}
	if err := s.Put(ctx, n); err == nil {
		t.Errorf("Put with err should fail")
	}
	if err := s.Delete(ctx, n.ID); err == nil {
		t.Errorf("Delete with err should fail")
	}
	if _, err := s.Exists(ctx, n.ID); err == nil {
		t.Errorf("Exists with err should fail")
	}
	if _, err := s.List(ctx, ports.ListFilter{}); err == nil {
		t.Errorf("List with err should fail")
	}
}

// — fake_indexer.go —

func TestFakeIndexer_UpsertSearchBacklinksLinks(t *testing.T) {
	ix := NewFakeIndexer()
	ctx := context.Background()
	// Two notes, one wikilinks the other.
	a := mustNote(t, "daily/2026-05-01", "Note A", domain.TypeDaily, "", "see [[notes/b]]")
	b := mustNote(t, "notes/b", "Note B", domain.TypeFree, "", "")
	if err := ix.Upsert(ctx, a, time.Now()); err != nil {
		t.Fatalf("Upsert a: %v", err)
	}
	if err := ix.Upsert(ctx, b, time.Now()); err != nil {
		t.Fatalf("Upsert b: %v", err)
	}
	// Search by title text
	res, err := ix.Search(ctx, domain.SearchQuery{Text: "Note A"})
	if err != nil || len(res) != 1 || res[0].ID != a.ID {
		t.Errorf("Search title: (%+v, %v)", res, err)
	}
	// Search by body text
	resBody, _ := ix.Search(ctx, domain.SearchQuery{Text: "see"})
	if len(resBody) != 1 || resBody[0].ID != a.ID {
		t.Errorf("Search body: %+v", resBody)
	}
	// Type filter
	resType, _ := ix.Search(ctx, domain.SearchQuery{Type: domain.TypeDaily})
	if len(resType) != 1 || resType[0].ID != a.ID {
		t.Errorf("Search type: %+v", resType)
	}
	// Empty filter returns both
	all, _ := ix.Search(ctx, domain.SearchQuery{})
	if len(all) != 2 {
		t.Errorf("Search empty: got %d, want 2", len(all))
	}
	// Limit
	lim, _ := ix.Search(ctx, domain.SearchQuery{Limit: 1})
	if len(lim) != 1 {
		t.Errorf("Search limit=1: got %d", len(lim))
	}
	// Project filter (no match)
	none, _ := ix.Search(ctx, domain.SearchQuery{Project: "x/y"})
	if len(none) != 0 {
		t.Errorf("Search project-no-match: %+v", none)
	}
	// Backlinks of b → should find a
	bl, err := ix.BacklinksOf(ctx, b.ID)
	if err != nil || len(bl) != 1 || bl[0].ID != a.ID {
		t.Errorf("BacklinksOf b: (%+v, %v)", bl, err)
	}
	// LinksFrom a → b
	lf, err := ix.LinksFrom(ctx, a.ID)
	if err != nil || len(lf) != 1 || lf[0].ID != b.ID {
		t.Errorf("LinksFrom a: (%+v, %v)", lf, err)
	}
	// LinksFrom unknown id → nil/nil
	lfNone, err := ix.LinksFrom(ctx, domain.ID("notes/missing"))
	if err != nil || lfNone != nil {
		t.Errorf("LinksFrom missing: (%+v, %v)", lfNone, err)
	}
	// Delete + rebuild
	if err := ix.Delete(ctx, a.ID); err != nil {
		t.Errorf("Delete: %v", err)
	}
	all2, _ := ix.Search(ctx, domain.SearchQuery{})
	if len(all2) != 1 {
		t.Errorf("After delete: %d entries left, want 1", len(all2))
	}
	rebuild := []ports.IndexEntry{{Note: a, Mtime: time.Now()}, {Note: b, Mtime: time.Now()}}
	if err := ix.Rebuild(ctx, rebuild); err != nil {
		t.Errorf("Rebuild: %v", err)
	}
	all3, _ := ix.Search(ctx, domain.SearchQuery{})
	if len(all3) != 2 {
		t.Errorf("After rebuild: %d, want 2", len(all3))
	}
}

func TestFakeIndexer_ErrPaths(t *testing.T) {
	ix := NewFakeIndexer()
	ix.UpsertErr = errInjected
	ix.DeleteErr = errInjected
	ix.SearchErr = errInjected
	ix.BacklinksErr = errInjected
	ix.LinksFromErr = errInjected
	ix.RebuildErr = errInjected
	ctx := context.Background()
	n := mustNote(t, "notes/x", "X", domain.TypeFree, "", "")
	if err := ix.Upsert(ctx, n, time.Now()); err == nil {
		t.Errorf("Upsert with err should fail")
	}
	if err := ix.Delete(ctx, n.ID); err == nil {
		t.Errorf("Delete with err should fail")
	}
	if _, err := ix.Search(ctx, domain.SearchQuery{}); err == nil {
		t.Errorf("Search with err should fail")
	}
	if _, err := ix.BacklinksOf(ctx, n.ID); err == nil {
		t.Errorf("BacklinksOf with err should fail")
	}
	if _, err := ix.LinksFrom(ctx, n.ID); err == nil {
		t.Errorf("LinksFrom with err should fail")
	}
	if err := ix.Rebuild(ctx, nil); err == nil {
		t.Errorf("Rebuild with err should fail")
	}
}

// — fake_editor.go —

func TestFakeEditor(t *testing.T) {
	e := &FakeEditor{}
	if err := e.Edit(context.Background(), "/p/a"); err != nil {
		t.Errorf("Edit: %v", err)
	}
	if len(e.Calls) != 1 || e.Calls[0] != "/p/a" {
		t.Errorf("Calls: %+v", e.Calls)
	}
	e.Err = errInjected
	if err := e.Edit(context.Background(), "/p/b"); err == nil {
		t.Errorf("Edit with err should fail")
	}
}

// — fake_legacy_source.go —

func TestFakeLegacySource(t *testing.T) {
	ls := &FakeLegacySource{
		Dailies:  []ports.LegacyDaily{{Path: "/a"}},
		Projects: []ports.LegacyProject{{Path: "/b"}},
	}
	d, err := ls.ListDailyNotes(context.Background(), "/")
	if err != nil || len(d) != 1 {
		t.Errorf("ListDailyNotes: (%+v, %v)", d, err)
	}
	p, err := ls.ListProjectNotes(context.Background(), "/")
	if err != nil || len(p) != 1 {
		t.Errorf("ListProjectNotes: (%+v, %v)", p, err)
	}
	ls.DailyErr = errInjected
	ls.ProjectErr = errInjected
	if _, err := ls.ListDailyNotes(context.Background(), "/"); err == nil {
		t.Errorf("ListDailyNotes with err should fail")
	}
	if _, err := ls.ListProjectNotes(context.Background(), "/"); err == nil {
		t.Errorf("ListProjectNotes with err should fail")
	}
}

// — fake_notebook_bundle.go —

func TestFakeNotebookBundle(t *testing.T) {
	b := &FakeNotebookBundle{}
	if err := b.ExportBundle(context.Background(), "/r", "/o"); err != nil {
		t.Errorf("ExportBundle: %v", err)
	}
	if err := b.ImportBundle(context.Background(), "/r", "/o"); err != nil {
		t.Errorf("ImportBundle: %v", err)
	}
	if len(b.Exports) != 1 || len(b.Imports) != 1 {
		t.Errorf("calls: exp=%+v imp=%+v", b.Exports, b.Imports)
	}
	b.ExportErr = errInjected
	b.ImportErr = errInjected
	if err := b.ExportBundle(context.Background(), "/r", "/o"); err == nil {
		t.Errorf("ExportBundle with err should fail")
	}
	if err := b.ImportBundle(context.Background(), "/r", "/o"); err == nil {
		t.Errorf("ImportBundle with err should fail")
	}
}

// — fake_notebook_init.go —

func TestFakeNotebookInit(t *testing.T) {
	ni := &FakeNotebookInit{IsRepoValue: false}
	ctx := context.Background()
	if r, _ := ni.IsRepo(ctx, "/"); r {
		t.Errorf("default IsRepo should be false")
	}
	if err := ni.Init(ctx, "/"); err != nil {
		t.Errorf("Init: %v", err)
	}
	if !ni.Initialized || !ni.IsRepoValue {
		t.Errorf("Init should flip Initialized + IsRepoValue")
	}
	if r, _ := ni.IsRepo(ctx, "/"); !r {
		t.Errorf("after Init IsRepo should be true")
	}
	ni.HasChangesValue = true
	if h, _ := ni.HasUncommittedChanges(ctx, "/"); !h {
		t.Errorf("HasUncommittedChanges should reflect set value")
	}
	if err := ni.Snapshot(ctx, "/", "msg"); err != nil {
		t.Errorf("Snapshot: %v", err)
	}
	if len(ni.Snapshots) != 1 || ni.Snapshots[0] != "msg" {
		t.Errorf("Snapshots: %+v", ni.Snapshots)
	}
	if ni.HasChangesValue {
		t.Errorf("Snapshot should clear HasChangesValue")
	}
	// Err paths
	ni.IsRepoErr = errInjected
	if _, err := ni.IsRepo(ctx, "/"); err == nil {
		t.Errorf("IsRepo err")
	}
	ni.InitErr = errInjected
	if err := ni.Init(ctx, "/"); err == nil {
		t.Errorf("Init err")
	}
	ni.HasChangesErr = errInjected
	if _, err := ni.HasUncommittedChanges(ctx, "/"); err == nil {
		t.Errorf("HasUncommittedChanges err")
	}
	ni.SnapshotErr = errInjected
	if err := ni.Snapshot(ctx, "/", "x"); err == nil {
		t.Errorf("Snapshot err")
	}
}

// — fake_notebook_remote.go —

func TestFakeNotebookRemote(t *testing.T) {
	r := &FakeNotebookRemote{URL: "git@x"}
	ctx := context.Background()
	got, err := r.GetRemote(ctx, "/")
	if err != nil || got != "git@x" {
		t.Errorf("GetRemote: (%q, %v)", got, err)
	}
	if err := r.SetRemote(ctx, "/", "git@new"); err != nil {
		t.Errorf("SetRemote: %v", err)
	}
	if r.SetURL != "git@new" {
		t.Errorf("SetURL: %q", r.SetURL)
	}
	r.Stats = ports.SyncStats{Pulled: true, Pushed: true}
	stats, err := r.Sync(ctx, "/root")
	if err != nil || !stats.Pulled || !stats.Pushed || r.SyncRoot != "/root" {
		t.Errorf("Sync: (%+v, %v) root=%q", stats, err, r.SyncRoot)
	}
	// Err paths
	r.GetErr = errInjected
	if _, err := r.GetRemote(ctx, "/"); err == nil {
		t.Errorf("GetRemote err")
	}
	r.SetErr = errInjected
	if err := r.SetRemote(ctx, "/", "x"); err == nil {
		t.Errorf("SetRemote err")
	}
	r.SyncErr = errInjected
	if _, err := r.Sync(ctx, "/"); err == nil {
		t.Errorf("Sync err")
	}
}

// — fake_repo.go —

func TestFakeRepoDetector(t *testing.T) {
	d := &FakeRepoDetector{Info: ports.RepoInfo{Root: "/r"}}
	info, err := d.Detect(context.Background(), ".")
	if err != nil || info.Root != "/r" {
		t.Errorf("Detect: (%+v, %v)", info, err)
	}
	d.Err = errInjected
	if _, err := d.Detect(context.Background(), "."); err == nil {
		t.Errorf("Detect err")
	}
}

// — fake_tar_snapshot.go —

func TestFakeTarSnapshot(t *testing.T) {
	ts := &FakeTarSnapshot{}
	if err := ts.Export(context.Background(), "/s", "/o"); err != nil {
		t.Errorf("Export: %v", err)
	}
	if err := ts.Import(context.Background(), "/a", "/t", ports.ConflictAbort); err != nil {
		t.Errorf("Import: %v", err)
	}
	if len(ts.Exports) != 1 || len(ts.Imports) != 1 {
		t.Errorf("calls: %+v %+v", ts.Exports, ts.Imports)
	}
	ts.ExportErr = errInjected
	ts.ImportErr = errInjected
	if err := ts.Export(context.Background(), "/", "/"); err == nil {
		t.Errorf("Export err")
	}
	if err := ts.Import(context.Background(), "/", "/", ports.ConflictAbort); err == nil {
		t.Errorf("Import err")
	}
}

// — fixed_clock.go —

func TestFakeFixedClock(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	c := FixedClock{Time: t0}
	if !c.Now().Equal(t0) {
		t.Errorf("Now: %s want %s", c.Now(), t0)
	}
}
