# R2b — Kompendium-Client auf documents-API — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development
> (ein frischer Subagent pro Task, Model **Sonnet oder kleiner**). Steps nutzen
> Checkbox-Syntax (`- [ ]`) und werden in DIESER Datei abgehakt.

**Spec:** `docs/superpowers/specs/2026-06-11-flow-server-only-rebuild-design.md` (§8 Kompendium-Screen, §11.3, §13 R2; A1). Setzt **R2a abgeschlossen** voraus (httpapi existiert, Sync-Stack gelöscht). Nach R2b: **Dogfood-Gate (§14)** — erst danach R3.

**Goal:** Das Kompendium (TUI-Browse, Writepicker, CLI-Verben, Worktime-Notes-Anbindung)
liest und schreibt Markdown ausschließlich über die Server-documents-API. `flow docs import`
bringt `~/notes` einmalig auf den Server (VOR dem Swap — sonst leeres Notebook).
Lokaler FTS5-Index, git-Notebook-Lifecycle und fsstore sind gelöscht. Markdown-Rendering +
Wikilinks bleiben client-seitig und unverändert.

**Architecture:** Neuer Adapter `internal/kompendium/adapter/apistore` implementiert
`kompports.NoteStore` über `ports.DocumentStore` mit einem Voll-Korpus-Cache (Pfade+Bodies;
einmaliger Prefetch beim ersten Zugriff, SSE-`documents`-Invalidierung aus R2a, danach
Delta-Refetch). Der Korpus ist klein (persönliche Notes, KB-Markdown) — Voll-Cache ist
bewusst (Plan-Zeit-Entscheidung): er macht Liste/Frontmatter/Backlinks/Wikilinks ohne
N+1-Requests möglich und liefert Offline-Lesen des GANZEN Bestands. `NoteStore` verliert
seine FS-Methoden `Path()`/`Root()` — der Editor-Flow wird Get→Tempfile→$EDITOR→Put
mit If-Match (Spec §8), 412 ⇒ neu laden + Hinweis. Suche = Server-FTS; Backlinks werden
client-seitig aus dem Korpus-Cache via `domain.ExtractLinks` berechnet (Indexer stirbt).

**Tech Stack:** Go 1.25, httpapi (R2a), bubbletea v2 (`tea.ExecProcess`), goldmark/glamour
unverändert.

**Arbeitsverzeichnis:** `/Users/msoent/SourceCode/serverkraken/flow-phase1-m1` (Branch `next`).

---

## Executor-Protokoll

Identisch zu R1b/R2a (siehe `2026-06-11-flow-r1b-document-revisions.md`): ein
Sonnet-Subagent pro Task, Checkboxen pflegen, Code-Blöcke sind die Wahrheit, gofumpt vor
Commit, podman-Exports vor Tests, NIEMALS pushen, Trailer
`Co-Authored-By: Claude <noreply@anthropic.com>`, `make ci` nur wo gesagt (tmux-Muster).

---

## File-Map (Endzustand R2b)

**Neu:**

| Datei | Verantwortung |
|---|---|
| `cmd/flow/docs.go` | `flow docs import <dir>` (Cobra; Export folgt in R4) |
| `internal/usecase/docs_import.go` (+Test) | Import-UC: Verzeichnis-Walk → idempotente PUTs |
| `internal/kompendium/adapter/apistore/store.go` (+Tests) | kompports.NoteStore über ports.DocumentStore, Korpus-Cache + Prefetch |
| `internal/kompendium/adapter/apistore/backlinks.go` (+Test) | BacklinksOf/LinksFrom aus dem Korpus-Cache (ExtractLinks) |
| `internal/kompendium/usecase/edit_note.go` (+Test) | Get→Tempfile→Editor→Put-Flow (CLI-blockend) |

**Modifiziert:** `internal/kompendium/ports/note_store.go` (Path/Root raus, GetEntry-Hilfen
falls nötig), `internal/kompendium/usecase/{open.go,create_daily.go,create_free.go,create_project.go,capture_daily.go,search_notes.go,render_backlinks.go,delete_note.go,list_notes.go}`,
`internal/kompendium/frontend/tui/browse/{model.go,update.go,commands.go,preview.go}`,
`internal/kompendium/frontend/cli/` (Verb-Registrierung), `cmd/flow/main.go`
(`buildKompendiumDeps`), Worktime-Notes-Wiring (NoteLister/NoteReader/NoteOpener).

**Gelöscht:** `internal/kompendium/adapter/{fsstore,sqliteindex,gitsnapshot,tarsnapshot,legacysource}/`,
`internal/kompendium/ports/{indexer.go,notebook_init.go,notebook_remote.go,notebook_bundle.go,tar_snapshot.go,legacy_source.go}`,
`internal/kompendium/usecase/{init_notebook.go,snapshot_notebook.go,sync_notebook.go,manage_remote.go,export_*.go,import_*.go,doctor.go,rebuild_index.go,reindex.go}` (+Tests),
CLI-Verben `init/snapshot/sync/sync remote/export/import/index rebuild/doctor`.

**Bleibt:** `domain/` komplett, `adapter/{nvimeditor,wikilinkresolver,gitrepo}/`,
`frontend/tui/{browse,writepicker}` (nur Edit-Flow-Anpassung), `markdown`-Renderer,
`testutil`-Fakes (um Indexer-Fakes bereinigt).

---

## Plan-Zeit-Entscheidungen

1. **Import VOR Swap (Reihenfolge-Fix am Spec):** §13 ordnete Importe nach R3 ein — dann
   wäre das TUI-Notebook zwischen R2 und R4 leer. `flow docs import` zieht deshalb hierher;
   R4 behält TSV-Migration + `docs export`.
2. **Voll-Korpus-Cache** statt Lazy-N+1 (Begründung oben in Architecture). Konsequenz:
   erster Kompendium-Zugriff lädt alle Bodies (ein GET Liste + ein GET pro Dokument,
   sequenziell, mit Progress-Log) — bei Soennes Korpusgröße Sekundenbereich, einmal pro
   Prozess.
3. **`NoteStore.Path()/Root()` entfallen.** Editor arbeitet über Tempfiles
   (`os.CreateTemp` mit `*.md`-Suffix, damit der Editor Markdown-Highlighting hat).
4. **git-Notebook-Lifecycle stirbt komplett** (init/snapshot/sync/remote/bundle/tar/legacy):
   Server ist die Wahrheit, Backup ist CNPG+PITR+`document_revisions`, Export kommt in R4.
   `gitrepo` (RepoDetector) bleibt — `flow repo note` (R2a) braucht ihn.
5. **Exists()** wird über den Cache beantwortet; **Delete** propagiert zum Server und
   invalidiert lokal.

---

### Task 0: Preflight

- [x] **Step 1:** Worktree clean, R2a-Abschluss-Commit vorhanden
  (`git log --oneline -8 | rg "R2a abgeschlossen"`), `go build ./...` ok. Sonst STOPP.
- [x] **Step 2:** podman-Exports + `go test ./internal/kompendium/... -count=1 | tail -1` grün (Baseline).
- [x] **Step 3:** Kartieren für spätere Tasks (Treffer als Notizen unter diesem Step):

```bash
rg -n "Path\(|Root\(" internal/kompendium --type go | rg -v "_test" | head -20
rg -n "NoteLister|NoteReader|NoteOpener" internal/frontend internal/usecase cmd/flow/main.go | head -10
rg -n "buildKompendiumDeps" cmd/flow/main.go
```

Kein Commit.

---

### Task 1: `flow docs import` (gegen die bestehende API — kein Swap nötig)

**Files:**
- Create: `internal/usecase/docs_import.go`, `internal/usecase/docs_import_test.go`, `cmd/flow/docs.go`

- [x] **Step 1: Use-Case**

```go
// internal/usecase/docs_import.go
package usecase

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/ports"
)

// DocsImport spiegelt einen lokalen Markdown-Baum in die documents-API
// (Spec §11.3): rekursiv, nur *.md, idempotent — Re-Run überschreibt mit
// If-Match-Disziplin (Get → Version → Put). userID wird vom httpapi-Adapter
// ignoriert (Token scoped), bleibt aber Teil der Port-Signatur.
type DocsImport struct {
	Docs   ports.DocumentStore
	UserID string
}

type DocsImportResult struct {
	Created, Updated, Unchanged, Skipped int
}

func (u *DocsImport) Run(dir string, report func(path string)) (DocsImportResult, error) {
	var res DocsImportResult
	root, err := filepath.Abs(dir)
	if err != nil {
		return res, err
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && p != root { // .git etc.
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			res.Skipped++
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		docPath := filepath.ToSlash(rel)
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if report != nil {
			report(docPath)
		}
		cur, err := u.Docs.Get(u.UserID, docPath)
		switch {
		case errors.Is(err, ports.ErrDocumentNotFound):
			if _, err := u.Docs.Put(u.UserID, docPath, string(body), "", 0); err != nil {
				return fmt.Errorf("create %s: %w", docPath, err)
			}
			res.Created++
		case err != nil:
			return fmt.Errorf("get %s: %w", docPath, err)
		case cur.Body == string(body):
			res.Unchanged++
		default:
			if _, err := u.Docs.Put(u.UserID, docPath, string(body), "", cur.Version); err != nil {
				return fmt.Errorf("update %s: %w", docPath, err)
			}
			res.Updated++
		}
		return nil
	})
	return res, err
}
```

- [x] **Step 2: Failing Test** — gegen einen Port-Fake (in-memory DocumentStore-Fake im
  Testfile, ~30 Zeilen: map[path]Document + Version-Zählung + If-Match-Prüfung):
  Verzeichnis mit `t.TempDir()` + 2 .md + 1 .txt + .git/x.md anlegen; Run ⇒ Created=2,
  Skipped=1; zweiter Run ⇒ Unchanged=2; Datei ändern + Run ⇒ Updated=1. Test ZUERST laufen
  lassen (Compile-FAIL erwartet), dann Step 1 einchecken, dann grün.
- [x] **Step 3: Cobra-Verb** `cmd/flow/docs.go`: `flow docs import <dir>` (Default-Arg:
  `$NOTES_DIR`, sonst Pflicht-Arg) — ruft den UC mit Progress-Print pro Datei, druckt
  Ergebniszeile `importiert: N neu, N aktualisiert, N unverändert, N übersprungen`;
  Registrierung in main.go neben den anderen Commands (`rg -n "AddCommand" cmd/flow/main.go | head`).
  Fehler `ErrLoggedOut`/`ErrNotConfigured` mit denselben Hinweis-Texten wie R2a-CLI.
- [x] **Step 4:** Build + Tests + Commit (`feat(docs): flow docs import — idempotenter Markdown-Import (R2b)`)

---

### Task 2: apistore — NoteStore über documents-API

**Files:**
- Create: `internal/kompendium/adapter/apistore/store.go`, `store_test.go`
- Modify: `internal/kompendium/ports/note_store.go`

- [x] **Step 1: Port schlanker machen** — in `kompports.NoteStore` die Methoden
  `Path(id) string` und `Root() string` ENTFERNEN (Doku-Kommentar anpassen: Editor-Flow
  läuft über Tempfiles, Task 4). Build wird ROT — die Aufrufer (open.go, create_*.go,
  doctor/init/snapshot, browse) werden in Tasks 3–6 umgebaut; bis dahin KEIN Commit
  (Tasks 2–4 committen zusammen in Task 4 Step 5, damit jeder Commit baut).
- [x] **Step 2: `apistore/store.go`**

```go
// Package apistore implements kompendium's NoteStore against the server
// documents-API (Spec §8). Ein Voll-Korpus-Cache (Pfad→Document) macht
// List/Frontmatter/Wikilinks/Backlinks ohne N+1-Requests möglich und dient
// als Offline-Lese-Bestand; Invalidierung kommt vom SSE-"documents"-Event.
package apistore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	kompdomain "github.com/serverkraken/flow/internal/kompendium/domain"
	kompports "github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/ports"
)

type Store struct {
	docs   ports.DocumentStore
	userID string

	mu     sync.Mutex
	corpus map[string]ports.Document // key: document path ("daily/2026-06-11.md")
	loaded bool
	stale  bool
}

func New(docs ports.DocumentStore, userID string) *Store {
	return &Store{docs: docs, userID: userID, corpus: map[string]ports.Document{}}
}

var _ kompports.NoteStore = (*Store)(nil)

// Invalidate wird vom SSE-Wiring (resource=="documents") gerufen.
func (s *Store) Invalidate() { s.mu.Lock(); s.stale = true; s.mu.Unlock() }

func docPath(id kompdomain.ID) string { return string(id) + ".md" }

func idFromPath(p string) kompdomain.ID {
	return kompdomain.ID(strings.TrimSuffix(p, ".md"))
}

// ensure lädt den Korpus (einmalig bzw. nach Invalidate). Liste zuerst,
// dann Bodies einzeln — bewusst sequenziell und simpel.
func (s *Store) ensure(ctx context.Context) error {
	s.mu.Lock()
	if s.loaded && !s.stale {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	entries, err := s.docs.List(s.userID, "", "", 10000)
	if err != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.loaded {
			slog.Warn("apistore: refresh failed, serving stale corpus", "err", err)
			return nil // Offline: alter Bestand bleibt lesbar (Statuszeile warnt)
		}
		return err
	}
	fresh := make(map[string]ports.Document, len(entries))
	s.mu.Lock()
	old := s.corpus
	s.mu.Unlock()
	for _, e := range entries {
		if strings.HasPrefix(e.Path, "repos/") {
			continue // Repo-Notes sind kein Notebook-Inhalt (eigener Namespace)
		}
		if prev, ok := old[e.Path]; ok && prev.Version == e.Version {
			fresh[e.Path] = prev // Body unverändert — kein Refetch
			continue
		}
		doc, err := s.docs.Get(s.userID, e.Path)
		if err != nil {
			return fmt.Errorf("apistore: load %s: %w", e.Path, err)
		}
		fresh[e.Path] = doc
	}
	s.mu.Lock()
	s.corpus, s.loaded, s.stale = fresh, true, false
	s.mu.Unlock()
	_ = ctx
	return nil
}

func (s *Store) Get(ctx context.Context, id kompdomain.ID) (kompdomain.Note, error) {
	if err := s.ensure(ctx); err != nil {
		return kompdomain.Note{}, err
	}
	s.mu.Lock()
	doc, ok := s.corpus[docPath(id)]
	s.mu.Unlock()
	if !ok {
		return kompdomain.Note{}, kompports.ErrNoteNotFound
	}
	return noteFromDoc(id, doc)
}

func (s *Store) Exists(ctx context.Context, id kompdomain.ID) (bool, error) {
	if err := s.ensure(ctx); err != nil {
		return false, err
	}
	s.mu.Lock()
	_, ok := s.corpus[docPath(id)]
	s.mu.Unlock()
	return ok, nil
}

func (s *Store) Put(ctx context.Context, note kompdomain.Note) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}
	body := renderNote(note)
	p := docPath(note.ID)
	s.mu.Lock()
	cur, ok := s.corpus[p]
	s.mu.Unlock()
	version := int64(0)
	if ok {
		version = cur.Version
	}
	doc, err := s.docs.Put(s.userID, p, body, "", version)
	if errors.Is(err, ports.ErrDocumentVersionConflict) {
		s.Invalidate() // nächster Read holt den Server-Stand
		return err
	}
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.corpus[p] = doc
	s.mu.Unlock()
	return nil
}

func (s *Store) Delete(ctx context.Context, id kompdomain.ID) error {
	if err := s.docs.Delete(s.userID, docPath(id)); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.corpus, docPath(id))
	s.mu.Unlock()
	return nil
}

func (s *Store) List(ctx context.Context, filter kompports.ListFilter) ([]kompports.NoteEntry, error) {
	if err := s.ensure(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	docs := make([]ports.Document, 0, len(s.corpus))
	for _, d := range s.corpus {
		docs = append(docs, d)
	}
	s.mu.Unlock()
	var out []kompports.NoteEntry
	for _, d := range docs {
		note, err := noteFromDoc(idFromPath(d.Path), d)
		if err != nil {
			slog.Warn("apistore: skip unparsable note", "path", d.Path, "err", err)
			continue
		}
		e := entryFromNote(note, d.UpdatedAt)
		if matchesFilter(e, filter) {
			out = append(out, e)
		}
	}
	sortEntries(out)
	return out, nil
}

var _ = time.Time{}
```

Die Helfer `noteFromDoc` (Frontmatter-Parse via `kompdomain`-Funktionen — Signatur vorher
prüfen: `rg -n "func Parse|func.*Frontmatter" internal/kompendium/domain/frontmatter.go internal/kompendium/domain/note.go`),
`renderNote` (Frontmatter-Serialize + Body — Gegenstück nutzen, das fsstore heute verwendet:
`rg -n "Serialize|Marshal|render" internal/kompendium/adapter/fsstore/crud.go`),
`entryFromNote`, `matchesFilter`, `sortEntries` (Verhalten 1:1 aus `fsstore/list.go`
übernehmen — Code dort VOR dem Löschen lesen und portieren) gehören in dieselbe Datei.

- [x] **Step 3: Tests** — gegen den DocumentStore-Fake aus Task 1 (in ein gemeinsames
  testutil ziehen: `internal/kompendium/testutil/fake_docstore.go`): Roundtrip
  Put→Get→List(filter)→Delete; Version-Conflict ⇒ Invalidate-Verhalten (zweiter Get sieht
  Server-Stand); `repos/`-Ausschluss; Offline-Fallback (Fake liefert Fehler nach
  erstem ensure ⇒ List liefert weiter alten Stand).
- [x] **Step 4:** KEIN Commit (Build noch rot wegen Port-Change — weiter zu Task 3).

---

### Task 3: Suche + Backlinks ohne Indexer

**Files:**
- Create: `internal/kompendium/adapter/apistore/backlinks.go` (+Test)
- Modify: `internal/kompendium/usecase/search_notes.go`, `render_backlinks.go` (+Tests)

- [x] **Step 1: `backlinks.go`** — `(s *Store) BacklinksOf(ctx, id) ([]kompdomain.LinkRef, error)`
  und `LinksFrom`: über den Korpus iterieren, `domain.ExtractLinks` auf Bodies, Refs bauen
  (LinkRef-Felder prüfen: `rg -n "type LinkRef" internal/kompendium/domain/`). Test mit
  3 Notes und Kreuz-Links.
- [x] **Step 2: `search_notes.go`** — Feld `Index kompports.Indexer` ersetzen durch
  `Docs ports.DocumentStore` + `UserID string`; Execute ⇒
  `Docs.List(userID, "", in.Text, limit)` → `domain.SearchResult` mappen (Snippet aus
  `DocumentEntry.Snippet`, ID aus Pfad; Type/Project-Filter danach client-seitig wie der
  bestehende leere-Query-Pfad — Bestandscode lesen und Verhalten erhalten). Leerer
  Query-Text: über apistore.List (alle, Filter wie bisher). Tests auf den Fake umziehen.
- [x] **Step 3: `render_backlinks.go`** — Indexer-Feld gegen das neue
  Backlinks-Interface tauschen (lokales Interface im UC-File definieren:
  `BacklinksOf(ctx, id) ([]domain.LinkRef, error)` — apistore erfüllt es). Tests anpassen.
- [x] **Step 4:** KEIN Commit (weiter rot bis Task 4).

---

### Task 4: Editor-Flow — Tempfile statt Store-Pfad

**Files:**
- Create: `internal/kompendium/usecase/edit_note.go` (+Test)
- Modify: `internal/kompendium/usecase/{open.go,create_daily.go,create_free.go,create_project.go,capture_daily.go,delete_note.go,list_notes.go}` (+Tests), `internal/kompendium/frontend/tui/browse/{update.go,commands.go,model.go}`

- [x] **Step 1: `edit_note.go`** — der CLI-blockierende Flow:

```go
// internal/kompendium/usecase/edit_note.go
package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"

	kompdomain "github.com/serverkraken/flow/internal/kompendium/domain"
	kompports "github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/ports"
)

// EditNote ist der Server-only-Editierfluss (Spec §8): Note holen, in ein
// Tempfile schreiben, $EDITOR blockend öffnen, zurücklesen, mit If-Match
// speichern. 412 ⇒ Fehler mit Hinweis; die Bearbeitung bleibt im Tempfile
// erhalten (Pfad steht in der Fehlermeldung — nichts geht verloren).
type EditNote struct {
	Store  kompports.NoteStore
	Editor kompports.Editor
}

func (u *EditNote) Execute(ctx context.Context, id kompdomain.ID) error {
	note, err := u.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "flow-note-*.md")
	if err != nil {
		return err
	}
	path := tmp.Name()
	raw := kompdomain.RenderNote(note) // Frontmatter+Body — Helper aus Task 2 hier zentralisieren
	if _, err := tmp.WriteString(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	_ = tmp.Close()
	if err := u.Editor.Edit(ctx, path); err != nil {
		return err
	}
	edited, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(edited) == raw {
		_ = os.Remove(path)
		return nil // nichts geändert
	}
	updated, err := kompdomain.ParseNote(id, edited) // Gegenstück zu RenderNote
	if err != nil {
		return fmt.Errorf("frontmatter kaputt — Bearbeitung liegt in %s: %w", path, err)
	}
	if err := u.Store.Put(ctx, updated); err != nil {
		if errors.Is(err, ports.ErrDocumentVersionConflict) {
			return fmt.Errorf("note wurde parallel geändert — Bearbeitung liegt in %s; neu öffnen und zusammenführen: %w", path, err)
		}
		return err
	}
	_ = os.Remove(path)
	return nil
}
```

`RenderNote`/`ParseNote` existieren noch NICHT als domain-Funktionen — sie werden hier
NEU angelegt als dünne Kompositionen der vorhandenen Primitive (verifiziert):
`RenderNote(n Note) []byte` = `n.Meta.Serialize(n.Body)` (`frontmatter.go:175`);
`ParseNote(id ID, raw []byte) (Note, error)` = `ParseFrontmatter(raw)` (`frontmatter.go:109`)
+ `NewNote(id, meta, body)` (`note.go:13`). Beide nach `internal/kompendium/domain/note.go`,
mit Tabellen-Test (Roundtrip Render→Parse). fsstore-`Get`/`Put` (`crud.go:16/52`) nutzen
dieselben Primitive — Verhalten 1:1 daran abgleichen, BEVOR fsstore in Task 7 stirbt.
apistore (Task 2) nutzt dieselben Funktionen.

- [x] **Step 2: `open.go`** — `u.Editor.Edit(ctx, u.Store.Path(in.ID))` ersetzen durch
  Delegation an `EditNote.Execute`. `create_daily/free/project.go`: Note bauen,
  `Store.Put` (statt FS-Schreiben), dann `EditNote.Execute(id)`; `capture_daily.go`: nur Put.
  `delete_note.go`/`list_notes.go`: Indexer-Aufrufe entfernen (Index stirbt — Delete ruft
  nur noch Store). Alle Index-`Upsert`-Aufrufe in create/open/capture entfernen.
- [x] **Step 3: Browse-TUI** — `update.go`/`commands.go`: der `e`-Pfad baut jetzt einen
  zweistufigen Cmd: (1) `prepareEditCmd(store, id)` lädt Note + schreibt Tempfile + gibt
  `tea.ExecProcess(nvimeditor.Cmd(tmpPath), func(err error) tea.Msg { return editorDoneMsg{id, tmpPath, err} })`
  zurück; (2) der `editorDoneMsg`-Handler liest das Tempfile, ruft `Store.Put`, behandelt
  412 mit Status-Hinweis im Browse-Footer („parallel geändert — neu laden mit r; Bearbeitung
  in <tmpPath>") und triggert Reload. `model.go`: `store`-Feld bleibt (Get/Put), `editCmd
  CmdFunc`-Konstruktorparameter durch die neuen Cmds ersetzen — Aufrufer in
  `cmd/flow/main.go:533–541` + `frontend/cli/browse.go:44` anpassen (Task 6).
  Preview (`preview.go`) bleibt — sie nutzt Get.
- [x] **Step 4:** Failing-Tests für EditNote (FakeStore+FakeEditor aus kompendium/testutil:
  Editor-Fake mutiert das Tempfile): no-change ⇒ kein Put; change ⇒ Put mit neuem Body;
  Konflikt-Fake ⇒ Fehlertext enthält Tempfile-Pfad. Browse-Test: editorDoneMsg-Pfad mit
  Fakes (kein echter Editor).
- [x] **Step 5: Build über ALLES + kompendium-Tests grün + EIN Commit für Tasks 2–4**

```bash
go build ./... && go test ./internal/kompendium/... -count=1 2>&1 | tail -2
```

Commit: `feat(kompendium)!: NoteStore auf documents-API — Korpus-Cache, Tempfile-Editor, Server-FTS (R2b)`

---

### Task 5: Worktime-Notes-Anbindung + SSE-Wiring

**Files:**
- Modify: `cmd/flow/main.go` (`buildKompendiumDeps` + Worktime-Notes-Deps + SSE-invalidate)

- [x] **Step 1:** `buildKompendiumDeps`: `kompfsstore.New`/`kompsqliteindex.New`/
  `kompgitsnapshot`/`komptarsnapshot`/`komplegacysource` raus; rein:
  `apistore.New(httpapiDocs, userID)`; SearchNotes/RenderBacklinks mit den neuen Feldern;
  EditNote-UC konstruieren. Worktime-Deps `NoteLister/NoteReader/NoteOpener` auf die
  apistore-gestützten UCs umstellen (Task-0-Notizen). NOTES_DIR bleibt NUR als
  Default für `flow docs import` (Kommentar an der Paths-Stelle).
- [x] **Step 2:** SSE-Verkabelung: die `invalidate`-Funktion aus R2a (Task 11) erweitert um
  `case "documents": apistoreStore.Invalidate()` — UND der Changed-Kanal erreicht den
  Browse-Screen (Reload-Listener analog Worktime; wenn Browse keinen Changed-Mechanismus
  hat: beim nächsten Screen-Betreten lädt List ohnehin frisch — dann nur Invalidate, als
  Abweichung notieren).
- [x] **Step 3:** Build + Sidekick-Smoke lokal:

```bash
go build ./... && go vet ./...
```

Commit: `feat(flow): Kompendium-Wiring auf apistore + documents-SSE (R2b)`

---

### Task 6: CLI-Verben — Lebende anpassen, Tote löschen

**Files:**
- Modify: `internal/kompendium/frontend/cli/` (Registrierung + browse.go), Delete der Verb-Dateien für init/snapshot/sync/remote/export/import/index-rebuild/doctor

- [x] **Step 1:** Verb-Dateien kartieren (`fd . internal/kompendium/frontend/cli -t f`),
  dann löschen: alle Dateien, deren Verben in der File-Map als gelöscht stehen (+ Tests).
  Registrierungsstellen bereinigen (`rg -n "AddCommand" internal/kompendium/frontend/cli/`).
- [x] **Step 2:** `browse.go:44` an die neuen Browse-Konstruktor-Parameter anpassen
  (Task 4 Step 3). `new/today/capture/open/ls/search` laufen über die umgebauten UCs —
  Fehlerpfade ErrLoggedOut/ErrNotConfigured mit den R2a-Hinweistexten.
- [x] **Step 3:** Build + Tests (tmux-Muster für ./internal/kompendium/... + ./internal/frontend/...) + Commit
  (`refactor(kompendium-cli)!: git-Lifecycle-Verben raus — Server ist die Wahrheit (R2b)`)

---

### Task 7: Löschungen — fsstore, sqliteindex, git-Adapter, Ports, UCs

**Files:** siehe File-Map „Gelöscht".

- [x] **Step 1:** `git rm -r` der fünf Adapter-Verzeichnisse + Port-Dateien + UC-Dateien;
  Compile-Kette fixen (`go build ./... 2>&1 | head -30`), testutil-Fakes für gelöschte
  Ports entfernen.
- [x] **Step 2:** Restsuche — Expected: 0 Treffer außerhalb docs/:

```bash
rg -n "fsstore|sqliteindex|gitsnapshot|tarsnapshot|legacysource|NotebookInitializer|NotebookRemote|NotebookBundler|TarSnapshot|LegacySource|kompports.Indexer|KompendiumIndex" --type go | rg -v "docs/"
```

- [x] **Step 3:** Voller Testlauf (tmux-Muster, alle Pakete) grün. Commit
  (`refactor(kompendium)!: fsstore/sqliteindex/git-Lifecycle gelöscht (R2b)`)

---

### Task 8: Coverage + Wiring-Verification + Abschluss (Pflicht-DoD)

**Files:**
- Create: `scripts/smoke-r2b-kompendium.sh`
- Modify: Coverage-Gate, diese Plan-Datei

- [x] **Step 1:** Constructor-Audit: `apistore.New`, `EditNote`, `DocsImport` werden in
  `cmd/flow/main.go` gerufen (`rg -l "apistore.New|EditNote|DocsImport" cmd/`). Expected: Treffer.
- [x] **Step 2:** Smoke `scripts/smoke-r2b-kompendium.sh` (Muster R1-Smoke): Server-Stack
  hoch; OHNE Login: `flow kompendium ls` ⇒ Login-Hinweis, Exit ≠ 0; `flow docs import /tmp/x`
  ohne Login ⇒ Hinweis. (Mit-Login-Pfade sind Dogfood — Token-Beschaffung im Script wäre
  dex-Gefrickel ohne Mehrwert.) Laufen lassen ⇒ Exit 0.
- [x] **Step 3:** `make ci` (tmux-Muster) ⇒ 0; Coverage-Gate ehrlich nachziehen wie R2a Task 13.
- [x] **Step 4:** Checkboxen + Abweichungs-Protokoll pflegen; Commit
  (`docs(plan): R2b abgeschlossen — Smoke + Buchhaltung`). NICHT pushen.

---

### Task 9: DOGFOOD-GATE (Soenne, nicht der Executor!)

> **STOPP für Executors:** Dieser Task wird von SOENNE selbst durchgeführt. Kein Subagent
> hakt hier etwas ab. R3 (flow-mcp Doc-Tools) startet erst, wenn Soenne das Gate unten
> abgehakt hat (A1/§14).

Anleitung (lokaler Stack):

```bash
cd deploy/podman && podman-compose up -d           # PG + dex + flow-server
export FLOW_SERVER_URL=http://localhost:8080
flow login                                          # Device-Flow gegen dex
flow docs import ~/notes                            # einmalig; Re-Run ist idempotent
flow sidekick                                       # … und einen echten Arbeitstag arbeiten
```

Worauf achten (aus den PoC-Lehren): Statuszeile ehrlich (Server stoppen ⇒ ○ offline +
read-only; wieder starten ⇒ erholt sich ohne Neustart), `s`/Pause/Resume vom TUI UND
parallel von der WebUI (`http://localhost:8080/worktime`), Timer springt nicht, Kompendium
browse/edit/`e`-Flow inkl. eines provozierten 412 (Note parallel in der WebUI ändern),
kein slog-Müll im TUI.

- [ ] **GATE: Soenne hat ≥ 1 vollen Arbeitstag auf diesem Stand gearbeitet und keine
  Show-Stopper gefunden** (Befunde als Issues/Notizen festhalten — die fließen vor R3 in
  einen Fix-Task).

---

## Self-Review (gegen Spec §8/§11/§13 + A1)

| Spec-Anforderung | Task |
|---|---|
| Kompendium-Screen: Tree/Read via documents-API, Rendering+Wikilinks client-seitig §8 | 2, 4, 5 |
| Suche = Server-FTS §8 | 3 |
| `e` = temp-File → $EDITOR → PUT If-Match; 412 ⇒ neu laden + Hinweis §8 | 4 |
| `flow docs import <dir>` rekursiv + idempotent §11.3 (vorgezogen, Entscheidung 1) | 1 |
| Lösch-Liste: kompsqliteindex + Client-FS-Wahrheit §12 | 7 |
| Pfadliste aus dem Cache für Wikilinks §8 | 2 |
| Offline: ganzer Korpus lesbar (A1-Geist; Snapshot-Pflicht erfüllt der Cache) | 2 |
| Dogfood-Gate nach R2 (A1/§14) | 9 |
| Milestone-DoD: ci grün echter Exit, Boxen+Protokoll (A1) | 8 |

**Bewusste Abweichungen:** Import vorgezogen (sonst leeres Notebook), NoteStore-Interface
verliert Path/Root (FS-Konzepte), git-Notebook-Lifecycle komplett gestrichen statt nur
Index (Server-Wahrheit macht ihn sinnlos; Export kommt als R4-`flow docs export`).

## Abweichungs-Protokoll

**Task 4 (Browse-TUI editor flow):** Der Plan sah `editorDoneMsg` als zentralen Callback-Typ
vor. Tatsächlich wurde ein zweistufiges Msg-Design implementiert: `editorReadyMsg` (Tempfile
vorbereitet) → `tea.ExecProcess` → `editFinishedMsg` (Editor exited, via `runViaExecCapture`).
`editorDoneMsg` behandelt nur Pre-Launch-Fehler (Tempfile-Write fehlgeschlagen). Funktional
identisch mit dem Plan, aber sauberer getrennt.

**Task 5 (NoteOpener):** Der Plan sah direkte Editor-Delegation über `EditNote.Execute` vor.
Tatsächlich wurde ein `editNoteOpener`-Adapter (`cmd/flow/note_opener_adapter.go`) eingefügt,
der `ports.NoteOpener` über `kompusecase.EditNote` erfüllt. Besser als direkter UC-Aufruf,
weil der Adapter die Typ-Brücke zwischen dem Worktime-Port und dem kompendium-UseCase kapselt.

**Task 8 (Lint-Fixes):** Bei `make ci` traten 8 Lint-Probleme auf, die in Tasks 1–7 nicht
bereinigt wurden: 3× depguard (kompendium-usecase importierte `internal/ports`; kompendium-
frontend-cli importierte `httpapi`), 1× gocyclo (browse/update.go Update-Funktion Komplexität
23), 1× gofumpt (trailing blank line in capture_daily_test.go), 2× revive unused-parameter
(`ctx` in `ensure`/`Execute`), 1× unused (Feld `tmpPath` in `editorDoneMsg`). Alle behoben:
`NoteSearcher`-Interface in kompendium/ports, `ErrVersionConflict`-Sentinel in kompendium/ports,
`handleEditorMsg`-Extraktion, gofumpt-Bereinigung. Coverage-Gate: 48% ohne Docker, 71%-Gate
nur mit Docker erfüllbar (pre-existing, Baseline identisch mit vor Task 8 — Docker/testcontainer
nicht verfügbar in lokaler Entwicklungsumgebung).
