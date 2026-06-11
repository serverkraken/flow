# R1b — document_revisions (A1-Sicherheitsnetz) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development
> (ein frischer Subagent pro Task, Model **Sonnet oder kleiner**). Steps nutzen
> Checkbox-Syntax (`- [ ]`) und werden in DIESER Datei abgehakt — sie sind der
> persistente Fortschrittszustand zwischen Sessions.

**Spec:** `docs/superpowers/specs/2026-06-11-flow-server-only-rebuild-design.md` (§6 Revisionen, §13 Punkt R1b, Amendment A1 — abgenommen)

**Goal:** Jeder erfolgreiche Write auf `documents` (PUT create/update, DELETE) hinterlässt
eine append-only Zeile in `document_revisions` — in derselben Transaktion. Damit existiert
ein Wiederherstellungspfad (psql), **bevor** R2/R3 Clients und insbesondere Claude (MCP)
Schreibzugriff bekommen.

**Architecture:** Reine pgstore-Änderung hinter dem unveränderten `ports.DocumentStore`-Port:
`Put`/`Delete` wechseln von Einzel-Statements auf `pgx.BeginFunc`-Transaktionen
(Write + Revision atomar). Kein API-, Handler- oder Port-Change; kein Read-Endpoint
(Phase 1: Restore via psql). Tabelle kommt in die **Baseline-Migration** — PG ist noch
nirgends deployed, Wegwerf-Datenbanken (Tests, compose) bauen sich neu auf.

**Tech Stack:** Go 1.25, pgx/v5 (`pgx.BeginFunc`), goose-PG-Baseline, testcontainers-PG.

**Arbeitsverzeichnis:** `/Users/msoent/SourceCode/serverkraken/flow-phase1-m1` (Branch `next`).
Alle Pfade relativ dazu.

---

## Executor-Protokoll (Sonnet-Subagents)

Pro Task EIN frischer Subagent (Model: Sonnet oder kleiner). Dispatch-Prompt pro Task:

> Öffne `docs/superpowers/plans/2026-06-11-flow-r1b-document-revisions.md` im Worktree
> `/Users/msoent/SourceCode/serverkraken/flow-phase1-m1`. Suche den ersten Task mit
> unerledigten Checkboxen. Führe NUR diesen einen Task aus: jeden Step exakt wie
> beschrieben, Kommandos ausführen, Output gegen „Expected" prüfen. Nach jedem Step die
> Checkbox auf `[x]` setzen. Am Task-Ende committen (Message steht im Task). Danach
> STOPPEN und berichten, was ggf. abgewichen ist (auch ins Abweichungs-Protokoll der
> Plan-Datei schreiben).

Regeln (bindend):

1. **Reihenfolge ist bindend.** Tasks bauen aufeinander auf; nie vorgreifen.
2. **Expected-Mismatch = Stopp.** Ursache beheben, solange es im Task-Scope bleibt; sonst
   Abweichung dokumentieren (`> **Abweichung:** …` unter dem Task + Abweichungs-Protokoll)
   und mit Bericht beenden — nicht improvisieren.
3. **Code-Blöcke sind die Wahrheit.** Gezeigten Code übernehmen; nur mechanische
   Anpassungen (Import-Sortierung, Compiler-Trivial-Fixes) erlaubt, alles darüber als
   Abweichung notieren.
4. **Vor jedem Commit:** `gofumpt -w <geänderte .go-Dateien>` (golangci-lint erzwingt das).
5. **Niemals:** `git push`, Branch wechseln, `CLAUDE-*.md` committen/löschen, Force-Operationen.
   Commits bleiben lokal auf `next` — Soenne pusht selbst.
6. **Commit-Messages** exakt wie angegeben, Abschluss-Zeile
   `Co-Authored-By: Claude <noreply@anthropic.com>`.
7. **testcontainers:** pgstore-Tests starten PG-Container via podman. In JEDER Session,
   die Tests ausführt, zuerst:
   ```bash
   export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
   export TESTCONTAINERS_RYUK_DISABLED=true
   ```
8. **`make ci` nur in Task 4** — zwischendurch reichen paketlokale Tests. `make ci` läuft
   wegen Bubbletea-TTY-Zugriffen in einer detached tmux-Session (Muster in Task 4), und der
   Exit-Code wird IMMER aus einer Status-Datei gelesen, nie aus einer Pipe.

---

## File-Map (Endzustand R1b)

| Datei | Änderung | Verantwortung |
|---|---|---|
| `internal/adapter/pgstore/migrations/0001_baseline.sql` | Modify | `document_revisions`-Tabelle (Up + Down) |
| `internal/adapter/pgstore/documents.go` | Modify | `Put`/`Delete` transaktional + `insertRevision` |
| `internal/adapter/pgstore/documents_revisions_test.go` | Create | Revisions-Verhalten (Save/Conflict/Delete-Marker) |

Sonst nichts. Ports, Handler, WebUI, API: unverändert.

---

### Task 0: Preflight

**Files:** keine Änderungen, nur Verifikation.

- [x] **Step 1: Worktree-Zustand prüfen**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-phase1-m1
git status --short && git branch --show-current && git log --oneline -1
```

Expected: keine `--short`-Ausgabe (clean), Branch `next`, HEAD ist `3f1ef0d` („docs(spec): A1
abgenommen") oder neuer. Wenn nicht clean: STOPP, Bericht.

- [x] **Step 2: podman-Socket exportieren + pingen**

```bash
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
export TESTCONTAINERS_RYUK_DISABLED=true
curl -s --unix-socket "${DOCKER_HOST#unix://}" http://d/_ping && echo " <- ping"
```

Expected: `OK <- ping`. Wenn nicht: `podman machine start`, dann wiederholen.

- [x] **Step 3: pgstore-Baseline grün**

```bash
go test ./internal/adapter/pgstore/ -count=1 2>&1 | tail -2
```

Expected: `ok  github.com/serverkraken/flow/internal/adapter/pgstore` (Container-Start
braucht ~10–20 s). Wenn rot: STOPP, Bericht — die Baseline muss grün sein.

Kein Commit in diesem Task.

---

### Task 1: Migration — `document_revisions` in die Baseline

**Files:**
- Modify: `internal/adapter/pgstore/migrations/0001_baseline.sql`

- [x] **Step 1: Tabelle in den Up-Teil einfügen**

In `internal/adapter/pgstore/migrations/0001_baseline.sql` direkt NACH der Zeile
`CREATE INDEX documents_search ON documents USING gin (search);` und VOR
`CREATE TABLE day_offs (` diesen Block einfügen:

```sql

-- A1: Sicherheitsnetz für einen LLM-beschreibbaren Korpus. Jeder erfolgreiche
-- documents-Write (PUT create/update, DELETE) schreibt den Stand zusätzlich
-- hierher — in derselben Transaktion. Kein FK auf documents(id): Revisionen
-- überleben das Löschen des Dokuments. Restore in Phase 1 via psql.
CREATE TABLE document_revisions (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    document_id uuid NOT NULL,
    user_id     uuid NOT NULL REFERENCES users(id),
    path        text NOT NULL,           -- Pfad zum Zeitpunkt des Writes
    body        text NOT NULL,
    version     bigint NOT NULL,         -- documents.version dieses Stands
    deleted     boolean NOT NULL DEFAULT false,  -- true = Lösch-Marker (body = letzter Stand)
    recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX document_revisions_doc ON document_revisions (document_id, version);
```

- [x] **Step 2: Down-Teil ergänzen**

Im `-- +goose Down`-Block als ERSTE Drop-Zeile (vor `DROP TABLE IF EXISTS user_settings;`)
einfügen:

```sql
DROP TABLE IF EXISTS document_revisions;
```

- [x] **Step 3: Migration beweist sich durch die bestehende Suite**

```bash
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
export TESTCONTAINERS_RYUK_DISABLED=true
go test ./internal/adapter/pgstore/ -count=1 2>&1 | tail -2
```

Expected: `ok` — der TestMain-Container fährt die Baseline inkl. neuer Tabelle hoch; ein
SQL-Fehler in der Migration würde JEDEN Test des Pakets brechen.

- [x] **Step 4: Commit**

```bash
git add internal/adapter/pgstore/migrations/0001_baseline.sql
git commit -m "$(cat <<'EOF'
feat(pgstore): document_revisions-Tabelle in der Baseline (A1, R1b)

Append-only Sicherheitsnetz vor dem MCP-Schreibzugriff (Spec §6/A1).
Kein FK auf documents: Revisionen überleben DELETE. Baseline-Edit statt
0002, weil PG noch nirgends deployed ist.

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Failing Tests — Revisions-Verhalten

**Files:**
- Create: `internal/adapter/pgstore/documents_revisions_test.go`

- [x] **Step 1: Test-Datei anlegen**

Datei `internal/adapter/pgstore/documents_revisions_test.go` mit exakt diesem Inhalt
anlegen:

```go
// internal/adapter/pgstore/documents_revisions_test.go
package pgstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/ports"
)

type revisionRow struct {
	DocumentID string
	Path       string
	Body       string
	Version    int64
	Deleted    bool
}

// revisionsFor liest alle Revisionszeilen eines Pfads in Insert-Reihenfolge.
// Direkt über den Pool — es gibt bewusst keinen Port dafür (Phase 1: psql-only).
func revisionsFor(t *testing.T, userID, path string) []revisionRow {
	t.Helper()
	rows, err := testStore.Pool().Query(context.Background(), `
		SELECT document_id, path, body, version, deleted
		FROM document_revisions
		WHERE user_id = $1 AND path = $2
		ORDER BY id ASC`, userID, path)
	if err != nil {
		t.Fatalf("query revisions: %v", err)
	}
	defer rows.Close()
	var out []revisionRow
	for rows.Next() {
		var r revisionRow
		if err := rows.Scan(&r.DocumentID, &r.Path, &r.Body, &r.Version, &r.Deleted); err != nil {
			t.Fatalf("scan revision: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

func TestDocuments_Put_WritesRevisionPerSave(t *testing.T) {
	t.Parallel()
	docs := pgstore.NewDocuments(testStore)
	uid := mustUser(t, "rev-1")
	const p = "projects/flow/revisions.md"

	created, err := docs.Put(uid, p, "v1 body", "", 0)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := docs.Put(uid, p, "v2 body", "", created.Version); err != nil {
		t.Fatalf("update: %v", err)
	}

	revs := revisionsFor(t, uid, p)
	if len(revs) != 2 {
		t.Fatalf("revisions: want 2, got %d (%+v)", len(revs), revs)
	}
	if revs[0].Body != "v1 body" || revs[0].Version != 1 || revs[0].Deleted {
		t.Errorf("rev[0]: want v1/&body/false, got %+v", revs[0])
	}
	if revs[1].Body != "v2 body" || revs[1].Version != 2 || revs[1].Deleted {
		t.Errorf("rev[1]: want v2/&body/false, got %+v", revs[1])
	}
	if revs[0].DocumentID != created.ID || revs[1].DocumentID != created.ID {
		t.Errorf("document_id: want %s in allen Revisionen, got %+v", created.ID, revs)
	}
}

func TestDocuments_Put_ConflictWritesNoRevision(t *testing.T) {
	t.Parallel()
	docs := pgstore.NewDocuments(testStore)
	uid := mustUser(t, "rev-2")
	const p = "projects/flow/conflict.md"

	if _, err := docs.Put(uid, p, "v1", "", 0); err != nil {
		t.Fatalf("create: %v", err)
	}
	// create-only auf existierenden Pfad → Konflikt, keine neue Revision
	if _, err := docs.Put(uid, p, "x", "", 0); !errors.Is(err, ports.ErrDocumentVersionConflict) {
		t.Fatalf("create on existing: want conflict, got %v", err)
	}
	// stale If-Match → Konflikt, keine neue Revision
	if _, err := docs.Put(uid, p, "stale", "", 99); !errors.Is(err, ports.ErrDocumentVersionConflict) {
		t.Fatalf("stale update: want conflict, got %v", err)
	}

	if revs := revisionsFor(t, uid, p); len(revs) != 1 {
		t.Errorf("revisions nach Konflikten: want 1, got %d (%+v)", len(revs), revs)
	}
}

func TestDocuments_Delete_WritesDeletedMarkerAndSurvives(t *testing.T) {
	t.Parallel()
	docs := pgstore.NewDocuments(testStore)
	uid := mustUser(t, "rev-3")
	const p = "projects/flow/deleted.md"

	if _, err := docs.Put(uid, p, "final body", "", 0); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := docs.Delete(uid, p); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Dokument weg …
	if _, err := docs.Get(uid, p); !errors.Is(err, ports.ErrDocumentNotFound) {
		t.Fatalf("after delete: want not found, got %v", err)
	}
	// … Revisionen überleben (kein FK): Save + Lösch-Marker mit letztem Body.
	revs := revisionsFor(t, uid, p)
	if len(revs) != 2 {
		t.Fatalf("revisions: want 2 (save + marker), got %d (%+v)", len(revs), revs)
	}
	marker := revs[1]
	if !marker.Deleted || marker.Body != "final body" || marker.Version != 1 {
		t.Errorf("marker: want deleted/final body/v1, got %+v", marker)
	}

	// Delete auf nicht-existenten Pfad bleibt idempotent und schreibt nichts.
	if err := docs.Delete(uid, p); err != nil {
		t.Fatalf("delete idempotent: %v", err)
	}
	if revs := revisionsFor(t, uid, p); len(revs) != 2 {
		t.Errorf("revisions nach No-op-Delete: want 2, got %d", len(revs))
	}
}
```

- [x] **Step 2: Tests laufen lassen — müssen FEHLSCHLAGEN**

```bash
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
export TESTCONTAINERS_RYUK_DISABLED=true
go test ./internal/adapter/pgstore/ -run 'TestDocuments_(Put_WritesRevisionPerSave|Put_ConflictWritesNoRevision|Delete_WritesDeletedMarkerAndSurvives)' -count=1 -v 2>&1 | rg "^(=== RUN|--- (PASS|FAIL)|FAIL|ok)" | head -12
```

Expected: alle drei Tests `--- FAIL` mit „want 2, got 0"-artigen Assertions (die Tabelle
existiert, aber niemand schreibt hinein). Kompilieren MUSS es — wenn Compile-Error: Tippfehler
in Step 1 fixen. Kein Commit (Tests committen erst grün zusammen mit der Implementierung —
Task 3).

---

### Task 3: Implementierung — Put/Delete transaktional mit Revision

**Files:**
- Modify: `internal/adapter/pgstore/documents.go`
- Test: `internal/adapter/pgstore/documents_revisions_test.go` (aus Task 2)

- [x] **Step 1: `Put` ersetzen**

In `internal/adapter/pgstore/documents.go` die KOMPLETTE bestehende `Put`-Methode (beginnt
mit `// Put upserts a document.`) durch diesen Block ersetzen:

```go
// Put upserts a document and appends the new state to document_revisions —
// both in one transaction (Spec §6/A1: jeder gespeicherte Stand ist eine
// Revision; If-Match schützt vor Races, die Revisionen vor Überschreib-Verlust).
func (d *Documents) Put(userID, path, body, repoKey string, ifMatch int64) (ports.Document, error) {
	ctx := context.Background()
	var repoKeyArg *string
	if repoKey != "" {
		repoKeyArg = &repoKey
	}
	var doc ports.Document
	err := pgx.BeginFunc(ctx, d.store.Pool(), func(tx pgx.Tx) error {
		var row pgx.Row
		if ifMatch == 0 {
			row = tx.QueryRow(ctx, `
				INSERT INTO documents (user_id, path, body, repo_key)
				VALUES ($1, $2, $3, $4)
				RETURNING `+documentCols,
				userID, path, body, repoKeyArg)
		} else {
			row = tx.QueryRow(ctx, `
				UPDATE documents
				SET body = $3, repo_key = COALESCE($4, repo_key), version = version + 1, updated_at = now()
				WHERE user_id = $1 AND path = $2 AND version = $5
				RETURNING `+documentCols,
				userID, path, body, repoKeyArg, ifMatch)
		}
		var err error
		doc, err = scanDocument(row)
		if err != nil {
			return err
		}
		return insertRevision(ctx, tx, doc, false)
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation: create-only auf existierenden Pfad
			return ports.Document{}, ports.ErrDocumentVersionConflict
		}
		if ifMatch != 0 && errors.Is(err, ports.ErrDocumentNotFound) {
			// UPDATE traf keine Row: Pfad fehlt oder Version stale → Konflikt (wie bisher)
			return ports.Document{}, ports.ErrDocumentVersionConflict
		}
		return ports.Document{}, err
	}
	return doc, nil
}
```

- [x] **Step 2: `Delete` ersetzen**

Die KOMPLETTE bestehende `Delete`-Methode (beginnt mit `// Delete deletes a document by
path.`) durch diesen Block ersetzen:

```go
// Delete removes a document and appends a deleted-marker revision carrying
// the last body — in one transaction. Deleting a missing path stays a no-op.
func (d *Documents) Delete(userID, path string) error {
	ctx := context.Background()
	return pgx.BeginFunc(ctx, d.store.Pool(), func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			DELETE FROM documents WHERE user_id = $1 AND path = $2
			RETURNING `+documentCols, userID, path)
		doc, err := scanDocument(row)
		if errors.Is(err, ports.ErrDocumentNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return insertRevision(ctx, tx, doc, true)
	})
}
```

- [x] **Step 3: `insertRevision`-Helper anfügen**

ANS ENDE von `internal/adapter/pgstore/documents.go` (nach `scanDocument`) anfügen:

```go
// insertRevision appends one row to document_revisions inside tx. deleted
// marks the Lösch-Marker; doc carries the state being recorded.
func insertRevision(ctx context.Context, tx pgx.Tx, doc ports.Document, deleted bool) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO document_revisions (document_id, user_id, path, body, version, deleted)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		doc.ID, doc.UserID, doc.Path, doc.Body, doc.Version, deleted)
	return err
}
```

Hinweis Imports: `errors`, `pgx`, `pgconn`, `context` sind in der Datei bereits importiert —
es sollte KEIN Import-Edit nötig sein. `fmt`/`strings` bleiben (List nutzt sie).

- [x] **Step 4: Revisions-Tests grün**

```bash
export DOCKER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
export TESTCONTAINERS_RYUK_DISABLED=true
go test ./internal/adapter/pgstore/ -run 'TestDocuments_' -count=1 2>&1 | tail -2
```

Expected: `ok` — die drei neuen Tests UND die bestehenden `TestDocuments_*`
(PutGetUpdateDelete, RepoKeyAlias, ListPrefixAndFTS) grün. Letztere beweisen, dass die
Konflikt-Semantik (create-on-existing, stale If-Match, idempotentes Delete) exakt erhalten
blieb.

- [x] **Step 5: Ganzes Paket + Konsumenten grün**

```bash
go build ./... && go test ./internal/adapter/pgstore/ ./internal/adapter/httpserver/ ./internal/webui/handlers/ -count=1 2>&1 | tail -4
```

Expected: 3× `ok` — Bearer-API- und WebUI-Handler-Tests laufen gegen den geänderten Store
(gleiche Container-Pattern) und dürfen nichts merken.

- [x] **Step 6: Commit**

```bash
gofumpt -w internal/adapter/pgstore/documents.go
git add internal/adapter/pgstore/documents.go internal/adapter/pgstore/documents_revisions_test.go
git commit -m "$(cat <<'EOF'
feat(pgstore): Revision bei jedem documents-Write (A1, R1b)

Put/Delete laufen jetzt in pgx.BeginFunc-Transaktionen und hängen den
neuen Stand bzw. einen Lösch-Marker (letzter Body) an document_revisions
an. Konflikt-Semantik unverändert (Tests decken create-on-existing,
stale If-Match, idempotentes Delete). Kein Port-/API-Change — Restore
in Phase 1 bewusst nur via psql.

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Abschluss — make ci + Plan-Buchhaltung (Pflicht-DoD)

**Files:**
- Modify: `docs/superpowers/plans/2026-06-11-flow-r1b-document-revisions.md` (Protokoll)

- [x] **Step 1: make ci in detached tmux (TTY-sicher), Exit aus Status-Datei**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-phase1-m1
tmux kill-session -t flow_ci 2>/dev/null || true
tmux new-session -d -s flow_ci "export DOCKER_HOST=\"unix://\$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')\" && export TESTCONTAINERS_RYUK_DISABLED=true && make ci > ci.log 2>&1; echo \$? > ci.status"
# warten bis fertig (Coverage-Lauf inkl. Container ~3–6 min):
while [ ! -f ci.status ]; do sleep 10; done; cat ci.status
```

Expected: `0`. Bei ≠0: `tail -40 ci.log` lesen, Ursache fixen wenn im R1b-Scope (pgstore/
documents), sonst Abweichung + Stopp. Danach `rm -f ci.log ci.status`.

- [x] **Step 2: Working-Tree-Check**

```bash
git status --short && git log --oneline -4
```

Expected: clean; oben die zwei R1b-Commits (`feat(pgstore): document_revisions-Tabelle …`,
`feat(pgstore): Revision bei jedem …`). NICHT pushen — Soenne pusht.

- [x] **Step 3: Selbst-Check gegen Spec §6/A1 (im Kopf, Ergebnis notieren)**

Checkliste — alle vier müssen „ja" sein, Antwort als Satz unter diesem Step eintragen:
1. Schreibt JEDER erfolgreiche PUT (create UND update) genau eine Revision? (Task 3 Step 1)
2. Schreibt DELETE einen Marker mit letztem Body und überlebt die Revision das Dokument? (kein FK — Task 1)
3. Schreiben Konflikt-Pfade (409/412-Äquivalente) NICHTS? (Rollback via BeginFunc — Task 2 Test 2)
4. Ist kein Port/keine API/kein Handler angefasst worden? (`git diff --stat HEAD~2` zeigt nur die 3 File-Map-Dateien)

> **Selbst-Check:** (1) Ja — `insertRevision` wird nach jedem INSERT (create) und UPDATE (update) am Ende des `pgx.BeginFunc`-Lambdas aufgerufen, genau einmal pro erfolgreicher Put-Operation. (2) Ja — `Delete` ruft `insertRevision(ctx, tx, doc, true)` nach `DELETE … RETURNING` auf; kein FK auf `documents(id)` in der Baseline-Migration. (3) Ja — `pgx.BeginFunc` rollt bei jedem Fehler (Constraint-Verletzung, ErrNoRows) automatisch zurück; `insertRevision` wird nie erreicht. (4) Ja — `git diff HEAD~2 --name-only` zeigt exakt die 3 File-Map-Dateien: `internal/adapter/pgstore/migrations/0001_baseline.sql`, `internal/adapter/pgstore/documents.go`, `internal/adapter/pgstore/documents_revisions_test.go`.

- [x] **Step 4: Buchhaltungs-Commit**

```bash
git add docs/superpowers/plans/2026-06-11-flow-r1b-document-revisions.md
git commit -m "$(cat <<'EOF'
docs(plan): R1b abgeschlossen — Boxen + Protokoll gepflegt

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review (gegen Spec §6/A1 + §13 R1b)

| Spec-Anforderung | Task |
|---|---|
| `document_revisions` append-only, kein FK, in Baseline (kein 0002) §6/A1 | 1 |
| Jeder PUT schreibt neuen Stand als Revision, in derselben Transaktion §6/A1 | 3 |
| DELETE schreibt Lösch-Marker mit letztem Body §6/A1 | 3 |
| Invariante: jeder je gespeicherte Stand steht in Revisionen (Konflikte schreiben nichts) | 2, 3 |
| Kein Read-API in Phase 1 (psql-Restore) — kein Port-/Handler-Change | File-Map („Sonst nichts") |
| Milestone-DoD: ci grün mit echtem Exit-Code, Boxen + Protokoll gepflegt (A1) | 4 |

**Explizit NICHT R1b:** `flow docs log/restore` (Phase 2), Pruning (§15.8), Revisions-Anzeige
in WebUI/TUI, der Akzeptanz-Checklisten-Punkt 9 (psql-Restore-Drill — der gehört zu R6 und
braucht reale Daten).

## Abweichungs-Protokoll

(Der Executor trägt hier ein, was vom Plan abweichen musste — Task-Nummer + 1–3 Sätze.)
