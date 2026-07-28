# `flow_patch_doc` Größendelta + Schrumpf-Guard — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Spec: `docs/superpowers/specs/2026-07-28-patch-doc-shrink-guard-design.md` · flow: [[specs/2026-07-28-patch-doc-shrink-guard-design]]
Auslöser: [[bugs/mcp-patch-doc-replace-section-truncation]]

**Goal:** `flow_patch_doc` und `flow_update_doc` melden künftig, wie viel sie am Body verändert haben, und verweigern ohne `allowShrink=true` jeden Schreibvorgang, der mehr als die Hälfte **und** mehr als 1 KB entfernt.

**Architecture:** Alles in `cmd/flow-mcp`. flow-mcp rechnet den neuen Body lokal aus und schickt einen vollen Body an den Server — der Server kann einen Patch nicht von einem WebUI-Save unterscheiden, ein serverseitiger Guard würde dort Fehlalarme auslösen. Zwei neue, reine Funktionen (`newBodyDelta`, `checkShrink`) in `write.go`, verdrahtet in den beiden Handlern in `tools_write.go`. `internal/` wird nicht angefasst.

**Tech Stack:** Go, `github.com/modelcontextprotocol/go-sdk/mcp`, Standardbibliothek (`strings`, `fmt`). Keine neuen Abhängigkeiten.

## Global Constraints

- Branch `patch-doc-shrink-guard`, Worktree `../flow-patch-doc-shrink-guard`, von `main` @ `6f7dfd8`.
- `make ci` (= `lint verify-generate verify-css verify-no-popups cover build`) muss am Ende grün sein.
- Coverage-Gate 75 % (`make cover`), `*_templ.go` ausgeschlossen. Echte Tests, kein Padding.
- Keine Änderung an `markdownSection` / `patchMarkdown` — die Subtree-Semantik bleibt exakt wie sie ist.
- Keine neuen Dateien. Alle Änderungen in `cmd/flow-mcp/{write,tools_write,server}.go` und den zugehörigen Tests.
- Schwellwerte exakt: `shrinkRatio = 0.5`, `shrinkMinBytes = 1024`. Beide Bedingungen müssen zutreffen.
- Feld heißt `allowShrink` und ist **getrennt** von `confirm`.

### Korrektur an der Spec (beim Planschreiben gefunden)

Spec §3.1 nannte ursprünglich `strings.Count(s, "\n") + 1`. Das zählt eine Zeile zu viel, sobald der Body auf `\n` endet — und `replaceMarkdownSection` (`write.go:205`) erzeugt **immer** einen abschließenden Newline, die Formel lag also bei praktisch jedem Body daneben. Korrekt ist `strings.Count(strings.TrimSuffix(s, "\n"), "\n") + 1`. **Die Spec wurde nachgezogen** (Commit im selben Branch); Plan und Spec stimmen überein.

---

## File Structure

| Datei | Verantwortung | Aktion |
|---|---|---|
| `cmd/flow-mcp/write.go` | reine Hilfsfunktionen der Schreibpfade: Ergebnisformung, Guards, Markdown-Mechanik | Modify — `bodyDelta`, `newBodyDelta`, `countLines`, `checkShrink`, Konstanten, Felder auf `documentWriteResult`, Signatur von `documentResult` |
| `cmd/flow-mcp/tools_write.go` | MCP-Handler und ihre Eingabetypen | Modify — `AllowShrink` auf `patchDocIn` + `updateDocIn`, Verdrahtung in `patchDoc` + `updateDoc`, vier `documentResult`-Call-Sites |
| `cmd/flow-mcp/server.go` | Tool-Registrierung und Descriptions | Modify — Descriptions von `flow_patch_doc` und `flow_update_doc` |
| `cmd/flow-mcp/write_test.go` | Unit-Tests der reinen Funktionen | Modify — Semantik-Charakterisierung, `newBodyDelta`, `checkShrink` |
| `cmd/flow-mcp/loopback_write_test.go` | End-to-End über eine echte MCP-Session gegen ein Fake-Backend | Modify — Delta in der Antwort, Guard, `allowShrink` |

---

## Task 1: Subtree-Semantik festnageln

Charakterisierungstests für Verhalten, das es heute schon gibt und das **nicht** verändert werden soll. Sie gehen sofort auf grün — das ist hier korrekt und beabsichtigt: sie sind das Sicherheitsnetz, das die folgenden Tasks davor bewahrt, die Semantik versehentlich zu verschieben. Der bestehende Test (`write_test.go:25-28`) ersetzt `## Notes`, die *letzte* H2 *ohne* Unterabschnitte — dort sind „Subtree" und „bis zur nächsten Überschrift" nicht unterscheidbar.

**Files:**
- Test: `cmd/flow-mcp/write_test.go`

**Interfaces:**
- Consumes: `patchMarkdown(body string, in patchDocIn) (string, error)` — bestehend, unverändert.
- Produces: nichts. Reine Absicherung.

- [ ] **Step 1: Die beiden Charakterisierungstests schreiben**

Ans Ende von `cmd/flow-mcp/write_test.go` anfügen:

```go
// TestPatchMarkdownReplaceSectionSpansSubsections nagelt die Baum-Semantik fest:
// ein Abschnitt schließt seine Unterabschnitte ein. Das ist gewolltes Verhalten,
// kein Bug — der Bug war, dass es weder dokumentiert noch sichtbar war.
func TestPatchMarkdownReplaceSectionSpansSubsections(t *testing.T) {
	base := "# Doc\n\n## One\n\nintro\n\n### One A\n\ndetail a\n\n### One B\n\ndetail b\n\n## Two\n\nkeep two\n"

	got, err := patchMarkdown(base, patchDocIn{Operation: "replace_section", Section: "One", Body: "replacement"})
	if err != nil {
		t.Fatalf("replace_section on H2 with subsections: %v", err)
	}
	want := "# Doc\n\n## One\n\nreplacement\n\n## Two\n\nkeep two\n"
	if got != want {
		t.Fatalf("replace_section on H2 with subsections =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(got, "detail a") || strings.Contains(got, "detail b") {
		t.Fatalf("subsections survived the replacement: %q", got)
	}
	if !strings.Contains(got, "keep two") {
		t.Fatalf("the following H2 section was swallowed: %q", got)
	}
}

// TestPatchMarkdownReplaceSectionOnH1SpansWholeDocument ist die Regression für
// den Verlustfall vom 2026-07-28: keine H2/H3 erfüllt gotLevel <= 1, also endet
// die H1-Sektion erst am Dateiende. Das Verhalten bleibt — der Schrumpf-Guard
// (Task 3/5) ist die Absicherung, nicht eine Änderung dieser Semantik.
func TestPatchMarkdownReplaceSectionOnH1SpansWholeDocument(t *testing.T) {
	base := "# Review\n\nintro\n\n## Findings\n\nS1 S2 S3\n\n## Method\n\nhow it was done\n"

	got, err := patchMarkdown(base, patchDocIn{Operation: "replace_section", Section: "Review", Body: "corrected intro"})
	if err != nil {
		t.Fatalf("replace_section on H1: %v", err)
	}
	want := "# Review\n\ncorrected intro\n"
	if got != want {
		t.Fatalf("replace_section on H1 =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(got, "Findings") || strings.Contains(got, "Method") {
		t.Fatalf("chapters survived — semantics changed unexpectedly: %q", got)
	}
}
```

- [ ] **Step 2: Tests laufen lassen**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard && go test ./cmd/flow-mcp/ -run 'TestPatchMarkdownReplaceSection' -v`
Expected: **PASS** für beide. Charakterisierungstests, kein Rot-Phase-TDD — sie beschreiben den Ist-Zustand. Gehen sie unerwartet auf Rot, stimmt die Annahme über `markdownSection` nicht und der Plan muss angehalten werden.

- [ ] **Step 3: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard
git add cmd/flow-mcp/write_test.go
git commit -m "test(flow-mcp): pin replace_section subtree semantics incl. the H1 case"
```

---

## Task 2: `bodyDelta` und `newBodyDelta`

**Files:**
- Modify: `cmd/flow-mcp/write.go` (neuer Block direkt nach `documentWriteResult`, ca. Zeile 42)
- Test: `cmd/flow-mcp/write_test.go`

**Interfaces:**
- Consumes: nichts.
- Produces:
  - `type bodyDelta struct { BytesBefore, BytesAfter, LinesBefore, LinesAfter int }`
  - `func newBodyDelta(before, after string) bodyDelta`
  - `func countLines(s string) int`

- [ ] **Step 1: Den failing test schreiben**

Ans Ende von `cmd/flow-mcp/write_test.go` anfügen:

```go
func TestNewBodyDelta(t *testing.T) {
	tests := []struct {
		name           string
		before, after  string
		wantBB, wantBA int
		wantLB, wantLA int
	}{
		{"empty after", "# A\n\nbody\n", "", 10, 0, 3, 0},
		{"empty before", "", "new\n", 0, 4, 0, 1},
		{"both empty", "", "", 0, 0, 0, 0},
		{"trailing newline counts once", "a\n", "a\nb\n", 2, 4, 1, 2},
		{"no trailing newline", "a\nb", "a", 3, 1, 2, 1},
		{"growth", "short", "much longer body", 5, 16, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newBodyDelta(tt.before, tt.after)
			want := bodyDelta{BytesBefore: tt.wantBB, BytesAfter: tt.wantBA, LinesBefore: tt.wantLB, LinesAfter: tt.wantLA}
			if got != want {
				t.Fatalf("newBodyDelta(%q, %q) = %+v, want %+v", tt.before, tt.after, got, want)
			}
		})
	}
}
```

- [ ] **Step 2: Test laufen lassen, Fehlschlag bestätigen**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard && go test ./cmd/flow-mcp/ -run TestNewBodyDelta`
Expected: FAIL — Compile-Fehler `undefined: newBodyDelta`, `undefined: bodyDelta`.

- [ ] **Step 3: Minimal implementieren**

In `cmd/flow-mcp/write.go`, direkt **nach** dem `documentWriteResult`-Struct (nach Zeile 42) einfügen:

```go
// bodyDelta misst, wie stark ein Schreibvorgang den Body verändert. Er speist
// sowohl das Größensignal in der Antwort als auch den Schrumpf-Guard, damit die
// Zahl in der Fehlermeldung und die Zahl in der Antwort nicht auseinanderlaufen
// können.
type bodyDelta struct {
	BytesBefore int
	BytesAfter  int
	LinesBefore int
	LinesAfter  int
}

func newBodyDelta(before, after string) bodyDelta {
	return bodyDelta{
		BytesBefore: len(before),
		BytesAfter:  len(after),
		LinesBefore: countLines(before),
		LinesAfter:  countLines(after),
	}
}

// countLines zählt Textzeilen: ein leerer Body hat 0, ein abschließender
// Newline erzeugt keine Extrazeile. Letzteres ist wichtig, weil
// replaceMarkdownSection immer mit "\n" abschließt.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(strings.TrimSuffix(s, "\n"), "\n") + 1
}
```

`strings` ist in `write.go:9` bereits importiert.

- [ ] **Step 4: Test laufen lassen, Erfolg bestätigen**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard && go test ./cmd/flow-mcp/ -run TestNewBodyDelta -v`
Expected: PASS, alle sechs Unterfälle.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard
git add cmd/flow-mcp/write.go cmd/flow-mcp/write_test.go
git commit -m "feat(flow-mcp): bodyDelta measures byte and line change of a write"
```

---

## Task 3: `checkShrink`

**Files:**
- Modify: `cmd/flow-mcp/write.go` (nach dem `bodyDelta`-Block aus Task 2)
- Test: `cmd/flow-mcp/write_test.go`

**Interfaces:**
- Consumes: `bodyDelta` aus Task 2.
- Produces:
  - `const shrinkRatio = 0.5`, `const shrinkMinBytes = 1024`
  - `func checkShrink(action string, d bodyDelta, allow bool) error` — `action` ist `"patch"` oder `"update"` und bestimmt das erste Wort der Meldung sowie ob `flow_update_doc` als Ausweg genannt wird.

- [ ] **Step 1: Den failing test schreiben**

Ans Ende von `cmd/flow-mcp/write_test.go` anfügen:

```go
func TestCheckShrink(t *testing.T) {
	// Der reale Verlustfall vom 2026-07-28: 14204 → 1021 Bytes, 312 → 25 Zeilen.
	// Nicht `real` nennen — das ist ein Go-Builtin.
	lossCase := bodyDelta{BytesBefore: 14204, BytesAfter: 1021, LinesBefore: 312, LinesAfter: 25}

	err := checkShrink("patch", lossCase, false)
	if err == nil {
		t.Fatal("the real loss case should be refused")
	}
	for _, want := range []string{"13183", "14204", "93%", "312", "25", "allowShrink", "flow_update_doc"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal %q does not mention %q", err.Error(), want)
		}
	}

	if err := checkShrink("patch", lossCase, true); err != nil {
		t.Fatalf("allowShrink=true should pass, got %v", err)
	}

	// update nennt flow_update_doc NICHT als Ausweg — das wäre zirkulär.
	err = checkShrink("update", lossCase, false)
	if err == nil || strings.Contains(err.Error(), "flow_update_doc") {
		t.Fatalf("update refusal = %v, want a refusal without the flow_update_doc hint", err)
	}
	if !strings.HasPrefix(err.Error(), "update would remove") {
		t.Fatalf("update refusal = %q, want it to start with 'update would remove'", err.Error())
	}

	passes := []struct {
		name string
		d    bodyDelta
	}{
		// Über 50 %, aber unter der absoluten Untergrenze: kleine Dokumente
		// dürfen nicht ständig anschlagen.
		{"above ratio, below floor", bodyDelta{BytesBefore: 612, BytesAfter: 280, LinesBefore: 20, LinesAfter: 9}},
		// Genau an der Untergrenze — > ist echt größer, 1024 selbst reicht nicht.
		{"exactly at the byte floor", bodyDelta{BytesBefore: 4096, BytesAfter: 3072, LinesBefore: 100, LinesAfter: 75}},
		// Viele Bytes entfernt, aber genau die Hälfte — > ist echt größer.
		{"exactly half removed", bodyDelta{BytesBefore: 8000, BytesAfter: 4000, LinesBefore: 200, LinesAfter: 100}},
		{"growth", bodyDelta{BytesBefore: 100, BytesAfter: 9000, LinesBefore: 3, LinesAfter: 300}},
		{"unchanged", bodyDelta{BytesBefore: 5000, BytesAfter: 5000, LinesBefore: 120, LinesAfter: 120}},
		{"empty before", bodyDelta{BytesBefore: 0, BytesAfter: 0, LinesBefore: 0, LinesAfter: 0}},
	}
	for _, tt := range passes {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkShrink("patch", tt.d, false); err != nil {
				t.Fatalf("checkShrink(%+v) = %v, want nil", tt.d, err)
			}
		})
	}
}
```

- [ ] **Step 2: Test laufen lassen, Fehlschlag bestätigen**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard && go test ./cmd/flow-mcp/ -run TestCheckShrink`
Expected: FAIL — `undefined: checkShrink`.

- [ ] **Step 3: Minimal implementieren**

In `cmd/flow-mcp/write.go` direkt nach `countLines` einfügen:

```go
const (
	// shrinkRatio ist der Anteil des Bodys, ab dem ein Schreibvorgang als
	// zerstörend gilt. shrinkMinBytes ist die absolute Untergrenze: ohne sie
	// schlägt der Guard bei kleinen Dokumenten ständig an, wo ein legitimer
	// Abschnittstausch schnell die halbe Datei ist — und Agenten gewöhnen sich
	// an allowShrink=true.
	shrinkRatio    = 0.5
	shrinkMinBytes = 1024
)

// checkShrink verweigert einen Schreibvorgang, der mehr als shrinkRatio des
// Bodys UND mehr als shrinkMinBytes entfernt, solange allow nicht gesetzt ist.
// action ist "patch" oder "update" und prägt die Meldung.
func checkShrink(action string, d bodyDelta, allow bool) error {
	if allow || d.BytesBefore == 0 {
		return nil
	}
	removed := d.BytesBefore - d.BytesAfter
	if removed <= shrinkMinBytes {
		return nil
	}
	if float64(removed)/float64(d.BytesBefore) <= shrinkRatio {
		return nil
	}
	pct := (removed*100 + d.BytesBefore/2) / d.BytesBefore
	msg := fmt.Sprintf("%s would remove %d of %d bytes (%d%%), %d lines to %d. Pass allowShrink=true if intended",
		action, removed, d.BytesBefore, pct, d.LinesBefore, d.LinesAfter)
	if action == "patch" {
		msg += ", or use flow_update_doc with the full body"
	}
	return errors.New(msg)
}
```

`fmt` ist in `write.go:8` bereits importiert. **`errors` nicht** — der Import muss ergänzt werden (alphabetisch vor `encoding/hex`… nein: nach `encoding/json`, vor `fmt`):

```go
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/domain"
)
```

**Kein abschließender Punkt am Fehlertext.** `golangci-lint` v2 führt die ST-Checks unter `staticcheck`, und ST1005 verbietet Fehlertexte mit abschließendem Satzzeichen — empirisch geprüft, `fmt.Errorf("%s.", msg)` bricht `make lint`. Der Punkt **innerhalb** der Meldung (nach „lines to 25") ist unbedenklich, nur das Ende zählt. `errors.New` mit einer Variablen wird von ST1005 nicht geprüft.

- [ ] **Step 4: Test laufen lassen, Erfolg bestätigen**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard && go test ./cmd/flow-mcp/ -run TestCheckShrink -v`
Expected: PASS.

- [ ] **Step 4b: Lint für das Paket laufen lassen**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard && golangci-lint run ./cmd/flow-mcp/`
Expected: keine Findings. Erscheint hier ST1005, endet der Fehlertext doch auf einem Satzzeichen — dann das Ende korrigieren, nicht den Linter abschalten.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard
git add cmd/flow-mcp/write.go cmd/flow-mcp/write_test.go
git commit -m "feat(flow-mcp): checkShrink refuses writes that remove >50% and >1KB"
```

---

## Task 4: Größendelta in der Antwort

Nur das Signal, noch kein Guard. Danach ist der Schaden sichtbar, aber noch nicht verhindert.

**Files:**
- Modify: `cmd/flow-mcp/write.go:32-42` (Struct), `:44-61` (`documentResult`)
- Modify: `cmd/flow-mcp/tools_write.go:66,112,160,225` (vier Call-Sites), `:84-119` (`updateDoc`), `:133-167` (`patchDoc`)
- Test: `cmd/flow-mcp/loopback_write_test.go`

**Interfaces:**
- Consumes: `newBodyDelta` aus Task 2.
- Produces: `func (h *handlers) documentResult(ctx context.Context, action string, d domain.Document, delta *bodyDelta) *mcp.CallToolResult` — **Signaturänderung**, alle vier Call-Sites müssen mitgezogen werden. `nil` bedeutet „kein Vorher".

- [ ] **Step 1: Den failing test schreiben**

Ans Ende von `cmd/flow-mcp/loopback_write_test.go` anfügen:

```go
func TestLoopback_PatchAndUpdateReportBodyDelta(t *testing.T) {
	sess := authedWriteServer(t)

	// Ein kleiner Patch: die Delta-Felder sind da, der Guard (Task 5) greift nicht.
	res, out := callText(t, sess, "flow_patch_doc", map[string]any{
		"id": "d-human", "operation": "set_checkbox", "checkbox": "F40 context", "checked": true,
		"confirm": true,
	})
	if res.IsError {
		t.Fatalf("checkbox patch errored: %s", out)
	}
	for _, want := range []string{`"bytesBefore":`, `"bytesAfter":`, `"linesBefore":`, `"linesAfter":`} {
		if !strings.Contains(out, want) {
			t.Fatalf("patch response %q is missing %s", out, want)
		}
	}

	// flow_update_doc mit leerem Body: bytesAfter/linesAfter MÜSSEN als 0
	// erscheinen. Mit `int` + omitempty würde json genau diese Null
	// verschlucken — also ausgerechnet den Totalverlust.
	res, out = callText(t, sess, "flow_update_doc", map[string]any{"id": "d-memory", "body": ""})
	if res.IsError {
		t.Fatalf("emptying update errored: %s", out)
	}
	if !strings.Contains(out, `"bytesAfter":0`) || !strings.Contains(out, `"linesAfter":0`) {
		t.Fatalf("emptying update response = %q, want bytesAfter:0 and linesAfter:0", out)
	}

	// Ein Update ohne body lässt den Text unangetastet — kein Delta.
	res, out = callText(t, sess, "flow_update_doc", map[string]any{"id": "d-memory-2", "title": "Renamed"})
	if res.IsError {
		t.Fatalf("title-only update errored: %s", out)
	}
	if strings.Contains(out, `"bytesBefore":`) || strings.Contains(out, `"linesAfter":`) {
		t.Fatalf("title-only update response = %q, want no delta fields", out)
	}

	// create hat kein Vorher.
	res, out = callText(t, sess, "flow_create_doc", map[string]any{
		"type": "memory", "path": "notes/delta", "title": "D", "body": "fresh",
	})
	if res.IsError {
		t.Fatalf("create errored: %s", out)
	}
	if strings.Contains(out, `"bytesBefore":`) {
		t.Fatalf("create response = %q, want no delta fields", out)
	}
}
```

- [ ] **Step 2: Test laufen lassen, Fehlschlag bestätigen**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard && go test ./cmd/flow-mcp/ -run TestLoopback_PatchAndUpdateReportBodyDelta`
Expected: FAIL — die Antwort enthält keine `bytesBefore`-Felder.

- [ ] **Step 3: Felder ans Ergebnis-Struct**

In `cmd/flow-mcp/write.go`, `documentWriteResult` (Zeilen 32-42) um vier Felder erweitern — **`*int`**, damit `omitempty` die aussagekräftige `0` nicht verschluckt:

```go
type documentWriteResult struct {
	Action    string `json:"action"`
	ID        string `json:"id"`
	Project   string `json:"project"`
	Type      string `json:"type,omitempty"`
	Path      string `json:"path,omitempty"`
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updatedAt"`
	Version   string `json:"version"`
	Hash      string `json:"hash"`
	// Pointer, nicht int: omitempty würde bei einem int die 0 unterschlagen —
	// also ausgerechnet "bytesAfter": 0, den Totalverlust, den diese Felder
	// sichtbar machen sollen. nil heißt "kein Vorher" (create, move).
	BytesBefore *int `json:"bytesBefore,omitempty"`
	BytesAfter  *int `json:"bytesAfter,omitempty"`
	LinesBefore *int `json:"linesBefore,omitempty"`
	LinesAfter  *int `json:"linesAfter,omitempty"`
}
```

- [ ] **Step 4: `documentResult` nimmt das Delta entgegen**

In `cmd/flow-mcp/write.go` die Funktion `documentResult` (Zeilen 44-61) ersetzen:

```go
func (h *handlers) documentResult(ctx context.Context, action string, d domain.Document, delta *bodyDelta) *mcp.CallToolResult {
	project := "none"
	if d.NodeID != nil {
		project = *d.NodeID
		if nodes, err := h.nodeList(ctx, false); err == nil {
			for _, node := range nodes {
				if node.ID == *d.NodeID {
					project = node.Slug
					break
				}
			}
		}
	}
	version := d.UpdatedAt.UTC().Format(time.RFC3339Nano)
	out := makeWriteResult(action, d.ID, project, version, d.Body)
	out.Type, out.Path, out.Title = string(d.Type), d.Path, d.Title
	if delta != nil {
		out.BytesBefore, out.BytesAfter = &delta.BytesBefore, &delta.BytesAfter
		out.LinesBefore, out.LinesAfter = &delta.LinesBefore, &delta.LinesAfter
	}
	return writeResult(out)
}
```

- [ ] **Step 5: Die vier Call-Sites nachziehen**

In `cmd/flow-mcp/tools_write.go`:

- Zeile 66: `out = h.documentResult(ctx, "created", d)` → `out = h.documentResult(ctx, "created", d, nil)`
- Zeile 225: `out = h.documentResult(ctx, "moved", d)` → `out = h.documentResult(ctx, "moved", d, nil)`

In `updateDoc`: den Block zwischen der „nothing to update"-Prüfung (Zeile 97-99) und `expectedUpdatedAt` (Zeile 100) um die Delta-Berechnung ergänzen, und die Ergebniszeile anpassen. Der lokale Name ist `bd`, weil `d` weiter unten schon das zurückgegebene Dokument ist:

```go
		if in.Title == nil && in.Body == nil && in.Tags == nil {
			return errGuard{fmt.Errorf("nothing to update: pass title, body, and/or tags")}
		}
		var delta *bodyDelta
		if in.Body != nil {
			bd := newBodyDelta(cur.Body, *in.Body)
			delta = &bd
		}
		expected, err := expectedUpdatedAt(in.ExpectedUpdatedAt, cur.UpdatedAt)
```

und Zeile 112: `out = h.documentResult(ctx, "updated", d)` → `out = h.documentResult(ctx, "updated", d, delta)`

In `patchDoc`: nach `patchMarkdown` (Zeile 146-149) das Delta berechnen und in der Ergebniszeile übergeben:

```go
		body, err := patchMarkdown(cur.Body, in)
		if err != nil {
			return errGuard{err}
		}
		delta := newBodyDelta(cur.Body, body)
		expected, err := expectedUpdatedAt(in.ExpectedUpdatedAt, cur.UpdatedAt)
```

und Zeile 160: `out = h.documentResult(ctx, "patched", d)` → `out = h.documentResult(ctx, "patched", d, &delta)`

- [ ] **Step 6: Test laufen lassen, Erfolg bestätigen**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard && go test ./cmd/flow-mcp/ -run TestLoopback_PatchAndUpdateReportBodyDelta -v`
Expected: PASS.

- [ ] **Step 7: Das ganze Paket laufen lassen**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard && go test ./cmd/flow-mcp/`
Expected: PASS — insbesondere kein Compile-Fehler an einer übersehenen `documentResult`-Call-Site.

- [ ] **Step 8: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard
git add cmd/flow-mcp/write.go cmd/flow-mcp/tools_write.go cmd/flow-mcp/loopback_write_test.go
git commit -m "feat(flow-mcp): report byte and line delta on patch and update"
```

---

## Task 5: Guard verdrahten + `allowShrink`

**Files:**
- Modify: `cmd/flow-mcp/tools_write.go:75-82` (`updateDocIn`), `:121-131` (`patchDocIn`), `:84-119` (`updateDoc`), `:133-167` (`patchDoc`)
- Test: `cmd/flow-mcp/loopback_write_test.go`

**Interfaces:**
- Consumes: `checkShrink` aus Task 3, `newBodyDelta`/`delta` aus Task 4.
- Produces: Feld `AllowShrink bool` auf `patchDocIn` und `updateDocIn`, JSON-Name `allowShrink`.

- [ ] **Step 1: Den failing test schreiben**

Ans Ende von `cmd/flow-mcp/loopback_write_test.go` anfügen:

```go
func TestLoopback_ShrinkGuardRefusesH1WipeUnlessAllowed(t *testing.T) {
	sess := authedWriteServer(t)

	// Ein Dokument, das groß genug ist, um die 1-KB-Untergrenze zu reißen:
	// eine H1 plus 60 Kapitel, ~2,9 KB.
	big := "# Review\n\n" + strings.Repeat("## Chapter\n\nfindings findings findings findings\n\n", 60)
	res, out := callText(t, sess, "flow_create_doc", map[string]any{
		"type": "memory", "path": "notes/big", "title": "Big", "body": big,
	})
	if res.IsError {
		t.Fatalf("seeding the big doc failed: %s", out)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("cannot read the created id from %q: %v", out, err)
	}

	// replace_section auf die H1 — genau der Verlustfall. Muss abgelehnt werden.
	res, out = callText(t, sess, "flow_patch_doc", map[string]any{
		"id": created.ID, "operation": "replace_section", "section": "Review", "body": "corrected intro",
	})
	if !res.IsError {
		t.Fatalf("H1 wipe was accepted; response = %q", out)
	}
	for _, want := range []string{"would remove", "allowShrink", "bytes"} {
		if !strings.Contains(out, want) {
			t.Fatalf("refusal %q does not mention %q", out, want)
		}
	}

	// Der Body ist unangetastet — der Guard sitzt vor dem Netzwerkaufruf.
	_, got := callText(t, sess, "flow_get_doc", map[string]any{"id": created.ID})
	if !strings.Contains(got, "findings") {
		t.Fatalf("document was modified despite the refusal: %q", got)
	}

	// Mit allowShrink=true geht derselbe Aufruf durch.
	res, out = callText(t, sess, "flow_patch_doc", map[string]any{
		"id": created.ID, "operation": "replace_section", "section": "Review", "body": "corrected intro",
		"allowShrink": true,
	})
	if res.IsError {
		t.Fatalf("allowShrink=true was still refused: %s", out)
	}
	if !strings.Contains(out, `"action":"patched"`) {
		t.Fatalf("allowShrink patch = %q, want action patched", out)
	}
	_, got = callText(t, sess, "flow_get_doc", map[string]any{"id": created.ID})
	if strings.Contains(got, "findings") {
		t.Fatalf("allowShrink patch did not apply: %q", got)
	}
}

func TestLoopback_ShrinkGuardAppliesToUpdateAndIsSeparateFromConfirm(t *testing.T) {
	sess := authedWriteServer(t)

	big := "# Notes\n\n" + strings.Repeat("a line of human prose that is worth keeping\n", 60)
	res, out := callText(t, sess, "flow_create_doc", map[string]any{
		"type": "memory", "path": "notes/big-update", "title": "BigU", "body": big,
	})
	if res.IsError {
		t.Fatalf("seeding failed: %s", out)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("cannot read the created id from %q: %v", out, err)
	}

	res, out = callText(t, sess, "flow_update_doc", map[string]any{"id": created.ID, "body": "oops"})
	if !res.IsError || !strings.Contains(out, "allowShrink") {
		t.Fatalf("clobbering update = (IsError=%v, %q), want a shrink refusal", res.IsError, out)
	}
	if strings.Contains(out, "flow_update_doc") {
		t.Fatalf("the update refusal should not point at flow_update_doc: %q", out)
	}

	res, out = callText(t, sess, "flow_update_doc", map[string]any{
		"id": created.ID, "body": "oops", "allowShrink": true,
	})
	if res.IsError {
		t.Fatalf("allowShrink=true update was refused: %s", out)
	}

	// confirm allein darf den Schrumpf-Guard NICHT entschärfen: sonst wäre er
	// ausgerechnet auf human-owned Notes wirkungslos, die confirm ohnehin
	// verlangen.
	humanBig := "# Keep\n\n" + strings.Repeat("something the human wrote and wants to keep\n", 60)
	res, out = callText(t, sess, "flow_update_doc", map[string]any{
		"id": "d-human", "body": humanBig, "confirm": true,
	})
	if res.IsError {
		t.Fatalf("growing the human note failed: %s", out)
	}
	res, out = callText(t, sess, "flow_update_doc", map[string]any{
		"id": "d-human", "body": "wiped", "confirm": true,
	})
	if !res.IsError || !strings.Contains(out, "allowShrink") {
		t.Fatalf("confirm=true alone bypassed the shrink guard: (IsError=%v, %q)", res.IsError, out)
	}
}
```

- [ ] **Step 2: Test laufen lassen, Fehlschlag bestätigen**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard && go test ./cmd/flow-mcp/ -run TestLoopback_ShrinkGuard`
Expected: FAIL — beide Aufrufe gehen durch, weil es weder Feld noch Guard gibt.

- [ ] **Step 3: `AllowShrink` an beide Eingabetypen**

In `cmd/flow-mcp/tools_write.go`, `updateDocIn` (Zeilen 75-82) um eine Zeile nach `Confirm` ergänzen:

```go
	AllowShrink       bool      `json:"allowShrink,omitempty" jsonschema:"required (true) to apply a write that removes more than half the document body"`
```

Und in `patchDocIn` (Zeilen 121-131) genauso nach `Confirm`:

```go
	AllowShrink       bool    `json:"allowShrink,omitempty" jsonschema:"required (true) to apply a write that removes more than half the document body"`
```

- [ ] **Step 4: Guard in `patchDoc` einsetzen**

In `cmd/flow-mcp/tools_write.go`, `patchDoc`: die in Task 4 eingefügte `delta`-Zeile um den Guard ergänzen:

```go
		delta := newBodyDelta(cur.Body, body)
		if err := checkShrink("patch", delta, in.AllowShrink); err != nil {
			return errGuard{err}
		}
```

- [ ] **Step 5: Guard in `updateDoc` einsetzen**

In `cmd/flow-mcp/tools_write.go`, `updateDoc`: den in Task 4 eingefügten Delta-Block um den Guard ergänzen:

```go
		var delta *bodyDelta
		if in.Body != nil {
			bd := newBodyDelta(cur.Body, *in.Body)
			if err := checkShrink("update", bd, in.AllowShrink); err != nil {
				return errGuard{err}
			}
			delta = &bd
		}
```

- [ ] **Step 6: Test laufen lassen, Erfolg bestätigen**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard && go test ./cmd/flow-mcp/ -run TestLoopback_ShrinkGuard -v`
Expected: PASS für beide Tests.

- [ ] **Step 7: Das ganze Paket laufen lassen**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard && go test ./cmd/flow-mcp/`
Expected: PASS. Schlägt hier ein *bestehender* Test fehl, weil er einen großen Abschnitt ersetzt, ist das ein echter Fund — dann den Test um `"allowShrink": true` ergänzen und im Commit begründen, nicht den Schwellwert aufweichen.

- [ ] **Step 8: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard
git add cmd/flow-mcp/tools_write.go cmd/flow-mcp/loopback_write_test.go
git commit -m "feat(flow-mcp): refuse destructive patches without allowShrink"
```

---

## Task 6: Tool-Descriptions + `make ci`

Die dritte Lücke aus dem Bug-Doc: die Semantik stand nirgends. Ohne diesen Task bleibt der Fußangel für jeden neuen Agenten unsichtbar.

**Files:**
- Modify: `cmd/flow-mcp/server.go:92-94` (`flow_patch_doc`), Description von `flow_update_doc`

**Interfaces:**
- Consumes: nichts.
- Produces: nichts. Reine Textänderung.

- [ ] **Step 1: Description von `flow_patch_doc` ersetzen**

In `cmd/flow-mcp/server.go`, Zeile 93:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_patch_doc",
		Description: "CAS-safe Markdown mutation without loading a large document into model context: replace_section, append_section, or set_checkbox (optionally changing checkbox label/status atomically). A section spans its subsections, so replacing the topmost heading replaces the whole document — to edit the intro, target the first subheading instead, or use flow_update_doc with the full body. A write that removes more than half the body requires allowShrink=true. Returns id, canonical project, version, updatedAt, hash, and the body delta (bytesBefore/bytesAfter, linesBefore/linesAfter).",
	}, h.patchDoc)
```

- [ ] **Step 2: Description von `flow_update_doc` ersetzen**

In `cmd/flow-mcp/server.go`, Zeile 89:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_update_doc",
		Description: "CAS-safe partial update of a document's title, body, and/or tags. Returns id, canonical project, version, updatedAt, hash, and — when body is supplied — the body delta (bytesBefore/bytesAfter, linesBefore/linesAfter). Modifying a human-owned note requires confirm=true. A write that removes more than half the body requires allowShrink=true.",
	}, h.updateDoc)
```

- [ ] **Step 3: Schema-Smoke — die Felder sind wirklich advertised**

Run:
```bash
cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard
go build ./cmd/flow-mcp && go test ./cmd/flow-mcp/ -run TestLoopback -v 2>&1 | tail -20
```
Expected: PASS. Der Build stellt sicher, dass die `jsonschema`-Tags übersetzt werden.

- [ ] **Step 4: Volles CI**

Run: `cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard && make ci`
Expected: alle Ziele grün — `lint`, `verify-generate`, `verify-css`, `verify-no-popups`, `cover` (≥ 75 %), `build`.

Bricht `cover` ab, fehlen Tests — **nicht** die Schwelle senken.

- [ ] **Step 5: Commit**

```bash
cd /Users/msoent/SourceCode/serverkraken/flow-patch-doc-shrink-guard
git add cmd/flow-mcp/server.go
git commit -m "docs(flow-mcp): document subtree semantics and the shrink guard"
```

---

## Done-Gate

- [ ] `make ci` grün.
- [ ] `go test ./cmd/flow-mcp/ -v` — die zwei Charakterisierungstests aus Task 1 sind weiterhin grün, die Semantik wurde also nicht verschoben.
- [ ] Live-Smoke gegen die echte PROD-Instanz: ein `flow_patch_doc` auf ein großes Dokument mit `replace_section` auf die H1 wird abgelehnt und nennt Bytes plus `allowShrink`; derselbe Aufruf mit `allowShrink=true` geht durch. Danach das Dokument zurücksetzen.
- [ ] Bug-Doc [[bugs/mcp-patch-doc-replace-section-truncation]] um den erledigten Stand ergänzen (Akzeptanzkriterien abhaken, Commit-Range nennen).
- [ ] `flow_set_active_context` mit Stand und offenem Rest (Revisionen-FR bleibt offen).
