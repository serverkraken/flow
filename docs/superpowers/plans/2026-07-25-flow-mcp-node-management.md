# flow-mcp Node-Management — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Die Domäne Nodes+Bindings über MCP vollständig bedienbar machen — sechs neue Tools, zwei geänderte, 25 → 31 — allein als Oberfläche in `cmd/flow-mcp`, ohne eine Zeile Backend.

**Architecture:** Jedes neue Tool ist ein `*handlers`-Handler in einer eigenen Datei, der seine Argumente clientseitig vorprüft (gute Fehlermeldung), Node-Referenzen über `h.lookupNode`/`h.nodeTarget` auflöst und seine Server-Arbeit in `h.do(ctx, req, func(c *apiclient.Client) error {...})` gegen bereits existierende `apiclient`-Methoden erledigt. Das Herzstück der Wiederverwendung ist `bindtarget.go`: `flow_bind_project`, alle vier Aktionen von `flow_node_binding` und das optionale `bind_path` von `flow_create_node` gehen durch dieselbe Ziel-Auflösung (`path | remote | cwd → Origin-Slug, Kind, Maschine, Pfad`). Alle Ausgabe-Formatierung liegt in `format_nodes.go`, damit die Handler frei von Rendering bleiben.

**Tech Stack:** Go (hexagonal), `modelcontextprotocol/go-sdk` MCP über StdioTransport, `internal/adapter/apiclient` als einziger Server-Zugang (Bearer-Token, owner-scoped), `internal/gitremote` + `internal/clientmachine` für die Umgebung, Loopback-Tests über `mcp.NewInMemoryTransports()` gegen ein `httptest`-Backend.

## Global Constraints

- **Keine Backend-Änderung, keine Migration, keine neue `apiclient`-Methode.** Alle acht Server-Endpunkte und alle benötigten `apiclient`-Methoden existieren (Spec §2). Wer in den Tasks 1 bis 11 `internal/` anfasst, ist vom Plan abgewichen.
- **Genau zwei bewusste Ausnahmen von der Regel darüber**, beide von Soenne am 2026-07-25 entschieden und jeweils in einem eigenen Task mit eigenem Commit isoliert, damit sie in Review und Historie sichtbar getrennt bleiben: **Task 0** zieht `internal/tui/screen/worktime/wtfmt` nach `internal/timefmt` hoch (reiner Refactor, damit `cmd/flow-mcp` die Minuten-Formatierung importieren darf, statt sie zu duplizieren), und **Task 12** ergänzt zwei `Emit`-Aufrufe in `internal/adapter/httpserver/projectbindings.go` (die SSE-Bestandslücke bei bind/unbind). Ein Task-Reviewer der Tasks 0 und 12 prüft gegen diese Ausnahme, nicht gegen den Satz darüber; ein Task-Reviewer der Tasks 1 bis 11 prüft gegen den Satz darüber.
- **Multi-Tenant ist bindend.** flow ist eine Multi-Tenant-App für Menschen UND AI-Agents (AGENTS.md Grundsätze). Jeder Datenzugriff läuft über `h.do` an den authentifizierten, owner-scoped `apiclient`; es entstehen keine neuen Store-Queries und keine neue Tenant-Fläche. „Ist nur ein User" ist als Begründung in Code, Kommentar, Commit und Trade-off UNZULÄSSIG. Kein neuer globaler Cache ohne Tenant-Schlüssel — der einzige Cache ist der bestehende, prozess-lokale `h.projects` hinter `h.projMu`, der zum authentifizierten Client eines einzigen Owners gehört.
- **Keine Monolithen:** eine Verantwortung pro Datei, Tests in einer eigenen `_test.go`-Datei (AGENTS.md). Der Dateiplan unten ist verbindlich.
- **TDD, wörtlich:** fehlschlagender Test → Fehlschlag mit exaktem Befehl bestätigen → minimale Implementierung → grün bestätigen → committen. Ein Commit pro Task, Conventional-Commit-Subject.
- **`make ci` muss GRÜN sein**, bevor irgendetwas „fertig" ist: `lint verify-generate verify-css verify-no-popups cover build` (Makefile:76).
- **NIEMALS `make fmt`** (Toolchain-Skew reformatiert das ganze Repo — AGENTS.md).
- **Coverage-Gate:** `COVER_THRESHOLD := 75`, aber `COVER_PKG := ./internal/...` (Makefile:4-5) — `cmd/flow-mcp` fließt NICHT ins Gate ein. Die Tests dieses Slices sind trotzdem Pflicht (TDD-Disziplin, und `make cover` führt `go test ./...` aus, bricht also bei einem roten Test in `cmd/flow-mcp` ab). Niemand schreibt hier Tests, um eine Zahl zu bewegen.
- **Kein templ, kein Tailwind, kein i18n in diesem Slice.** Es entsteht keine `.templ`-Änderung (also kein `make generate`), keine `web/tailwind.css`-Änderung (also kein `make web`) und keine neue Nutzertext-Zeile in der WebUI (also kein Eintrag in `catalog_de.go`/`catalog_en.go`). MCP-Tool-Beschreibungen und Tool-Ausgaben sind Modell-gerichtete API-Strings, keine lokalisierten UI-Texte — genauso wie die bestehenden 25 Descriptions in `server.go`. Wer diesen Absatz überspringt und i18n-Keys anlegt, ist abgewichen.
- **Keine Browser-Popups, keine Emoji-Piktogramme.** Für Kind-Marker in Textausgaben gilt der monospace-Glyph-Satz aus AGENTS.md (`● ◆ ⬡ ▶ ■`), nie ein Emoji.
- **Zustände einer Textoberfläche.** Diese Oberfläche hat kein Viewport und keinen laufenden Timer, aber sie hat Zustände, und jeder Renderer-Task benennt sie: **leer** (jede Liste hat eine eigene „No …"-Meldung, die sagt, was als Nächstes zu tun ist — nie eine leere Antwort), **lang** und **Fehlerpfad**. Für „lang" gilt eine feste Regel: **Einzelwerte werden NIE gekürzt** — Name, Slug, ID und Binding-Pfad sind Adressen, die ein Modell direkt zurück in den nächsten Tool-Call schreibt, und ein abgeschnittener Slug ist eine kaputte Adresse. Gekürzt werden ausschließlich **Wiederholungen** (Aufzählungen im Löschbericht via `joinCapped`), wobei die exakte Anzahl im umgebenden Satz erhalten bleibt. Fehlerpfad heißt: Validierungsfehler nennen die gültigen Werte, Serverfehler kommen wörtlich durch `h.resultErr` beim Modell an.
- **SSE:** Alle Mutationen dieses Slices laufen über REST-Endpunkte, die serverseitig bereits emittieren — `POST /api/v1/nodes` → `node.created` (`internal/adapter/httpserver/worktime.go:237`), `POST /api/v1/nodes/create-bound` → `node.created` (`projectbindings.go:54`), `POST /api/v1/nodes/{id}/move` → `node.moved` (`nodemove.go:37`), `DELETE /api/v1/nodes/{id}` → `node.deleted` (`worktime.go:294`), `PUT /api/v1/nodes/{id}/tags` → `node.updated` (`nodetags.go:51`). Konsument ist die WebUI (`hx-ext="sse"` + `hx-trigger="sse:node.created"` etc.) und der TUI-Nodetree. Der MCP-Prozess emittiert nichts selbst und darf das auch nicht. **Bekannte Bestandslücke, in diesem Plan geschlossen:** `PUT /api/v1/nodes/{id}/bindings` und `DELETE /api/v1/nodes/bindings` emittieren heute KEIN Event (`projectbindings.go:83,113`). Weil dieser Slice Bindings erstmals bequem änderbar macht, würde die WebUI nach einem MCP-`bind` ohne Reload veralten. Soennes Entscheidung vom 2026-07-25: schließen, aber **nach** dem Done-Gate und in einem eigenen Commit — das ist **Task 12**. Für die Tasks 1 bis 11 gilt weiterhin: der MCP-Prozess emittiert nichts selbst und darf das auch nicht.
- **Bestandsnamen nur aus dem Dossier.** Jeder Task, der einen Bestandsnamen tippt, hat vorher einen `rg`-Verifikationsstep. Bei Abweichung gewinnt der Bestand, nicht der Plan. Das gilt ausdrücklich auch für Test-Fixtures: die Felder von `domain.User`, `domain.Artifact`, `domain.Tag`, `domain.ProjectBinding` und `apiclient.CreateBoundNodeInput/Result`, die die Fake-Backends encodieren, sind Bestand und werden mitverifiziert.
- **Binden ist ein Upsert, kein Konflikt.** Ein Ziel (Remote-Slug bzw. Maschine+Pfad) kann nur an EINEN Node gebunden sein: `BindNode.Execute` endet in `Bindings.Upsert` (`internal/usecase/bind_node.go:54`), und der Store konfliktet auf `(owner_id, remote_slug)` bzw. `(owner_id, machine_id, path)` und ersetzt **nur `node_id`** (`internal/adapter/pgstore/projectbindings.go:49`). Ein erneutes Binden verschiebt das Ziel also **still** auf den neuen Node — kein 409, keine Warnung. Weil dieser Slice Bindings erstmals bequem änderbar macht, sagen die Descriptions von `flow_bind_project` und `flow_node_binding` das ausdrücklich und empfehlen `action="resolve"` als Vorabprüfung; Task 10 beweist das Verhalten in der Kette. Serverseitig wird nichts geändert.
- **Owner-Scoping der Löschfolgen-Abfrage:** `GetNode`, `ListArtifacts`, `ListDocumentsScoped`, `NodeStats`, `ListBindings` sind alle owner-scoped. `ListBindings` liefert die Bindings **aller Geräte** dieses Owners — deshalb weist jede Bindings-Zeile Maschinen-Label UND Maschinen-ID aus.

---

## File Structure

**Verschoben (Task 0, reiner Refactor):**

- `internal/tui/screen/worktime/wtfmt/` → `internal/timefmt/` — Minuten-Formatierung und HH:MM-/Dauer-Parsing als Leaf-Package unterhalb der Adapter, damit `cmd/flow-mcp` es importieren darf. Acht Importeure im TUI-Baum ziehen mit um. Kein neues Verhalten.

**Geändert außerhalb von `cmd/flow-mcp` (Task 12, nach dem Done-Gate):**

- `internal/adapter/httpserver/projectbindings.go` — zwei `Emit`-Aufrufe für bind und unbind, plus je ein Handler-Test. Schließt die SSE-Bestandslücke, die dieser Slice sichtbar macht.

**Neu (je mit eigener Testdatei):**

- `cmd/flow-mcp/bindtarget.go` — **die eine Ziel-Auflösung.** `path | remote | cwd` → Origin-Slug, Binding-Kind, Maschine, absoluter Pfad. Existenzprüfung, Tilde-Expansion, Ausschluss von `path`+`remote`. Plus die Umgebungs-Injektion (`bindEnv`) und das Mapping Ziel→Wire-Shape.
- `cmd/flow-mcp/bindtarget_test.go` — Ziel-Auflösung inkl. aller Randfälle aus Spec §7.
- `cmd/flow-mcp/format_nodes.go` — **alle Node-Renderer.** Baum, Node-Detail, Bindings-Liste, Resolve-Antwort, Tag-Antwort, Löschfolgen-Bericht, Minuten-Formatierung, Kind-Glyph.
- `cmd/flow-mcp/format_nodes_test.go` — Renderer-Tests.
- `cmd/flow-mcp/tools_bindings.go` — `flow_node_binding` (vier Aktionen) + `flow_bind_project` (Umzug aus `tools_project.go`).
- `cmd/flow-mcp/tools_bindings_test.go`
- `cmd/flow-mcp/tools_node_create.go` — `flow_create_node` + Kind-/Parent-Validierung.
- `cmd/flow-mcp/tools_node_create_test.go`
- `cmd/flow-mcp/tools_node_lifecycle.go` — `flow_move_node`, `flow_delete_node`, Löschfolgen-Erhebung.
- `cmd/flow-mcp/tools_node_lifecycle_test.go`
- `cmd/flow-mcp/tools_node_inspect.go` — `flow_get_node` + der Bindings-Filter auf einen Node.
- `cmd/flow-mcp/tools_node_inspect_test.go`
- `cmd/flow-mcp/tools_node_tags.go` — `flow_set_node_tags`.
- `cmd/flow-mcp/tools_node_tags_test.go`

**Geändert:**

- `cmd/flow-mcp/scope.go` — bekommt `nodeTarget` (Node-Referenz oder gebundener Node, nie „alle"/„unassigned") und `prefixGuard` (Argument-Präfix in Guard-Meldungen). Beide sitzen bewusst neben `lookupNode`, weil das die Node-Referenz-Auflösung dieses Pakets ist. *(Ergänzung zum Dateiplan der Spec §6 — Begründung im Self-Review-Appendix.)*
- `cmd/flow-mcp/bind.go` — `bindNodeCore` verliert den Create-Zweig und nimmt ein aufgelöstes `bindTarget`; neu `bindTargetTo` und `unbindTarget` als Commit-Seite. `validateBindRef` entfällt. `decideBindKind` **bleibt hier** und wird von `bindtarget.go` gerufen — nicht verschieben.
- `cmd/flow-mcp/tools_project.go` — behält `projectContext`, `listProjectsTool`, `updateNode`; verliert `bindNodeIn` und `bindProject` (nach `tools_bindings.go`). `listProjectsTool` rendert über `formatNodeTree`.
- `cmd/flow-mcp/format.go` — `formatProjects` wird gelöscht (Ersatz: `formatNodeTree` in `format_nodes.go`).
- `cmd/flow-mcp/server.go` — sechs neue Registrierungen, zwei neu formulierte Descriptions.
- `cmd/flow-mcp/bind_test.go` — `TestValidateBindRef` und `TestBindNodeCore_CreatePathRejectsMissingMachineBeforeAnyAPIWrite` entfallen (die Guards sind nach `bindtarget.go` gewandert); `TestDecideBindKind` bleibt.
- `cmd/flow-mcp/format_test.go` — `TestFormatProjects` und `TestFormatProjectsIncludesStatus` entfallen (Ersatz in `format_nodes_test.go`).
- `cmd/flow-mcp/loopback_test.go` — die Tool-Zahl-Assertion (`:353`) wandert 25 → 31, ein Schritt pro neuem Tool; `TestLoopback_BindProject` verliert den `create_name`-Zweig.

`format_nodes.go` und `format_nodes_test.go` werden von mehreren Tasks **additiv** erweitert (Task 2 Baum, Task 6 Löschbericht + Minuten, Task 7 Detail, Task 8 Tags, Task 9 Bindings/Resolve). Das ist beabsichtigt: die Datei hat *eine* Verantwortung — Node-Rendering. Kein Task schreibt eine dort schon existierende Funktion um.

---

### Task 0: `wtfmt` nach `internal/timefmt` hochziehen (Vorarbeit, reiner Refactor)

**Warum dieser Task existiert.** Task 6 und Task 7 brauchen eine Minuten-Formatierung (`"12h 30m"`). Die gibt es schon als `wtfmt.FormatMin`, aber `cmd/flow-mcp` dürfte sie nicht importieren: `internal/tui/screen/worktime/wtfmt` liegt im Unterbaum eines anderen Adapters, und ein Adapter-auf-Adapter-Import dreht die hexagonale Abhängigkeitsrichtung, die AGENTS.md festlegt. Soennes Entscheidung vom 2026-07-25: das Package hochziehen statt die Funktion zu kopieren (Offene Entscheidungen #2). Ergebnis ist ein Leaf-Package unterhalb der Adapter, das TUI und MCP gleichermaßen importieren dürfen.

**Dies ist ein reiner Refactor: kein neues Verhalten, kein neuer Test.** Der TDD-Zyklus lautet hier deshalb nicht „failing test first", sondern: die vorhandenen Tests ziehen mit um und müssen unverändert grün bleiben, und danach darf es keine Referenz auf den alten Pfad mehr geben. Wer hier einen neuen Test schreibt, testet Bestandsverhalten doppelt.

**Files:**
- Move: `internal/tui/screen/worktime/wtfmt/wtfmt.go` → `internal/timefmt/timefmt.go`
- Move: `internal/tui/screen/worktime/wtfmt/wtfmt_test.go` → `internal/timefmt/timefmt_test.go`
- Move: `internal/tui/screen/worktime/wtfmt/timeparse.go` → `internal/timefmt/timeparse.go`
- Move: `internal/tui/screen/worktime/wtfmt/timeparse_test.go` → `internal/timefmt/timeparse_test.go`
- Modify (Importpfad + Qualifier `wtfmt.` → `timefmt.`): `internal/tui/screen/worktime/daydetail/dialogs.go`, `internal/tui/screen/worktime/daydetail/route.go`, `internal/tui/screen/worktime/dayoffs/route.go`, `internal/tui/screen/worktime/dialogs.go`, `internal/tui/screen/worktime/format_test.go`, `internal/tui/screen/worktime/statsrange/route.go`, `internal/tui/screen/worktime/week/route.go`, `internal/tui/screen/worktime/week/summary.go`

**Interfaces:**
- Consumes: nichts (erster Task).
- Produces: Package `github.com/serverkraken/flow/internal/timefmt` mit unveränderten Signaturen — `func FormatMin(min int) string` (`"Xh YYm"`, negative Eingabe wird auf 0 geklemmt), `func FormatSaldo(min int) string` (`"+Xh YYm"` / `"-Xh YYm"`), `func ParseHM(s string) (time.Duration, error)`, `func NormalizeDurationArg(s string) string`, `func ParseStop(arg string, start, now time.Time) (time.Time, error)`. Task 6 und Task 7 rufen ausschließlich `timefmt.FormatMin`.

- [ ] **Step 1: Bestand verifizieren** — diese Befehle:

```bash
cat internal/tui/screen/worktime/wtfmt/wtfmt.go
rg -lN "worktime/wtfmt" --type go | sort
rg -oN "wtfmt\.[A-Z][A-Za-z]*" --type go | sort | uniq -c | sort -rn
ls internal/timefmt 2>&1
```

Erwartet: vier Dateien im Package, **zehn** Dateien mit dem Importpfad (die acht Importeure oben plus die zwei Testdateien des Packages selbst, die mit umziehen), die fünf Funktionsnamen aus dem Produces-Block, und `internal/timefmt` existiert noch **nicht**. Weicht die Importeurs-Liste ab, gewinnt der Bestand — dann gilt die Liste aus `rg`, nicht die aus diesem Plan.

- [ ] **Step 2: Package verschieben** — `git mv` erhält die Historie:

```bash
mkdir -p internal/timefmt
git mv internal/tui/screen/worktime/wtfmt/wtfmt.go      internal/timefmt/timefmt.go
git mv internal/tui/screen/worktime/wtfmt/wtfmt_test.go internal/timefmt/timefmt_test.go
git mv internal/tui/screen/worktime/wtfmt/timeparse.go      internal/timefmt/timeparse.go
git mv internal/tui/screen/worktime/wtfmt/timeparse_test.go internal/timefmt/timeparse_test.go
rmdir internal/tui/screen/worktime/wtfmt
```

- [ ] **Step 3: Package-Klausel und Package-Doc umschreiben** — in allen vier verschobenen Dateien `package wtfmt` → `package timefmt`:

```bash
sed -i '' 's/^package wtfmt$/package timefmt/' internal/timefmt/*.go
```

Dann den Doc-Kommentar am Kopf von `internal/timefmt/timefmt.go` ersetzen — der alte Text behauptet eine Worktime-Bindung, die nach dem Umzug nicht mehr stimmt:

```go
// Package timefmt holds minute-based duration formatting and HH:MM/duration
// parsing. It imports only the standard library, so any layer may use it —
// the TUI worktime routes and the MCP node tools both do.
package timefmt
```

- [ ] **Step 4: Importeure umstellen** — Importpfad und Qualifier in einem Durchgang, über die von `rg` gefundene Menge (nicht über eine gepflegte Liste):

```bash
for f in $(rg -lN "worktime/wtfmt" --type go); do
  sed -i '' \
    -e 's|"github.com/serverkraken/flow/internal/tui/screen/worktime/wtfmt"|"github.com/serverkraken/flow/internal/timefmt"|' \
    -e 's|\bwtfmt\.|timefmt.|g' "$f"
done
```

- [ ] **Step 5: Keine Rückstände** — beide Befehle müssen leer bleiben:

```bash
rg -n "wtfmt" --type go
rg -n "worktime/wtfmt" .
```

Ein Treffer heißt: eine Datei wurde übersehen (etwa weil sie den Import in einer ungewöhnlichen Form schreibt). Dann diese Datei von Hand nachziehen und den Befehl erneut laufen lassen.

- [ ] **Step 6: Bestehende Tests grün bestätigen** — der Beweis, dass der Refactor verhaltensneutral war:

```bash
go build ./... && echo BUILD-OK
go test ./internal/timefmt/ -v 2>&1 | tail -20
go test ./internal/tui/... 2>&1 | tail -20
```

Erwartet: `BUILD-OK`, alle Tests des verschobenen Packages PASS (dieselben Testnamen wie vorher), und die TUI-Pakete unverändert grün. Schlägt hier etwas fehl, ist es ein Umzugsfehler und kein Bestandsfehler — Step 4 und 5 prüfen.

- [ ] **Step 7: Lint** — `golangci-lint run ./internal/timefmt/... ./internal/tui/... 2>&1 | tail -20` → keine Funde. **Niemals `make fmt`** (Toolchain-Skew, AGENTS.md).

- [ ] **Step 8: Commit**

```bash
git add -A internal/timefmt internal/tui
git commit -m "refactor: hoist wtfmt to internal/timefmt so non-TUI callers may use it"
```

---

### Task 1: Ziel-Auflösung `bindtarget.go` + geteilte Node-Referenz-Helfer

Der Fundament-Task: alles Weitere konsumiert diese vier Namen. Nichts davon wird registriert, es gibt noch kein neues Tool.

**Files:**
- Create: `cmd/flow-mcp/bindtarget.go`
- Create: `cmd/flow-mcp/bindtarget_test.go`
- Modify: `cmd/flow-mcp/scope.go` (anfügen: `nodeTarget`, `prefixGuard`)
- Modify: `cmd/flow-mcp/server.go` (`refreshResolved` :224-234 — Node-Ref-Cache mit invalidieren)
- Test: `cmd/flow-mcp/bindtarget_test.go`, `cmd/flow-mcp/scope_test.go`

**Interfaces:**
- Consumes: `decideBindKind(kindOverride string, originOK bool) (string, error)` (`cmd/flow-mcp/bind.go:32`); `errGuard{err error}` (`cmd/flow-mcp/tools_write.go:16`); `gitremote.OriginSlug(dir string) (slug string, ok bool, err error)`; `clientmachine.Load() (clientmachine.Machine, error)` mit `Machine{ID, Label string}`; `domain.NormalizeRemoteSlug(raw string) (string, bool)`; `h.lookupNode(ctx context.Context, ref string) (domain.Node, error)` (`scope.go:68`); `h.resolved() (domain.Node, bool)` (`server.go:164`).
- Produces:
  - `type bindTargetArgs struct{ Path, Remote, Kind string }`
  - `type bindEnv struct{ Cwd, Home string; Machine clientmachine.Machine; Origin func(dir string) (string, bool, error) }`
  - `type bindTarget struct{ Kind, RemoteSlug, MachineID, MachineLabel, Path string }`
  - `func liveBindEnv() (bindEnv, error)`
  - `func resolveBindTarget(in bindTargetArgs, env bindEnv) (bindTarget, error)`
  - `func expandHomePath(path, home string) string`
  - `func bindTargetLabel(tgt bindTarget) string`
  - `func bindingFieldsFor(tgt bindTarget) apiclient.BindingFields`
  - `func (h *handlers) nodeTarget(ctx context.Context, ref string) (domain.Node, error)`
  - `func prefixGuard(prefix string, err error) error`

- [ ] **Step 1: Bestandsnamen verifizieren, bevor irgendetwas getippt wird** — genau diese drei Befehle laufen lassen und die Ausgabe lesen. Bei Abweichung gewinnt der Bestand:

```bash
rg -n "func decideBindKind|type errGuard|func \(h \*handlers\) resolved|func \(h \*handlers\) lookupNode" cmd/flow-mcp/
rg -n "func OriginSlug|func Load\(\)|type Machine struct" internal/gitremote/ internal/clientmachine/
rg -n "func NormalizeRemoteSlug" internal/domain/
```

Erwartet: `bind.go:32`, `tools_write.go:16`, `server.go:164`, `scope.go:68`, `gitremote.go:22`, `clientmachine.go` mit `Load() (Machine, error)` und `Machine{ID, Label string}`, `remoteslug.go:13`.

- [ ] **Step 2: Failing test für die Ziel-Auflösung schreiben** — neue Datei `cmd/flow-mcp/bindtarget_test.go`:

```go
package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/clientmachine"
)

// noOrigin / withOrigin are injected git-origin lookups so the tests never run
// git and never depend on the checkout they happen to live in.
func noOrigin(string) (string, bool, error)   { return "", false, nil }
func withOrigin(string) (string, bool, error) { return "github.com/serverkraken/flow", true, nil }

// testBindEnv builds an env whose cwd and home are real temp directories, so the
// existence check runs against the real filesystem without touching $HOME.
func testBindEnv(t *testing.T, origin func(string) (string, bool, error)) bindEnv {
	t.Helper()
	return bindEnv{
		Cwd:     t.TempDir(),
		Home:    t.TempDir(),
		Machine: clientmachine.Machine{ID: "m1", Label: "notebook-a"},
		Origin:  origin,
	}
}

func TestResolveBindTarget_PathAndRemoteAreMutuallyExclusive(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	_, err := resolveBindTarget(bindTargetArgs{Path: env.Cwd, Remote: "github.com/a/b"}, env)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("path+remote err = %v, want a mutually-exclusive guard", err)
	}
}

func TestResolveBindTarget_MissingDirectoryIsAnError(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	missing := filepath.Join(env.Cwd, "definitely-not-here")
	_, err := resolveBindTarget(bindTargetArgs{Path: missing}, env)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing dir err = %v, want a 'does not exist' guard", err)
	}
	var g errGuard
	if !errors.As(err, &g) {
		t.Fatalf("missing dir err type = %T, want errGuard so the model sees it verbatim", err)
	}
}

func TestResolveBindTarget_FileInsteadOfDirectoryIsAnError(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	file := filepath.Join(env.Cwd, "README.md")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := resolveBindTarget(bindTargetArgs{Path: file}, env)
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("file path err = %v, want a 'not a directory' guard", err)
	}
}

func TestResolveBindTarget_OriginPresentYieldsRemoteKind(t *testing.T) {
	env := testBindEnv(t, withOrigin)
	got, err := resolveBindTarget(bindTargetArgs{Path: env.Cwd}, env)
	if err != nil {
		t.Fatalf("resolveBindTarget: %v", err)
	}
	if got.Kind != "remote" || got.RemoteSlug != "github.com/serverkraken/flow" {
		t.Fatalf("got %+v, want remote kind with the origin slug", got)
	}
	if got.Path != filepath.Clean(env.Cwd) {
		t.Fatalf("Path = %q, want the resolved directory (resolve needs all three tiers)", got.Path)
	}
}

func TestResolveBindTarget_NoOriginYieldsPathKindWithMachine(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	got, err := resolveBindTarget(bindTargetArgs{Path: env.Cwd}, env)
	if err != nil {
		t.Fatalf("resolveBindTarget: %v", err)
	}
	if got.Kind != "path" || got.MachineID != "m1" || got.MachineLabel != "notebook-a" {
		t.Fatalf("got %+v, want path kind carrying the machine identity", got)
	}
}

func TestResolveBindTarget_KindRemoteWithoutOriginIsAnError(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	_, err := resolveBindTarget(bindTargetArgs{Path: env.Cwd, Kind: "remote"}, env)
	if err == nil || !strings.Contains(err.Error(), "git origin") {
		t.Fatalf("kind=remote without origin err = %v, want the decideBindKind guard", err)
	}
}

func TestResolveBindTarget_RelativePathResolvesAgainstProcessCwd(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	sub := filepath.Join(env.Cwd, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveBindTarget(bindTargetArgs{Path: "nested"}, env)
	if err != nil {
		t.Fatalf("resolveBindTarget: %v", err)
	}
	if got.Path != filepath.Clean(sub) {
		t.Fatalf("Path = %q, want %q (relative resolves against the MCP process cwd)", got.Path, sub)
	}
}

func TestResolveBindTarget_TildeExpandsAgainstHome(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	sub := filepath.Join(env.Home, "SourceCode")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveBindTarget(bindTargetArgs{Path: "~/SourceCode"}, env)
	if err != nil {
		t.Fatalf("resolveBindTarget: %v", err)
	}
	if got.Path != filepath.Clean(sub) {
		t.Fatalf("Path = %q, want %q (a JSON tool argument never passes through a shell)", got.Path, sub)
	}
}

func TestResolveBindTarget_OmittedPathUsesProcessCwd(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	got, err := resolveBindTarget(bindTargetArgs{}, env)
	if err != nil {
		t.Fatalf("resolveBindTarget: %v", err)
	}
	if got.Path != filepath.Clean(env.Cwd) {
		t.Fatalf("Path = %q, want the process cwd %q", got.Path, env.Cwd)
	}
}

func TestResolveBindTarget_RemoteNeedsNoLocalDirectory(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	got, err := resolveBindTarget(bindTargetArgs{Remote: "git@github.com:serverkraken/flow.git"}, env)
	if err != nil {
		t.Fatalf("resolveBindTarget: %v", err)
	}
	if got.Kind != "remote" || got.RemoteSlug != "github.com/serverkraken/flow" {
		t.Fatalf("got %+v, want a normalized remote binding without any local checkout", got)
	}
	if got.Path != "" {
		t.Fatalf("Path = %q, want empty for a remote-only target", got.Path)
	}
}

func TestResolveBindTarget_UnparseableRemoteIsAnError(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	_, err := resolveBindTarget(bindTargetArgs{Remote: "not a url at all"}, env)
	if err == nil {
		t.Fatal("unparseable remote: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "clone URL") {
		t.Fatalf("error = %v, want it to name the accepted forms", err)
	}
}

// TestResolveBindTarget_BlankArgumentsAreErrorsNotSilentCwdFallbacks: a
// present-but-whitespace argument is a mistake. Trimming it to "" and falling
// back to the process cwd would bind a directory the caller never named.
func TestResolveBindTarget_BlankArgumentsAreErrorsNotSilentCwdFallbacks(t *testing.T) {
	env := testBindEnv(t, noOrigin)

	if _, err := resolveBindTarget(bindTargetArgs{Remote: "   "}, env); err == nil ||
		!strings.Contains(err.Error(), "must not be blank") {
		t.Fatalf("blank remote err = %v, want a 'must not be blank' guard", err)
	}
	if _, err := resolveBindTarget(bindTargetArgs{Path: "  "}, env); err == nil ||
		!strings.Contains(err.Error(), "must not be blank") {
		t.Fatalf("blank path err = %v, want a 'must not be blank' guard", err)
	}
	// Genuinely omitted still means "the process cwd".
	got, err := resolveBindTarget(bindTargetArgs{}, env)
	if err != nil {
		t.Fatalf("omitted arguments must still resolve to the cwd: %v", err)
	}
	if got.Path != filepath.Clean(env.Cwd) {
		t.Fatalf("Path = %q, want the cwd", got.Path)
	}
}

// TestResolveBindTarget_GitExecFailure separates "no origin here" (ok=false,
// err=nil) from "git could not run" (err!=nil, gitremote.go:31). Auto-detect
// degrades to a path binding like bindProject always has; an explicit
// kind="remote" gets the true reason instead of "needs a git origin".
func TestResolveBindTarget_GitExecFailure(t *testing.T) {
	broken := func(string) (string, bool, error) {
		return "", false, errors.New(`exec: "git": executable file not found in $PATH`)
	}
	env := testBindEnv(t, broken)

	got, err := resolveBindTarget(bindTargetArgs{Path: env.Cwd}, env)
	if err != nil {
		t.Fatalf("auto-detect must degrade to a path binding when git cannot run, got %v", err)
	}
	if got.Kind != "path" {
		t.Fatalf("Kind = %q, want path", got.Kind)
	}

	_, err = resolveBindTarget(bindTargetArgs{Path: env.Cwd, Kind: "remote"}, env)
	if err == nil {
		t.Fatal(`kind="remote" with a broken git: want an error`)
	}
	if !strings.Contains(err.Error(), "cannot run git") {
		t.Fatalf("error = %v, want the real reason, not the misleading 'needs a git origin'", err)
	}
}

func TestResolveBindTarget_PathKindWithoutMachineIDIsAnError(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	env.Machine = clientmachine.Machine{}
	_, err := resolveBindTarget(bindTargetArgs{Path: env.Cwd}, env)
	if err == nil || !strings.Contains(err.Error(), "machine id") {
		t.Fatalf("missing machine id err = %v, want the machine-id guard", err)
	}
}

func TestResolveBindTarget_KindPathWithRemoteArgIsAnError(t *testing.T) {
	env := testBindEnv(t, noOrigin)
	_, err := resolveBindTarget(bindTargetArgs{Remote: "github.com/a/b", Kind: "path"}, env)
	if err == nil || !strings.Contains(err.Error(), "local directory") {
		t.Fatalf("kind=path with remote err = %v, want a 'needs a local directory' guard", err)
	}
}

func TestExpandHomePath(t *testing.T) {
	cases := []struct {
		name, in, home, want string
	}{
		{"no tilde", "/abs/path", "/home/x", "/abs/path"},
		{"bare tilde", "~", "/home/x", "/home/x"},
		{"tilde slash", "~/SourceCode/flow", "/home/x", "/home/x/SourceCode/flow"},
		{"no home available", "~/x", "", "~/x"},
		{"bare user form is not expanded", "~other/x", "/home/x", "~other/x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := expandHomePath(c.in, c.home); got != c.want {
				t.Fatalf("expandHomePath(%q, %q) = %q, want %q", c.in, c.home, got, c.want)
			}
		})
	}
}

func TestBindTargetLabelAndBindingFields(t *testing.T) {
	remote := bindTarget{Kind: "remote", RemoteSlug: "github.com/a/b"}
	if got := bindTargetLabel(remote); got != "remote github.com/a/b" {
		t.Errorf("bindTargetLabel(remote) = %q", got)
	}
	rf := bindingFieldsFor(remote)
	if rf.Kind != "remote" || rf.RemoteSlug != "github.com/a/b" || rf.Path != "" {
		t.Errorf("bindingFieldsFor(remote) = %+v", rf)
	}

	path := bindTarget{Kind: "path", MachineID: "m1", MachineLabel: "notebook-a", Path: "/work/flow"}
	if got := bindTargetLabel(path); got != "path /work/flow" {
		t.Errorf("bindTargetLabel(path) = %q", got)
	}
	pf := bindingFieldsFor(path)
	if pf.Kind != "path" || pf.MachineID != "m1" || pf.MachineLabel != "notebook-a" || pf.Path != "/work/flow" {
		t.Errorf("bindingFieldsFor(path) = %+v", pf)
	}
	if pf.RemoteSlug != "" {
		t.Errorf("bindingFieldsFor(path).RemoteSlug = %q, want empty", pf.RemoteSlug)
	}
}
```

- [ ] **Step 3: Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestResolveBindTarget|TestExpandHomePath|TestBindTargetLabel' 2>&1 | head -20` → FAIL mit `undefined: resolveBindTarget`, `undefined: bindTargetArgs`, `undefined: bindEnv`, `undefined: expandHomePath`, `undefined: bindTargetLabel`, `undefined: bindingFieldsFor` (Compile-Fehler des Testpakets).

- [ ] **Step 4: `bindtarget.go` schreiben** — neue Datei `cmd/flow-mcp/bindtarget.go`:

```go
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/gitremote"
)

// bindTargetArgs are the three addressing arguments shared by
// flow_bind_project, all four actions of flow_node_binding, and the optional
// bind_path of flow_create_node (Spec §4). One resolver for all of them is the
// whole point: a binding target must mean the same thing everywhere.
type bindTargetArgs struct {
	Path   string
	Remote string
	Kind   string
}

// bindEnv is the environment the resolver needs, injected so tests never run git
// and never touch the real home directory. Origin has gitremote.OriginSlug's
// signature; Home may be "" when the process has no resolvable home (then a ~
// path stays literal and fails loudly at the existence check).
type bindEnv struct {
	Cwd     string
	Home    string
	Machine clientmachine.Machine
	Origin  func(dir string) (slug string, ok bool, err error)
}

// bindTarget is a resolved binding address. Kind decides which apiclient call
// bind/unbind uses. RemoteSlug, MachineID and Path are ALL populated for a
// directory-derived target, because ResolveNode matches on all three tiers at
// once (internal/adapter/apiclient/projectbindings.go:39) — resolve would
// otherwise lose the path tier for a repo that has a git origin.
type bindTarget struct {
	Kind         string // "remote" | "path"
	RemoteSlug   string // normalized origin slug; "" when the directory has none
	MachineID    string
	MachineLabel string
	Path         string // absolute + cleaned; "" for a remote-only target
}

// liveBindEnv builds the production environment. A missing cwd is fatal (the
// same guard bindProject has had all along); a missing home or machine id is
// best-effort — the ~ expansion and the path-kind branch each fail loudly on
// their own when they actually need what is missing.
func liveBindEnv() (bindEnv, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return bindEnv{}, errGuard{fmt.Errorf("cannot determine the working directory: %v", err)}
	}
	home, _ := os.UserHomeDir()
	machine, _ := clientmachine.Load()
	return bindEnv{Cwd: cwd, Home: home, Machine: machine, Origin: gitremote.OriginSlug}, nil
}

// expandHomePath replaces a leading "~" (bare, or followed by "/") with home.
// A CLI invocation gets the tilde expanded by the shell before the program sees
// it; an MCP tool call hands over a raw JSON string with no shell involved, and
// a model writes "~/SourceCode/foo" (Spec §4). The "~user" form is deliberately
// NOT expanded — it needs a passwd lookup and no model writes it; it stays
// literal and the existence check rejects it loudly.
func expandHomePath(path, home string) string {
	if home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// resolveBindTarget turns path | remote | (neither) into a concrete binding
// target. A directory MUST exist, because gitremote.OriginSlug runs
// `git -C <dir>` and treats every non-zero exit as "no origin" without an error
// (internal/gitremote/gitremote.go:22-33) — a typo in the path would otherwise
// silently become a path binding that never resolves.
func resolveBindTarget(in bindTargetArgs, env bindEnv) (bindTarget, error) {
	path := strings.TrimSpace(in.Path)
	remote := strings.TrimSpace(in.Remote)
	// A present-but-blank argument is a mistake, not an omission. Trimming it to
	// "" and silently falling back to the cwd would bind the wrong directory.
	if path == "" && in.Path != "" {
		return bindTarget{}, errGuard{errors.New(`"path" must not be blank: pass a directory, or omit it to use the flow-mcp process's working directory`)}
	}
	if remote == "" && in.Remote != "" {
		return bindTarget{}, errGuard{errors.New(`"remote" must not be blank: pass a clone URL or a host/path slug, or omit it`)}
	}
	if path != "" && remote != "" {
		return bindTarget{}, errGuard{errors.New(`"path" and "remote" are mutually exclusive: pass a directory in "path", a git remote in "remote", or neither to use the flow-mcp process's working directory`)}
	}

	if remote != "" {
		slug, ok := domain.NormalizeRemoteSlug(remote)
		if !ok {
			return bindTarget{}, errGuard{fmt.Errorf("cannot read a git remote slug from %q; pass a clone URL (git@host:owner/repo.git or https://host/owner/repo) or a host/path slug like github.com/serverkraken/flow", remote)}
		}
		kind, err := decideBindKind(in.Kind, true)
		if err != nil {
			return bindTarget{}, err
		}
		if kind == "path" {
			return bindTarget{}, errGuard{errors.New(`kind "path" needs a local directory: pass it in "path" instead of "remote"`)}
		}
		return bindTarget{Kind: "remote", RemoteSlug: slug}, nil
	}

	dir := env.Cwd
	if path != "" {
		dir = expandHomePath(path, env.Home)
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(env.Cwd, dir)
		}
	}
	dir = filepath.Clean(dir)
	info, statErr := os.Stat(dir)
	if statErr != nil {
		return bindTarget{}, errGuard{fmt.Errorf("path %s does not exist or is not readable: %v", dir, statErr)}
	}
	if !info.IsDir() {
		return bindTarget{}, errGuard{fmt.Errorf("path %s is not a directory; a binding addresses a directory, not a file", dir)}
	}

	originSlug, originOK, originErr := env.Origin(dir)
	// originErr means git itself could not be executed (gitremote.go:31) — NOT
	// "no origin here", which the helper reports as ok=false, err=nil. Auto-detect
	// keeps degrading to a path binding as bindProject always has, but an EXPLICIT
	// kind="remote" must not be answered with the misleading "needs a git origin
	// in this directory" when the truth is that git never ran.
	if originErr != nil && strings.TrimSpace(in.Kind) == "remote" {
		return bindTarget{}, errGuard{fmt.Errorf("cannot run git in %s to read its origin: %v", dir, originErr)}
	}
	kind, err := decideBindKind(in.Kind, originOK)
	if err != nil {
		return bindTarget{}, err
	}
	if kind == "path" && env.Machine.ID == "" {
		return bindTarget{}, errGuard{errors.New("cannot determine this device's machine id for a path binding")}
	}
	return bindTarget{
		Kind: kind, RemoteSlug: originSlug,
		MachineID: env.Machine.ID, MachineLabel: env.Machine.Label, Path: dir,
	}, nil
}

// bindTargetLabel names a resolved target for a result message.
func bindTargetLabel(tgt bindTarget) string {
	if tgt.Kind == "remote" {
		return "remote " + tgt.RemoteSlug
	}
	return "path " + tgt.Path
}

// bindingFieldsFor maps a resolved target onto the wire shape CreateBoundNode
// wants (internal/adapter/apiclient/projectbindings.go:12).
func bindingFieldsFor(tgt bindTarget) apiclient.BindingFields {
	if tgt.Kind == "remote" {
		return apiclient.BindingFields{Kind: "remote", RemoteSlug: tgt.RemoteSlug}
	}
	return apiclient.BindingFields{
		Kind: "path", MachineID: tgt.MachineID, MachineLabel: tgt.MachineLabel, Path: tgt.Path,
	}
}
```

- [ ] **Step 5: Test laufen lassen, grün bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestResolveBindTarget|TestExpandHomePath|TestBindTargetLabel' -v 2>&1 | tail -30` → alle PASS.

- [ ] **Step 6: Failing test für die zwei Node-Referenz-Helfer** — an `cmd/flow-mcp/scope_test.go` anfügen (die Datei hat schon `fakeProjects()` — mit `rg -n "func fakeProjects" -A 12 cmd/flow-mcp/scope_test.go` prüfen, welche Nodes sie liefert, und diese Slugs verwenden):

```go
func TestNodeTarget_ExplicitRefWins(t *testing.T) {
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) { return fakeProjects(), nil }}
	h.proj, h.matched = domain.Node{ID: "bound1", Name: "Bound", Slug: "bound"}, true

	got, err := h.nodeTarget(context.Background(), fakeProjects()[0].Slug)
	if err != nil {
		t.Fatalf("nodeTarget: %v", err)
	}
	if got.ID != fakeProjects()[0].ID {
		t.Fatalf("nodeTarget(explicit) = %q, want the named node %q", got.ID, fakeProjects()[0].ID)
	}
}

func TestNodeTarget_OmittedUsesBoundNode(t *testing.T) {
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) { return fakeProjects(), nil }}
	h.proj, h.matched = domain.Node{ID: "bound1", Name: "Bound", Slug: "bound"}, true

	got, err := h.nodeTarget(context.Background(), "  ")
	if err != nil {
		t.Fatalf("nodeTarget: %v", err)
	}
	if got.ID != "bound1" {
		t.Fatalf("nodeTarget(\"\") = %q, want the directory-bound node", got.ID)
	}
}

func TestNodeTarget_OmittedAndUnboundIsAnActionableError(t *testing.T) {
	h := &handlers{listProjects: func(context.Context) ([]domain.Node, error) { return fakeProjects(), nil }}
	// h.matched stays false: nothing is bound to this directory.
	_, err := h.nodeTarget(context.Background(), "")
	if err == nil {
		t.Fatal("unbound nodeTarget: want an error, got nil")
	}
	for _, want := range []string{"flow_node_binding", "flow_bind_project", "node="} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must name %q so the model knows how to fix it", err.Error(), want)
		}
	}
	var g errGuard
	if !errors.As(err, &g) {
		t.Fatalf("error type = %T, want errGuard", err)
	}
}

func TestPrefixGuard(t *testing.T) {
	guard := prefixGuard("parent", errGuard{errors.New(`unknown project "x"`)})
	var g errGuard
	if !errors.As(guard, &g) {
		t.Fatalf("prefixGuard dropped the guard type: %T", guard)
	}
	if !strings.HasPrefix(guard.Error(), "parent: ") {
		t.Fatalf("prefixGuard message = %q, want a 'parent: ' prefix", guard.Error())
	}
	// A transport/auth failure must NOT be downgraded to a validation error.
	transport := errors.New("flow server error listing projects: dial tcp: refused")
	if got := prefixGuard("parent", transport); got != transport {
		t.Fatalf("prefixGuard(transport) = %v, want the original error untouched", got)
	}
}
```

> `scope_test.go` importiert heute `context`, `strings`, `testing` und `domain` — `errors` ggf. ergänzen. Mit `rg -n "^import" -A 10 cmd/flow-mcp/scope_test.go` prüfen.

- [ ] **Step 7: Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestNodeTarget|TestPrefixGuard' 2>&1 | head -20` → FAIL mit `h.nodeTarget undefined` und `undefined: prefixGuard`.

- [ ] **Step 8: `nodeTarget` + `prefixGuard` an `scope.go` anfügen** — am Ende von `cmd/flow-mcp/scope.go`:

```go
// nodeTarget resolves a tool's optional `node` argument to the node it names,
// or — when omitted — to the node bound to this directory. Unlike resolveScope it
// never yields the "all projects" or "unassigned" scopes: every tool in the node
// family acts on exactly one node. The miss message names both binding tools
// because either one fixes it (Spec §3 flow_get_node).
//
// Only Node.ID is guaranteed fresh. The omitted branch returns the auth-time
// resolved snapshot, so a node renamed since then still carries its old
// Name/Slug and has no LogoRef. A caller that PRINTS those fields must re-read
// the node with GetNode by ID (flow_get_node does exactly that); a caller that
// only needs the ID to address a mutation may use the result directly.
func (h *handlers) nodeTarget(ctx context.Context, ref string) (domain.Node, error) {
	if r := strings.TrimSpace(ref); r != "" {
		return h.lookupNode(ctx, r)
	}
	if node, matched := h.resolved(); matched {
		return node, nil
	}
	return domain.Node{}, errGuard{errors.New(`no node is bound to this directory: pass node=<slug/name/id>, or bind this directory with flow_node_binding (action="bind") or flow_bind_project`)}
}

// prefixGuard prefixes a guard error's message so the model learns WHICH
// argument was bad. A non-guard error (transport, auth, server) is returned
// untouched — wrapping it in errGuard would downgrade a server failure to
// invalid_request and tell the model to fix its arguments instead of retrying.
func prefixGuard(prefix string, err error) error {
	var g errGuard
	if errors.As(err, &g) {
		return errGuard{fmt.Errorf("%s: %s", prefix, g.Error())}
	}
	return err
}
```

- [ ] **Step 9: Test laufen lassen, grün bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestNodeTarget|TestPrefixGuard' -v 2>&1 | tail -20` → PASS. Danach das ganze Paket: `go test ./cmd/flow-mcp/ 2>&1 | tail -5` → ok (nichts Bestehendes ist angefasst).

- [ ] **Step 10: Failing test für die Cache-Invalidierung bei Identitätswechsel** — an `cmd/flow-mcp/scope_test.go` anfügen. Der Node-Ref-Cache (`h.projects`, `h.projFetched`) wird heute nirgends geleert; `refreshResolved` läuft aber nach **jedem** erfolgreichen Client-Neuaufbau, und ein Neuaufbau kann eine andere Identität tragen. Ein überlebender Cache würde Owner B die Slugs von Owner A zeigen — `lookupNode` listet sie in seiner Miss-Meldung auf (`scope.go:87`). Das ist ein Tenant-Leak und in AGENTS.md ein Critical-Finding:

```go
// TestRefreshResolved_DropsTheNodeCache pins the tenant boundary: the node-ref
// cache must not survive an authenticated client rebuild, because the rebuilt
// client can belong to a different owner. Without this, lookupNode's
// "known slugs: …" message leaks the previous owner's slugs (scope.go:87).
func TestRefreshResolved_DropsTheNodeCache(t *testing.T) {
	var fetches int
	ownerASlugs := []domain.Node{{ID: "a1", Name: "Owner A Node", Slug: "owner-a-secret"}}
	ownerBSlugs := []domain.Node{{ID: "b1", Name: "Owner B Node", Slug: "owner-b-node"}}

	h := &handlers{resources: map[string]string{}}
	h.listProjects = func(context.Context) ([]domain.Node, error) {
		fetches++
		if fetches == 1 {
			return ownerASlugs, nil
		}
		return ownerBSlugs, nil
	}

	// Owner A warms the cache.
	if _, err := h.nodeList(context.Background(), false); err != nil {
		t.Fatalf("warm cache: %v", err)
	}
	h.projMu.Lock()
	warmed := h.projFetched
	h.projMu.Unlock()
	if !warmed {
		t.Fatal("cache was not warmed")
	}

	// A client rebuild happens (new identity). refreshResolved must invalidate.
	h.refreshResolved(context.Background(), nil)

	h.projMu.Lock()
	stillFetched, cached := h.projFetched, h.projects
	h.projMu.Unlock()
	if stillFetched || cached != nil {
		t.Fatalf("cache survived the rebuild: projFetched=%v projects=%v", stillFetched, cached)
	}

	// The next lookup must therefore see Owner B only.
	_, err := h.lookupNode(context.Background(), "owner-a-secret")
	if err == nil {
		t.Fatal("Owner A's slug still resolves after the identity change")
	}
	if strings.Contains(err.Error(), "owner-a-secret") {
		t.Fatalf("the miss message leaks the previous owner's slug: %v", err)
	}
	if !strings.Contains(err.Error(), "owner-b-node") {
		t.Fatalf("the miss message should list the CURRENT owner's slugs, got: %v", err)
	}
}
```

> `refreshResolved` ruft `resolveProject(ctx, c, mcpLog())` mit `c == nil` — das ist im Test unkritisch, weil `projectresolve.Resolve` den Fehler zu „kein Projekt" degradiert (`resolve.go:22-26`) und `reconcileResourcesLocked` nur eine Warnung loggt. Vor dem Tippen mit `rg -n "func (h \*handlers) refreshResolved" -A 14 cmd/flow-mcp/server.go` und `rg -n "func resolveProject" -A 14 cmd/flow-mcp/resolve.go` bestätigen.

- [ ] **Step 11: Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run TestRefreshResolved_DropsTheNodeCache -v 2>&1 | head -20` → FAIL mit `cache survived the rebuild: projFetched=true projects=[...]`.

- [ ] **Step 12: Cache-Invalidierung in `refreshResolved` einbauen** — in `cmd/flow-mcp/server.go`, in `refreshResolved`, die `projMu`-Sektion um zwei Zuweisungen erweitern:

```go
	h.projMu.Lock()
	h.proj, h.matched = proj, matched
	// Drop the node-ref cache with it. refreshResolved runs after EVERY
	// authenticated client build, and a rebuild can carry a different identity —
	// a surviving cache would let one owner resolve or even see another owner's
	// slugs (lookupNode lists them in its miss message). The cost is one extra
	// ListNodes on the next lookup; the alternative is a cross-tenant leak.
	h.projects, h.projFetched = nil, false
	h.projMu.Unlock()
```

Der Rest von `refreshResolved` bleibt unverändert. Aufrufer sind davon unbelastet: `createNode` invalidiert bereits selbst vor `refreshResolved`, und jeder `lookupNode` holt bei kaltem Cache neu.

- [ ] **Step 13: Test laufen lassen, grün bestätigen** — `go test ./cmd/flow-mcp/ -run TestRefreshResolved_DropsTheNodeCache -v 2>&1 | tail -10` → PASS. Dann das ganze Paket: `go test ./cmd/flow-mcp/ 2>&1 | tail -5` → ok. Insbesondere `TestLoopback_BindProject` muss grün bleiben (es ruft `flow_bind_project`, das `refreshResolved` triggert, und danach `flow_project_context`).

- [ ] **Step 14: Commit** — `git add cmd/flow-mcp/bindtarget.go cmd/flow-mcp/bindtarget_test.go cmd/flow-mcp/scope.go cmd/flow-mcp/scope_test.go cmd/flow-mcp/server.go && git commit -m "feat(flow-mcp): shared bind-target resolution, node-reference helpers, tenant-safe cache invalidation"`

---

### Task 2: `flow_list_projects` rendert die Hierarchie

`formatProjects` (flach, alphabetisch, ohne `kind` und `parent`) wird durch einen Baum-Renderer ersetzt. Genau diese beiden Felder braucht `flow_create_node`, um einen gültigen Parent zu wählen — ohne sie rät das Modell (Spec §3).

**Files:**
- Create: `cmd/flow-mcp/format_nodes.go`
- Create: `cmd/flow-mcp/format_nodes_test.go`
- Modify: `cmd/flow-mcp/format.go` (`formatProjects` :168-184 löschen)
- Modify: `cmd/flow-mcp/format_test.go` (`TestFormatProjects` :96, `TestFormatProjectsIncludesStatus` :112 löschen)
- Modify: `cmd/flow-mcp/tools_project.go` (`listProjectsTool` :70 → `formatNodeTree`)
- Modify: `cmd/flow-mcp/server.go` (Description von `flow_list_projects` :103-106)

**Interfaces:**
- Consumes: `domain.Node{ID, Name, Slug string; Status domain.NodeStatus; Kind domain.NodeKind; ParentID *string; UpstreamGit string}`; `domain.KindEngagement/KindVorhaben/KindRepo/KindBranch`.
- Produces:
  - `type nodeTreeEntry struct{ Node domain.Node; Children []*nodeTreeEntry }`
  - `func buildNodeForest(nodes []domain.Node) []*nodeTreeEntry`
  - `func formatNodeTree(nodes []domain.Node) string`
  - `func nodeKindGlyph(k domain.NodeKind) string`

- [ ] **Step 1: Bestand verifizieren** — diese drei Befehle laufen lassen:

```bash
rg -n "func formatProjects" -A 18 cmd/flow-mcp/format.go
rg -n "formatProjects" cmd/flow-mcp/
rg -n "KindEngagement|KindVorhaben|KindRepo|KindBranch|NodeActive|NodePaused|NodeArchived" internal/domain/node.go
```

Erwartet: `formatProjects` in `format.go:168`, Aufrufe nur in `tools_project.go:70` und `format_test.go:101,113`; die vier Kind-Konstanten bei `node.go:21-24` und die drei Status-Konstanten bei `node.go:12-14` — genau diese Namen verwenden Renderer und Fixtures. Gibt es weitere `formatProjects`-Aufrufer, gewinnt der Bestand und sie sind mit umzustellen.

- [ ] **Step 2: Failing test schreiben** — neue Datei `cmd/flow-mcp/format_nodes_test.go`:

```go
package main

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// treeFixture is an engagement with a vorhaben and a repo under it, plus a
// second root — enough to assert indentation, ordering and kind per line.
func treeFixture() []domain.Node {
	eng, vor := "e1", "v1"
	return []domain.Node{
		{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, ParentID: &vor, Status: domain.NodeActive},
		{ID: "e2", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement, Status: domain.NodePaused},
		{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben, ParentID: &eng, Status: domain.NodeActive},
		{ID: "e1", Name: "Zeta", Slug: "zeta", Kind: domain.KindEngagement, Status: domain.NodeActive},
	}
}

func TestFormatNodeTree_IndentsAndNamesKindPerLine(t *testing.T) {
	out := formatNodeTree(treeFixture())
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("tree has %d lines, want 4:\n%s", len(lines), out)
	}
	// Roots alphabetical: Alpha before Zeta.
	if !strings.Contains(lines[0], "Alpha") || !strings.Contains(lines[1], "Zeta") {
		t.Fatalf("roots not alphabetical:\n%s", out)
	}
	// Indentation: the vorhaben under Zeta is deeper than Zeta, the repo deeper still.
	if strings.HasPrefix(lines[2], "  ") == false || strings.HasPrefix(lines[3], "    ") == false {
		t.Fatalf("children are not indented two spaces per level:\n%s", out)
	}
	if strings.HasPrefix(lines[0], " ") {
		t.Fatalf("a root must not be indented:\n%s", out)
	}
	// Every line carries kind, slug, status and id — what flow_create_node needs.
	for _, want := range []string{"engagement", "vorhaben", "repo", "jukebox", "rebuild", "paused", "active", "e1", "v1", "r1"} {
		if !strings.Contains(out, want) {
			t.Errorf("tree missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatNodeTree_UpstreamIsShownWhenSet(t *testing.T) {
	out := formatNodeTree([]domain.Node{
		{ID: "r1", Name: "Flow", Slug: "flow", Kind: domain.KindRepo, Status: domain.NodeActive,
			UpstreamGit: "git@github.com:serverkraken/flow.git"},
	})
	if !strings.Contains(out, "github.com") {
		t.Fatalf("tree must show upstream when set, got %q", out)
	}
}

func TestFormatNodeTree_EmptyPointsAtCreateNode(t *testing.T) {
	out := formatNodeTree(nil)
	if out == "" {
		t.Fatal("formatNodeTree(nil) must return a non-empty message")
	}
	if !strings.Contains(out, "flow_create_node") {
		t.Fatalf("empty message = %q, want it to name flow_create_node (create_name is gone)", out)
	}
	if strings.Contains(out, "create_name") {
		t.Fatalf("empty message = %q, must not mention the removed create_name parameter", out)
	}
}

func TestBuildNodeForest_DanglingParentBecomesARootInsteadOfVanishing(t *testing.T) {
	absent := "not-in-this-list"
	roots := buildNodeForest([]domain.Node{
		{ID: "x1", Name: "Orphan", Slug: "orphan", Kind: domain.KindRepo, ParentID: &absent},
		{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement},
	})
	if len(roots) != 2 {
		t.Fatalf("forest has %d roots, want 2 (a dangling parent must not hide the node)", len(roots))
	}
	// True roots come before dangling ones.
	if roots[0].Node.ID != "e1" || roots[1].Node.ID != "x1" {
		t.Fatalf("root order = %s,%s; want the true root first", roots[0].Node.ID, roots[1].Node.ID)
	}
}

func TestBuildNodeForest_SiblingsAreNameSortedCaseInsensitively(t *testing.T) {
	parent := "e1"
	roots := buildNodeForest([]domain.Node{
		{ID: "e1", Name: "Root", Slug: "root", Kind: domain.KindEngagement},
		{ID: "b", Name: "beta", Slug: "beta", Kind: domain.KindVorhaben, ParentID: &parent},
		{ID: "a", Name: "Alpha", Slug: "alpha", Kind: domain.KindVorhaben, ParentID: &parent},
	})
	if len(roots) != 1 || len(roots[0].Children) != 2 {
		t.Fatalf("unexpected forest shape: %+v", roots)
	}
	if roots[0].Children[0].Node.ID != "a" {
		t.Fatalf("siblings not case-insensitively name-sorted: %s before %s",
			roots[0].Children[0].Node.Name, roots[0].Children[1].Node.Name)
	}
}

// TestFormatNodeTree_LongNamesAndSlugsPassThroughVerbatim pins the "long" state
// for this surface. A model, not a 375px viewport, reads this output, and a
// truncated slug or id is an UNUSABLE address — it would be passed straight back
// into flow_get_node and fail. So single values are never shortened; only
// repeated enumerations are capped (see joinCapped in the delete report).
func TestFormatNodeTree_LongNamesAndSlugsPassThroughVerbatim(t *testing.T) {
	longName := strings.Repeat("Sehr-Langer-Engagement-Name-", 12)
	longSlug := strings.Repeat("langer-slug-", 12) + "ende"
	out := formatNodeTree([]domain.Node{
		{ID: "e1", Name: longName, Slug: longSlug, Kind: domain.KindEngagement, Status: domain.NodeActive},
	})
	if !strings.Contains(out, longName) {
		t.Errorf("a long name must not be truncated:\n%s", out)
	}
	if !strings.Contains(out, longSlug) {
		t.Errorf("a long slug must not be truncated — it is the node's address:\n%s", out)
	}
	if strings.Contains(out, "…") {
		t.Errorf("no ellipsis may appear in a single value:\n%s", out)
	}
	if n := strings.Count(out, "\n"); n != 0 {
		t.Errorf("one node must stay one line, got %d newlines:\n%s", n, out)
	}
}

func TestNodeKindGlyph_UsesMonospaceGlyphsOnly(t *testing.T) {
	for _, k := range []domain.NodeKind{domain.KindEngagement, domain.KindVorhaben, domain.KindRepo, domain.KindBranch} {
		g := nodeKindGlyph(k)
		if g == "" {
			t.Fatalf("nodeKindGlyph(%s) is empty", k)
		}
		if len([]rune(g)) != 1 {
			t.Fatalf("nodeKindGlyph(%s) = %q, want exactly one glyph rune (AGENTS.md bans emoji pictograms)", k, g)
		}
	}
	if nodeKindGlyph(domain.NodeKind("bogus")) == "" {
		t.Fatal("an unknown kind must still get a fallback glyph")
	}
}
```

- [ ] **Step 3: Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestFormatNodeTree|TestBuildNodeForest|TestNodeKindGlyph' 2>&1 | head -20` → FAIL mit `undefined: formatNodeTree`, `undefined: buildNodeForest`, `undefined: nodeKindGlyph`.

- [ ] **Step 4: `format_nodes.go` schreiben** — neue Datei `cmd/flow-mcp/format_nodes.go`:

```go
package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// nodeKindGlyph maps a node kind to a monospace glyph. AGENTS.md bans emoji
// pictograms and sanctions ● ◆ ⬡ ▶ ■. Every line also prints the kind word, so
// the glyph is a scanning aid and never information a model has to decode.
func nodeKindGlyph(k domain.NodeKind) string {
	switch k {
	case domain.KindEngagement:
		return "●"
	case domain.KindVorhaben:
		return "◆"
	case domain.KindRepo:
		return "⬡"
	case domain.KindBranch:
		return "▶"
	default:
		return "·"
	}
}

// nodeTreeEntry is a node plus its name-sorted children.
type nodeTreeEntry struct {
	Node     domain.Node
	Children []*nodeTreeEntry
}

// buildNodeForest groups a flat node list into a parent→children forest. A node
// whose ParentID is nil is a root; a node whose ParentID points at an ID the
// list does not contain becomes a root too, rather than vanishing — hiding a
// node the owner does own would be worse than showing it unindented. Roots and
// every child level are name-sorted case-insensitively, true roots before
// dangling ones. Acyclicity is a server invariant (MoveNode rejects cycles with
// usecase.ErrNodeCycle), so the recursion always terminates.
func buildNodeForest(nodes []domain.Node) []*nodeTreeEntry {
	byID := make(map[string]*nodeTreeEntry, len(nodes))
	for i := range nodes {
		byID[nodes[i].ID] = &nodeTreeEntry{Node: nodes[i]}
	}
	var roots, dangling []*nodeTreeEntry
	// Iterate the slice, not the map, so the pre-sort order is deterministic.
	for i := range nodes {
		entry := byID[nodes[i].ID]
		pid := entry.Node.ParentID
		if pid == nil {
			roots = append(roots, entry)
			continue
		}
		if parent, ok := byID[*pid]; ok {
			parent.Children = append(parent.Children, entry)
			continue
		}
		dangling = append(dangling, entry)
	}
	var sortRec func(ts []*nodeTreeEntry)
	sortRec = func(ts []*nodeTreeEntry) {
		sort.Slice(ts, func(i, j int) bool {
			return strings.ToLower(ts[i].Node.Name) < strings.ToLower(ts[j].Node.Name)
		})
		for _, t := range ts {
			sortRec(t.Children)
		}
	}
	sortRec(roots)
	sortRec(dangling)
	return append(roots, dangling...)
}

// formatNodeTree renders the hierarchy indented two spaces per level, one line
// per node: kind glyph, name, slug, kind, status, id, and upstream when set.
// The flat alphabetical predecessor showed neither kind nor parent — exactly the
// information flow_create_node needs to pick a valid parent (Spec §3).
func formatNodeTree(nodes []domain.Node) string {
	if len(nodes) == 0 {
		return `No nodes yet. Create the first one with flow_create_node (kind="engagement", no parent).`
	}
	var b strings.Builder
	var walk func(ts []*nodeTreeEntry, depth int)
	walk = func(ts []*nodeTreeEntry, depth int) {
		for _, t := range ts {
			n := t.Node
			fmt.Fprintf(&b, "%s%s %s (%s) — %s — %s — %s",
				strings.Repeat("  ", depth), nodeKindGlyph(n.Kind), n.Name, n.Slug, n.Kind, n.Status, n.ID)
			if n.UpstreamGit != "" {
				fmt.Fprintf(&b, " — %s", n.UpstreamGit)
			}
			b.WriteByte('\n')
			walk(t.Children, depth+1)
		}
	}
	walk(buildNodeForest(nodes), 0)
	return strings.TrimRight(b.String(), "\n")
}
```

- [ ] **Step 5: Test laufen lassen, grün bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestFormatNodeTree|TestBuildNodeForest|TestNodeKindGlyph' -v 2>&1 | tail -25` → PASS.

- [ ] **Step 6: `formatProjects` entfernen und den Aufrufer umstellen** — drei Änderungen:

In `cmd/flow-mcp/format.go` den kompletten Block `formatProjects` (Zeilen 166-184, Doc-Kommentar eingeschlossen) löschen. Danach prüfen, ob `sort` noch benutzt wird (`formatTags` nutzt `sort.SliceStable`) — ja, der Import bleibt.

In `cmd/flow-mcp/tools_project.go`, in `listProjectsTool`, die Ergebniszeile ersetzen:

```go
	return textResult(formatNodeTree(ps)), nil, nil
```

In `cmd/flow-mcp/format_test.go` die beiden Tests `TestFormatProjects` (Zeilen 96-110) und `TestFormatProjectsIncludesStatus` (Zeilen 112-122) löschen. Ihre Zusicherungen leben jetzt in `format_nodes_test.go`.

- [ ] **Step 7: Description von `flow_list_projects` neu fassen** — in `cmd/flow-mcp/server.go` den Block bei `:103-106` ersetzen:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_list_projects",
		Description: "List the complete flow node hierarchy as an indented tree — kind glyph, name, slug, kind, status and id per line, two spaces per level. Use this to find an existing node before binding, and to pick a valid parent for flow_create_node (an engagement is always a root; a vorhaben or repo needs an engagement or vorhaben as parent).",
	}, h.listProjectsTool)
```

- [ ] **Step 8: Ganzes Paket laufen lassen, grün bestätigen** — `go test ./cmd/flow-mcp/ 2>&1 | tail -10` → ok. Insbesondere muss `TestLoopback_BindProject` weiter grün sein (es prüft `strings.Contains(lpTxt, "Alpha")` und `"alpha"` — der Baum enthält beides) und die Tool-Zahl bleibt 25.

- [ ] **Step 9: Commit** — `git add cmd/flow-mcp/format_nodes.go cmd/flow-mcp/format_nodes_test.go cmd/flow-mcp/format.go cmd/flow-mcp/format_test.go cmd/flow-mcp/tools_project.go cmd/flow-mcp/server.go && git commit -m "feat(flow-mcp): render flow_list_projects as the node hierarchy"`

---

### Task 3: `flow_bind_project` — `path` + `remote` rein, Create-Zweig raus

`flow_bind_project` bleibt die Kurzform für den Alltagsfall, bindet aber nicht mehr nur das cwd des MCP-Prozesses und legt nichts mehr an. Der bisherige Aufruf (`project` allein) bleibt unverändert gültig (Spec §3).

**Files:**
- Create: `cmd/flow-mcp/tools_bindings.go` (`bindNodeIn` + `bindProject`, Umzug aus `tools_project.go`)
- Create: `cmd/flow-mcp/tools_bindings_test.go`
- Modify: `cmd/flow-mcp/bind.go` (`validateBindRef` :18-28 löschen; `bindNodeCore` :51-111 neu; `bindTargetTo` + `unbindTarget` neu; `decideBindKind` :32-49 unverändert stehen lassen)
- Modify: `cmd/flow-mcp/tools_project.go` (`bindNodeIn` :73-79 und `bindProject` :81-107 entfernen)
- Modify: `cmd/flow-mcp/bind_test.go` (`TestValidateBindRef` :11-32 und `TestBindNodeCore_CreatePathRejectsMissingMachineBeforeAnyAPIWrite` :63-73 löschen)
- Modify: `cmd/flow-mcp/server.go` (Description von `flow_bind_project` :107-110)
- Modify: `cmd/flow-mcp/loopback_test.go` (`TestLoopback_BindProject` :337-434 — Create-Zweig ersetzen)

**Interfaces:**
- Consumes: `resolveBindTarget(in bindTargetArgs, env bindEnv) (bindTarget, error)`, `liveBindEnv() (bindEnv, error)`, `bindTargetLabel(tgt bindTarget) string`, `bindTarget{Kind, RemoteSlug, MachineID, MachineLabel, Path string}` (Task 1); `h.lookupNode(ctx, ref string) (domain.Node, error)`; `h.do(ctx, req, func(*apiclient.Client) error) error`; `h.refreshResolved(ctx, c *apiclient.Client)`; `c.BindRemote(ctx, nodeID, remoteSlug string) (domain.ProjectBinding, error)`; `c.BindPath(ctx, nodeID, machineID, machineLabel, path string) (domain.ProjectBinding, error)`; `c.UnbindRemote(ctx, remoteSlug string) error`; `c.UnbindPath(ctx, machineID, path string) error`.
- Produces:
  - `type bindNodeIn struct{ Project, Path, Remote, Kind string }` (mit json/jsonschema-Tags)
  - `func (h *handlers) bindProject(ctx context.Context, req *mcp.CallToolRequest, in bindNodeIn) (*mcp.CallToolResult, any, error)`
  - `func (h *handlers) bindNodeCore(ctx context.Context, c *apiclient.Client, nodeRef string, tgt bindTarget) (domain.Node, error)`
  - `func bindTargetTo(ctx context.Context, c *apiclient.Client, nodeID string, tgt bindTarget) error`
  - `func unbindTarget(ctx context.Context, c *apiclient.Client, tgt bindTarget) error`

- [ ] **Step 1: Bestand verifizieren** — diese drei Befehle laufen lassen:

```bash
rg -n "bindNodeIn|bindNodeCore|validateBindRef|create_name|CreateName|CreateParent" cmd/flow-mcp/
rg -n "func \(c \*Client\) (BindRemote|BindPath|UnbindRemote|UnbindPath)" -A 6 internal/adapter/apiclient/projectbindings.go
rg -n "type User struct" -A 8 internal/domain/user.go
```

Erwartet: die `create_name`-Fundstellen in `tools_project.go:73-79`, `bind.go:18,51`, `bind_test.go:17-22,67`, `loopback_test.go:384-385,406-407`, `server.go:109` (jede weitere gewinnt und muss mit umgestellt werden); `BindRemote(ctx, nodeID, remoteSlug string) (domain.ProjectBinding, error)` und `BindPath(ctx, nodeID, machineID, machineLabel, path string) (domain.ProjectBinding, error)`, beide mit Node — im Gegensatz zu `UnbindRemote(ctx, remoteSlug)` / `UnbindPath(ctx, machineID, path)` ohne; und die `domain.User`-Felder, die jeder Fake-Backend für `/api/v1/me` encodiert (`ID`, `DisplayName`, `Email` — so wie `loopback_test.go:22` es schon tut).

- [ ] **Step 2: Failing test schreiben** — neue Datei `cmd/flow-mcp/tools_bindings_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// bindRecorder captures which binding endpoint was hit with which body, so the
// tests can prove a path argument really became a path binding on that directory
// instead of the MCP process's own cwd.
type bindRecorder struct {
	mu           sync.Mutex
	bindNodeID   string
	bindKind     string
	bindRemote   string
	bindMachine  string
	bindPath     string
	bindCalls    int
	unbindQuery  string
	unbindCalls  int
	createBounds int
}

func (r *bindRecorder) snapshot() bindRecorder {
	r.mu.Lock()
	defer r.mu.Unlock()
	return bindRecorder{
		bindNodeID: r.bindNodeID, bindKind: r.bindKind, bindRemote: r.bindRemote,
		bindMachine: r.bindMachine, bindPath: r.bindPath, bindCalls: r.bindCalls,
		unbindQuery: r.unbindQuery, unbindCalls: r.unbindCalls, createBounds: r.createBounds,
	}
}

// fakeBindingBackend serves every endpoint the binding family touches.
// Nodes: e1/alpha (engagement), v1/rebuild (vorhaben under e1), r1/jukebox (repo under v1).
func fakeBindingBackend(t *testing.T, rec *bindRecorder) *httptest.Server {
	t.Helper()
	e1, v1 := "e1", "v1"
	nodes := []domain.Node{
		{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement, Status: domain.NodeActive},
		{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben, ParentID: &e1, Status: domain.NodeActive},
		{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, ParentID: &v1, Status: domain.NodeActive},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(nodes)
	})
	mux.HandleFunc("PUT /api/v1/nodes/{id}/bindings", func(w http.ResponseWriter, r *http.Request) {
		var body apiclient.BindingFields
		_ = json.NewDecoder(r.Body).Decode(&body)
		rec.mu.Lock()
		rec.bindNodeID, rec.bindKind = r.PathValue("id"), body.Kind
		rec.bindRemote, rec.bindMachine, rec.bindPath = body.RemoteSlug, body.MachineID, body.Path
		rec.bindCalls++
		rec.mu.Unlock()
		_ = json.NewEncoder(w).Encode(domain.ProjectBinding{ID: "b1", NodeID: r.PathValue("id")})
	})
	mux.HandleFunc("DELETE /api/v1/nodes/bindings", func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.unbindQuery = r.URL.RawQuery
		rec.unbindCalls++
		rec.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/nodes/create-bound", func(w http.ResponseWriter, _ *http.Request) {
		rec.mu.Lock()
		rec.createBounds++
		rec.mu.Unlock()
		http.Error(w, "create-bound must not be reachable from flow_bind_project any more", http.StatusInternalServerError)
	})
	return httptest.NewServer(mux)
}

// authedBindingServer builds a loopback session bound to r1/jukebox.
func authedBindingServer(t *testing.T) (*mcp.ClientSession, *bindRecorder) {
	t.Helper()
	rec := &bindRecorder{}
	be := fakeBindingBackend(t, rec)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo})
	return connect(t, h.srv), rec
}

func TestLoopback_BindProject_SchemaHasPathAndRemoteAndNoCreateBranch(t *testing.T) {
	sess, _ := authedBindingServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name != "flow_bind_project" {
			continue
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("input schema type = %T, want map", tool.InputSchema)
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("schema has no properties: %#v", schema)
		}
		if props["path"] == nil || props["remote"] == nil {
			t.Errorf("flow_bind_project schema must offer path and remote, got %v", keysOf(props))
		}
		if props["create_name"] != nil || props["create_parent"] != nil {
			t.Errorf("flow_bind_project must no longer offer create_name/create_parent, got %v", keysOf(props))
		}
		return
	}
	t.Fatal("flow_bind_project not advertised")
}

// keysOf is a stable-ish helper for assertion messages only.
func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestLoopback_BindProject_PathArgumentBindsThatDirectoryNotTheProcessCwd(t *testing.T) {
	sess, rec := authedBindingServer(t)
	dir := t.TempDir() // a real directory with no git origin → path binding

	res, out := callText(t, sess, "flow_bind_project", map[string]any{
		"project": "jukebox", "path": dir, "kind": "path",
	})
	if res.IsError {
		t.Fatalf("bind with path errored: %s", out)
	}
	got := rec.snapshot()
	if got.bindCalls != 1 || got.bindNodeID != "r1" || got.bindKind != "path" {
		t.Fatalf("recorder = %+v, want exactly one path bind on r1", got)
	}
	if got.bindPath != dir {
		t.Fatalf("bound path = %q, want the passed directory %q (not the MCP process cwd)", got.bindPath, dir)
	}
	if got.createBounds != 0 {
		t.Fatalf("create-bound was called %d times; flow_bind_project must never create", got.createBounds)
	}
}

func TestLoopback_BindProject_RemoteArgumentNeedsNoLocalDirectory(t *testing.T) {
	sess, rec := authedBindingServer(t)

	res, out := callText(t, sess, "flow_bind_project", map[string]any{
		"project": "jukebox", "remote": "git@github.com:serverkraken/elsewhere.git",
	})
	if res.IsError {
		t.Fatalf("bind with remote errored: %s", out)
	}
	got := rec.snapshot()
	if got.bindKind != "remote" || got.bindRemote != "github.com/serverkraken/elsewhere" {
		t.Fatalf("recorder = %+v, want a normalized remote binding", got)
	}
}

// TestLoopback_BindProject_ProjectOnlyStaysTheHistoricalCall is the backward
// compatibility contract from Spec §3: the pre-slice invocation — project alone,
// no path, no remote, no kind — must keep working and keep binding the flow-mcp
// process's working directory with auto-detected kind. The test process runs
// inside a git checkout with an origin, so auto-detect lands on remote.
func TestLoopback_BindProject_ProjectOnlyStaysTheHistoricalCall(t *testing.T) {
	sess, rec := authedBindingServer(t)

	res, out := callText(t, sess, "flow_bind_project", map[string]any{"project": "jukebox"})
	if res.IsError {
		t.Fatalf("the historical project-only call errored: %s", out)
	}
	got := rec.snapshot()
	if got.bindCalls != 1 || got.bindNodeID != "r1" {
		t.Fatalf("recorder = %+v, want exactly one bind on r1", got)
	}
	if got.bindKind != "remote" && got.bindKind != "path" {
		t.Fatalf("bindKind = %q, want an auto-detected remote or path binding", got.bindKind)
	}
	if got.bindKind == "path" && got.bindPath == "" {
		t.Fatal("an auto-detected path binding must carry the process cwd")
	}
	if got.createBounds != 0 {
		t.Fatalf("create-bound was called %d times", got.createBounds)
	}
}

func TestLoopback_BindProject_MissingProjectNamesCreateNode(t *testing.T) {
	sess, rec := authedBindingServer(t)

	res, out := callText(t, sess, "flow_bind_project", map[string]any{})
	if !res.IsError {
		t.Fatalf("bind without project: want IsError, got %q", out)
	}
	if !strings.Contains(out, "flow_create_node") {
		t.Fatalf("error = %q, want it to point at flow_create_node", out)
	}
	if got := rec.snapshot(); got.bindCalls != 0 {
		t.Fatalf("bindCalls = %d, want 0 (validation must short-circuit before any HTTP call)", got.bindCalls)
	}
}

func TestLoopback_BindProject_PathAndRemoteTogetherIsAnError(t *testing.T) {
	sess, rec := authedBindingServer(t)

	res, out := callText(t, sess, "flow_bind_project", map[string]any{
		"project": "jukebox", "path": t.TempDir(), "remote": "github.com/a/b",
	})
	if !res.IsError || !strings.Contains(out, "mutually exclusive") {
		t.Fatalf("path+remote = (IsError=%v, %q), want a mutually-exclusive error", res.IsError, out)
	}
	if got := rec.snapshot(); got.bindCalls != 0 {
		t.Fatalf("bindCalls = %d, want 0", got.bindCalls)
	}
}

func TestLoopback_BindProject_UnknownProjectErrors(t *testing.T) {
	sess, _ := authedBindingServer(t)
	res, out := callText(t, sess, "flow_bind_project", map[string]any{"project": "bogus", "kind": "path"})
	if !res.IsError || !strings.Contains(out, "unknown project") {
		t.Fatalf("unknown project = (IsError=%v, %q), want IsError + 'unknown project'", res.IsError, out)
	}
}
```

- [ ] **Step 3: Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run TestLoopback_BindProject_ 2>&1 | head -20` → FAIL. Erwartet: `unknown field "path" in bindNodeIn` bzw. `IsError` mit Schema-Validierungsmeldung, plus der Schema-Test scheitert an `create_name` (noch vorhanden).

- [ ] **Step 4: `bind.go` neu fassen** — in `cmd/flow-mcp/bind.go` den Block `validateBindRef` (Zeilen 15-28, Doc-Kommentar eingeschlossen) löschen, `decideBindKind` (32-49) **unverändert** stehen lassen, und `bindNodeCore` (51-111) durch diese drei Funktionen ersetzen:

```go
// bindNodeCore resolves the node reference and commits the already-resolved
// target as its binding. The create branch is gone: creating a node is
// flow_create_node's job (Spec §3), which keeps this function to one job.
func (h *handlers) bindNodeCore(ctx context.Context, c *apiclient.Client, nodeRef string, tgt bindTarget) (domain.Node, error) {
	ref := strings.TrimSpace(nodeRef)
	if ref == "" {
		return domain.Node{}, errGuard{errors.New(`"project" is required: pass an existing node's id, slug, or name (flow_list_projects shows the tree). To create a node use flow_create_node.`)}
	}
	node, err := h.lookupNode(ctx, ref)
	if err != nil {
		return domain.Node{}, err
	}
	if err := bindTargetTo(ctx, c, node.ID, tgt); err != nil {
		return domain.Node{}, err
	}
	return node, nil
}

// bindTargetTo commits a resolved target as a binding on nodeID.
func bindTargetTo(ctx context.Context, c *apiclient.Client, nodeID string, tgt bindTarget) error {
	if tgt.Kind == "remote" {
		_, err := c.BindRemote(ctx, nodeID, tgt.RemoteSlug)
		return err
	}
	_, err := c.BindPath(ctx, nodeID, tgt.MachineID, tgt.MachineLabel, tgt.Path)
	return err
}

// unbindTarget deletes the binding a target addresses. Neither unbind call takes
// a node id (internal/adapter/apiclient/projectbindings.go:82,96): a binding is
// identified by its target alone, which is why flow_node_binding rejects a
// `node` argument for unbind (Spec §3).
func unbindTarget(ctx context.Context, c *apiclient.Client, tgt bindTarget) error {
	if tgt.Kind == "remote" {
		return c.UnbindRemote(ctx, tgt.RemoteSlug)
	}
	return c.UnbindPath(ctx, tgt.MachineID, tgt.Path)
}
```

Die Imports von `bind.go` anpassen: `fmt` bleibt (`decideBindKind`), `errors` bleibt, `strings` bleibt, `context` bleibt, `apiclient` bleibt, `domain` bleibt; **`path/filepath` und `clientmachine` entfallen** (die sind nach `bindtarget.go` gewandert). `go build ./cmd/flow-mcp` zeigt ungenutzte Imports sofort an.

- [ ] **Step 5: `tools_bindings.go` mit dem umgezogenen `flow_bind_project` anlegen** — neue Datei `cmd/flow-mcp/tools_bindings.go`:

```go
package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// bindNodeIn binds a directory or a git remote to an EXISTING node. The old
// create_name/create_parent branch is gone — creating is flow_create_node's job.
// Omitting both path and remote keeps the historical behaviour: bind the
// flow-mcp process's working directory (Spec §3 flow_bind_project).
type bindNodeIn struct {
	Project string `json:"project,omitempty" jsonschema:"the existing node to bind to: id, slug, or name. Create a new node with flow_create_node."`
	Path    string `json:"path,omitempty" jsonschema:"an existing directory to bind; ~ and relative paths resolve against the flow-mcp process. Mutually exclusive with remote; omit both to bind the process's working directory."`
	Remote  string `json:"remote,omitempty" jsonschema:"a git clone URL or host/path slug to bind; no local checkout needed. Mutually exclusive with path."`
	Kind    string `json:"kind,omitempty" jsonschema:"binding kind override: 'remote' (git origin) or 'path' (this device); omit to auto-detect"`
}

// bindProject binds the resolved target to an existing node, then re-resolves so
// subsequent tools are scoped there.
func (h *handlers) bindProject(ctx context.Context, req *mcp.CallToolRequest, in bindNodeIn) (*mcp.CallToolResult, any, error) {
	env, err := liveBindEnv()
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	tgt, err := resolveBindTarget(bindTargetArgs{Path: in.Path, Remote: in.Remote, Kind: in.Kind}, env)
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	var bound domain.Node
	derr := h.do(ctx, req, func(c *apiclient.Client) error {
		node, e := h.bindNodeCore(ctx, c, in.Project, tgt)
		if e != nil {
			return e
		}
		bound = node
		h.refreshResolved(ctx, c)
		return nil
	})
	if derr != nil {
		return h.resultErr(derr), nil, nil
	}
	return textResult(fmt.Sprintf("Bound %s to project %s (%s) via %s binding. flow_project_context now resolves here.",
		bindTargetLabel(tgt), bound.Name, bound.Slug, tgt.Kind)), nil, nil
}
```

- [ ] **Step 6: `tools_project.go` entschlacken** — dort `bindNodeIn` (Zeilen 73-79) und `bindProject` (81-107) samt Doc-Kommentaren löschen. Danach werden `os`, `clientmachine` und `gitremote` in dieser Datei nicht mehr gebraucht — Imports entsprechend kürzen (`context`, `fmt`, `mcp`, `apiclient`, `domain` bleiben). `go build ./cmd/flow-mcp` verifiziert das.

- [ ] **Step 7: Description von `flow_bind_project` neu fassen** — in `cmd/flow-mcp/server.go` den Block bei `:107-110` ersetzen:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_bind_project",
		Description: "Bind a directory or a git remote to an EXISTING flow node so other tools auto-scope there. Pass project (id/slug/name) plus optionally path (a directory that must exist; ~ and relative paths resolve against the flow-mcp process) or remote (a clone URL or host/path slug, no local checkout needed); omit both to bind the process's working directory. Auto-detects a git-origin (remote) vs per-device (path) binding; override with kind. A target can only be bound to ONE node: binding a target that is already bound MOVES it to the new node — check with flow_node_binding action=resolve first. To create a node use flow_create_node; for unbind/list/resolve use flow_node_binding.",
	}, h.bindProject)
```

- [ ] **Step 8: Die zwei überholten Tests in `bind_test.go` löschen** — `TestValidateBindRef` (Zeilen 11-32) und `TestBindNodeCore_CreatePathRejectsMissingMachineBeforeAnyAPIWrite` (63-73) entfernen. `TestDecideBindKind` bleibt unverändert. Die Machine-ID-Vorprüfung wird jetzt von `TestResolveBindTarget_PathKindWithoutMachineIDIsAnError` (Task 1) abgedeckt — deshalb geht keine Zusicherung verloren. Danach sind `context`, `strings` und `clientmachine` in `bind_test.go` ungenutzt: Imports auf `testing` kürzen.

- [ ] **Step 9: `TestLoopback_BindProject` auf den neuen Vertrag ziehen** — in `cmd/flow-mcp/loopback_test.go` Abschnitt 3 (Zeilen 380-399) ersetzen. Statt `create_name` wird ein bestehender Node über `project` gebunden; die `createBoundCalled`-Zusicherung dreht sich um:

```go
	// 3. bind an existing node by slug; create-bound must NOT be used any more.
	bindCalled, createBoundCalled = false, false
	res, bindTxt := callText(t, sess, "flow_bind_project", map[string]any{
		"project": "alpha",
		"kind":    "remote", // deterministic: the test process runs inside a git checkout with an origin
	})
	if res.IsError {
		t.Fatalf("bind existing IsError: %s", bindTxt)
	}
	if !strings.Contains(bindTxt, "Alpha") {
		t.Fatalf("bind result = %q, want it to name 'Alpha'", bindTxt)
	}
	if !bindCalled {
		t.Fatal("PUT /api/v1/nodes/{id}/bindings was never called")
	}
	if createBoundCalled {
		t.Fatal("flow_bind_project must never create a node any more (create-bound was called)")
	}
```

Und Abschnitt 4 (Zeilen 401-408), der heute auf `create_name` prüft:

```go
	// 4. error case: no project reference at all.
	resErr, errTxt := callText(t, sess, "flow_bind_project", map[string]any{})
	if !resErr.IsError {
		t.Fatalf("no-ref bind: want IsError, got %q", errTxt)
	}
	if !strings.Contains(errTxt, "project") || !strings.Contains(errTxt, "flow_create_node") {
		t.Fatalf("no-ref error = %q, want mention of 'project' and 'flow_create_node'", errTxt)
	}
```

> `kind: "remote"` in Abschnitt 3 und 5 hängt daran, dass der Testprozess in einem git-Checkout mit Origin läuft — genau die Annahme, die der bestehende Kommentar an `:380-381` schon dokumentiert. Sie bleibt gültig, weil `resolveBindTarget` ohne `path` das Prozess-cwd nimmt.

- [ ] **Step 10: Tests laufen lassen, grün bestätigen** — `go test ./cmd/flow-mcp/ 2>&1 | tail -10` → ok. Die Tool-Zahl bleibt 25 (dieser Task registriert nichts Neues).

- [ ] **Step 11: Commit** — `git add cmd/flow-mcp/tools_bindings.go cmd/flow-mcp/tools_bindings_test.go cmd/flow-mcp/bind.go cmd/flow-mcp/bind_test.go cmd/flow-mcp/tools_project.go cmd/flow-mcp/server.go cmd/flow-mcp/loopback_test.go && git commit -m "feat(flow-mcp): flow_bind_project takes path/remote and no longer creates nodes"`

---

### Task 4: `flow_create_node` — der einzige Weg, einen Node anzulegen

Erstes neues Tool. **Tool-Zahl 25 → 26.**

**Files:**
- Create: `cmd/flow-mcp/tools_node_create.go`
- Create: `cmd/flow-mcp/tools_node_create_test.go`
- Modify: `cmd/flow-mcp/server.go` (Registrierung `flow_create_node`)
- Modify: `cmd/flow-mcp/loopback_test.go` (`:353-355` Tool-Zahl 25 → 26)

**Interfaces:**
- Consumes: `apiclient.CreateNodeFields{Name, Slug, Kind string; ParentID *string; Color, Glyph, Description, UpstreamGit string; CountsTowardTarget *bool}`; `c.CreateNode(ctx, in apiclient.CreateNodeFields) (domain.Node, error)`; `apiclient.CreateBoundNodeInput{Node apiclient.CreateNodeFields; Binding apiclient.BindingFields}`; `apiclient.CreateBoundNodeResult{Node domain.Node; Binding domain.ProjectBinding}`; `c.CreateBoundNode(ctx, in apiclient.CreateBoundNodeInput) (apiclient.CreateBoundNodeResult, error)`; `domain.ValidParentKind(child, parent domain.NodeKind) bool`; `resolveBindTarget`, `liveBindEnv`, `bindingFieldsFor`, `bindTargetLabel` (Task 1); `h.lookupNode`, `h.nodeList(ctx, refresh bool) ([]domain.Node, error)`, `h.refreshResolved`, `prefixGuard` (Task 1); `mcpLog() *slog.Logger`.
- Produces:
  - `type createNodeIn struct{ Name, Kind, Parent, Description, Color, Glyph, Upstream string; CountsTowardTarget *bool; BindPath string }`
  - `func validateCreateNode(in createNodeIn) error`
  - `func (h *handlers) createNode(ctx context.Context, req *mcp.CallToolRequest, in createNodeIn) (*mcp.CallToolResult, any, error)`
  - `var nodeKindsForCreate = []string{"engagement", "vorhaben", "repo"}`

- [ ] **Step 1: Bestand verifizieren** — diese drei Befehle laufen lassen:

```bash
rg -n "type CreateNodeFields" -A 12 internal/adapter/apiclient/nodes.go
rg -n "func ValidParentKind" -A 12 internal/domain/node.go
rg -n "type (CreateBoundNodeInput|CreateBoundNodeResult|BindingFields) struct" -A 8 internal/adapter/apiclient/projectbindings.go
```

Erwartet: `CreateNodeFields` **ohne `Icon`-Feld** (deshalb kennt `flow_create_node` kein `icon` — das setzt `flow_update_node` danach); `ValidParentKind` mit `default: return false`, also Engagement ausschließlich als Root; und die exakten Feldnamen `CreateBoundNodeInput{Node, Binding}`, `CreateBoundNodeResult{Node, Binding}`, `BindingFields{Kind, RemoteSlug, MachineID, MachineLabel, Path}` — genau die encodiert der Fake-Backend.

- [ ] **Step 2: Failing test schreiben** — neue Datei `cmd/flow-mcp/tools_node_create_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestValidateCreateNode(t *testing.T) {
	cases := []struct {
		name    string
		in      createNodeIn
		wantErr string // substring; "" = must pass
	}{
		{"engagement as root", createNodeIn{Name: "Alpha", Kind: "engagement"}, ""},
		{"engagement with parent", createNodeIn{Name: "Alpha", Kind: "engagement", Parent: "zeta"}, "always a root"},
		{"vorhaben with parent", createNodeIn{Name: "Rebuild", Kind: "vorhaben", Parent: "alpha"}, ""},
		{"vorhaben without parent", createNodeIn{Name: "Rebuild", Kind: "vorhaben"}, `needs a "parent"`},
		{"repo with parent", createNodeIn{Name: "Jukebox", Kind: "repo", Parent: "rebuild"}, ""},
		{"repo without parent", createNodeIn{Name: "Jukebox", Kind: "repo"}, `needs a "parent"`},
		{"branch is reserved", createNodeIn{Name: "wip", Kind: "branch", Parent: "jukebox"}, "reserved"},
		{"unknown kind", createNodeIn{Name: "X", Kind: "folder"}, "invalid kind"},
		{"missing kind", createNodeIn{Name: "X"}, "invalid kind"},
		{"missing name", createNodeIn{Kind: "engagement"}, "name is required"},
		{"blank name", createNodeIn{Name: "   ", Kind: "engagement"}, "name is required"},
		{"upstream on repo", createNodeIn{Name: "J", Kind: "repo", Parent: "r", Upstream: "git@github.com:a/b.git"}, ""},
		{"upstream on engagement", createNodeIn{Name: "A", Kind: "engagement", Upstream: "git@github.com:a/b.git"}, `only valid for kind "repo"`},
		{"upstream on vorhaben", createNodeIn{Name: "V", Kind: "vorhaben", Parent: "a", Upstream: "git@github.com:a/b.git"}, `only valid for kind "repo"`},
		{"bind_path on repo", createNodeIn{Name: "J", Kind: "repo", Parent: "r", BindPath: "/tmp"}, ""},
		{"bind_path on engagement", createNodeIn{Name: "A", Kind: "engagement", BindPath: "/tmp"}, `"bind_path" is only valid for kind "repo"`},
		{"bind_path on vorhaben", createNodeIn{Name: "V", Kind: "vorhaben", Parent: "a", BindPath: "/tmp"}, `"bind_path" is only valid for kind "repo"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateCreateNode(c.in)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("validateCreateNode(%#v) = %v, want nil", c.in, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateCreateNode(%#v) = nil, want an error containing %q", c.in, c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), c.wantErr)
			}
		})
	}
}

func TestValidateCreateNode_InvalidKindListsTheValidOnes(t *testing.T) {
	err := validateCreateNode(createNodeIn{Name: "X", Kind: "folder"})
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range nodeKindsForCreate {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must list the valid kind %q (never come back empty)", err.Error(), want)
		}
	}
	if strings.Contains(err.Error(), "branch") {
		t.Errorf("error %q must not offer the reserved kind branch", err.Error())
	}
}

// createRecorder captures the create bodies so the tests can prove which
// endpoint was used and that omitted fields stayed zero/nil.
type createRecorder struct {
	mu         sync.Mutex
	plain      []apiclient.CreateNodeFields
	bound      []apiclient.CreateBoundNodeInput
	plainCalls int
	boundCalls int
}

func (r *createRecorder) counts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.plainCalls, r.boundCalls
}

func (r *createRecorder) lastPlain(t *testing.T) apiclient.CreateNodeFields {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.plain) == 0 {
		t.Fatal("POST /api/v1/nodes was never called")
	}
	return r.plain[len(r.plain)-1]
}

func (r *createRecorder) lastBound(t *testing.T) apiclient.CreateBoundNodeInput {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.bound) == 0 {
		t.Fatal("POST /api/v1/nodes/create-bound was never called")
	}
	return r.bound[len(r.bound)-1]
}

// fakeCreateBackend serves the create endpoints. Fixture tree:
// e1/alpha (engagement) → v1/rebuild (vorhaben) → r1/jukebox (repo).
func fakeCreateBackend(t *testing.T, rec *createRecorder) *httptest.Server {
	t.Helper()
	e1, v1 := "e1", "v1"
	nodes := []domain.Node{
		{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement, Status: domain.NodeActive},
		{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben, ParentID: &e1, Status: domain.NodeActive},
		{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, ParentID: &v1, Status: domain.NodeActive},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(nodes)
	})
	mux.HandleFunc("POST /api/v1/nodes", func(w http.ResponseWriter, r *http.Request) {
		var f apiclient.CreateNodeFields
		_ = json.NewDecoder(r.Body).Decode(&f)
		rec.mu.Lock()
		rec.plain = append(rec.plain, f)
		rec.plainCalls++
		rec.mu.Unlock()
		slug := strings.ToLower(strings.ReplaceAll(f.Name, " ", "-"))
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(domain.Node{
			ID: "new1", Name: f.Name, Slug: slug, Kind: domain.NodeKind(f.Kind),
			ParentID: f.ParentID, Status: domain.NodeActive,
		})
	})
	mux.HandleFunc("POST /api/v1/nodes/create-bound", func(w http.ResponseWriter, r *http.Request) {
		var in apiclient.CreateBoundNodeInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		rec.mu.Lock()
		rec.bound = append(rec.bound, in)
		rec.boundCalls++
		rec.mu.Unlock()
		slug := strings.ToLower(strings.ReplaceAll(in.Node.Name, " ", "-"))
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.CreateBoundNodeResult{
			Node: domain.Node{ID: "new1", Name: in.Node.Name, Slug: slug,
				Kind: domain.NodeKind(in.Node.Kind), ParentID: in.Node.ParentID, Status: domain.NodeActive},
			Binding: domain.ProjectBinding{ID: "b1", NodeID: "new1"},
		})
	})
	return httptest.NewServer(mux)
}

func authedCreateServer(t *testing.T) (*mcp.ClientSession, *createRecorder) {
	t.Helper()
	rec := &createRecorder{}
	be := fakeCreateBackend(t, rec)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo})
	return connect(t, h.srv), rec
}

func TestLoopback_CreateNode_Advertised(t *testing.T) {
	sess, _ := authedCreateServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(tools.Tools, "flow_create_node") {
		t.Fatalf("flow_create_node not advertised; got %v", toolNames(tools.Tools))
	}
}

func TestLoopback_CreateNode_EngagementUsesPlainCreateWithNoParent(t *testing.T) {
	sess, rec := authedCreateServer(t)

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "Neues Engagement", "kind": "engagement", "description": "d",
	})
	if res.IsError {
		t.Fatalf("create engagement errored: %s", out)
	}
	plain, bound := rec.counts()
	if plain != 1 || bound != 0 {
		t.Fatalf("plainCalls=%d boundCalls=%d, want 1/0 (no bind_path → plain CreateNode)", plain, bound)
	}
	f := rec.lastPlain(t)
	if f.Kind != "engagement" || f.ParentID != nil {
		t.Fatalf("body = %+v, want engagement with nil parentId", f)
	}
	if f.Slug != "" {
		t.Fatalf("Slug = %q, want empty: the server derives the slug from the name", f.Slug)
	}
	if f.CountsTowardTarget != nil {
		t.Fatalf("CountsTowardTarget = %v, want nil so the server default survives", f.CountsTowardTarget)
	}
	if !strings.Contains(out, "Neues Engagement") || !strings.Contains(out, "new1") {
		t.Fatalf("result = %q, want it to name the node and its id", out)
	}
}

func TestLoopback_CreateNode_RepoUnderVorhabenResolvesParentToID(t *testing.T) {
	sess, rec := authedCreateServer(t)

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "Neues Repo", "kind": "repo", "parent": "rebuild",
		"upstream": "git@github.com:serverkraken/neu.git",
	})
	if res.IsError {
		t.Fatalf("create repo errored: %s", out)
	}
	f := rec.lastPlain(t)
	if f.ParentID == nil || *f.ParentID != "v1" {
		t.Fatalf("ParentID = %v, want the resolved id v1 (not the slug)", f.ParentID)
	}
	if f.UpstreamGit != "git@github.com:serverkraken/neu.git" {
		t.Fatalf("UpstreamGit = %q, want the passed clone URL verbatim", f.UpstreamGit)
	}
}

func TestLoopback_CreateNode_RepoUnderRepoIsRejectedBeforeAnyWrite(t *testing.T) {
	sess, rec := authedCreateServer(t)

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "Nested", "kind": "repo", "parent": "jukebox", // jukebox IS a repo
	})
	if !res.IsError {
		t.Fatalf("repo under repo: want IsError, got %q", out)
	}
	if !strings.Contains(out, "engagement or a vorhaben") {
		t.Fatalf("error = %q, want it to name the legal parent kinds", out)
	}
	if plain, bound := rec.counts(); plain != 0 || bound != 0 {
		t.Fatalf("plainCalls=%d boundCalls=%d, want 0/0 (the pre-check must precede the write)", plain, bound)
	}
}

func TestLoopback_CreateNode_UnknownParentSaysParent(t *testing.T) {
	sess, rec := authedCreateServer(t)
	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "X", "kind": "vorhaben", "parent": "bogus",
	})
	if !res.IsError {
		t.Fatalf("unknown parent: want IsError, got %q", out)
	}
	// The result text is the structured error JSON, so assert on substrings.
	if !strings.Contains(out, "parent:") {
		t.Errorf("error = %q, want the message prefixed with 'parent:' so the model knows WHICH argument was bad", out)
	}
	if !strings.Contains(out, "unknown project") {
		t.Errorf("error = %q, want lookupNode's actionable message to survive the prefix", out)
	}
	if plain, bound := rec.counts(); plain != 0 || bound != 0 {
		t.Errorf("plainCalls=%d boundCalls=%d, want 0/0", plain, bound)
	}
}

func TestLoopback_CreateNode_BindPathUsesOneAtomicCreateBoundCommand(t *testing.T) {
	sess, rec := authedCreateServer(t)
	dir := t.TempDir() // real directory, no git origin → path binding

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "Bound Repo", "kind": "repo", "parent": "rebuild", "bind_path": dir,
	})
	if res.IsError {
		t.Fatalf("create with bind_path errored: %s", out)
	}
	plain, bound := rec.counts()
	if plain != 0 || bound != 1 {
		t.Fatalf("plainCalls=%d boundCalls=%d, want 0/1 — node+binding must stay ONE atomic REST command (Finding 56, 2026-07-15)", plain, bound)
	}
	in := rec.lastBound(t)
	if in.Binding.Kind != "path" || in.Binding.Path != dir {
		t.Fatalf("binding = %+v, want a path binding on %q", in.Binding, dir)
	}
	if in.Binding.MachineID == "" {
		t.Fatalf("binding = %+v, want a machine id on a path binding", in.Binding)
	}
	if in.Node.ParentID == nil || *in.Node.ParentID != "v1" {
		t.Fatalf("node = %+v, want parent v1", in.Node)
	}
	if !strings.Contains(out, dir) {
		t.Fatalf("result = %q, want it to name the bound directory", out)
	}
}

func TestLoopback_CreateNode_BindPathMissingDirectoryFailsBeforeAnyWrite(t *testing.T) {
	sess, rec := authedCreateServer(t)

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "X", "kind": "repo", "parent": "rebuild", "bind_path": "/definitely/not/here",
	})
	if !res.IsError || !strings.Contains(out, "does not exist") {
		t.Fatalf("missing bind_path = (IsError=%v, %q), want a 'does not exist' error", res.IsError, out)
	}
	if plain, bound := rec.counts(); plain != 0 || bound != 0 {
		t.Fatalf("plainCalls=%d boundCalls=%d, want 0/0 — a bad path must never create a node", plain, bound)
	}
}

// TestLoopback_CreateNode_CountsTowardTargetIsThreeValued walks all three states
// Spec §3 requires: omitted → nil (server default survives), false → Privat,
// true → Work. The omitted case is asserted in the engagement test above.
func TestLoopback_CreateNode_CountsTowardTargetIsThreeValued(t *testing.T) {
	sess, rec := authedCreateServer(t)

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "Privat", "kind": "engagement", "counts_toward_target": false,
	})
	if res.IsError {
		t.Fatalf("create with false errored: %s", out)
	}
	f := rec.lastPlain(t)
	if f.CountsTowardTarget == nil || *f.CountsTowardTarget != false {
		t.Fatalf("CountsTowardTarget = %v, want an explicit false (Privat), not nil", f.CountsTowardTarget)
	}

	res, out = callText(t, sess, "flow_create_node", map[string]any{
		"name": "Arbeit", "kind": "engagement", "counts_toward_target": true,
	})
	if res.IsError {
		t.Fatalf("create with true errored: %s", out)
	}
	f = rec.lastPlain(t)
	if f.CountsTowardTarget == nil || *f.CountsTowardTarget != true {
		t.Fatalf("CountsTowardTarget = %v, want an explicit true (Work), not nil", f.CountsTowardTarget)
	}

	res, out = callText(t, sess, "flow_create_node", map[string]any{
		"name": "Erbt", "kind": "engagement",
	})
	if res.IsError {
		t.Fatalf("create without the flag errored: %s", out)
	}
	if f = rec.lastPlain(t); f.CountsTowardTarget != nil {
		t.Fatalf("CountsTowardTarget = %v, want nil so the server default survives", f.CountsTowardTarget)
	}
}

// TestLoopback_CreateNode_BindPathOnANonRepoIsRejectedBeforeAnyWrite: the atomic
// usecase refuses a bound node that is not a repo (create_bound_node.go:46), so
// the client says so precisely instead of letting a server 400 through.
func TestLoopback_CreateNode_BindPathOnANonRepoIsRejectedBeforeAnyWrite(t *testing.T) {
	sess, rec := authedCreateServer(t)

	res, out := callText(t, sess, "flow_create_node", map[string]any{
		"name": "Bound Vor", "kind": "vorhaben", "parent": "alpha", "bind_path": t.TempDir(),
	})
	if !res.IsError {
		t.Fatalf("bind_path on a vorhaben: want IsError, got %q", out)
	}
	if !strings.Contains(out, `"bind_path" is only valid for kind "repo"`) {
		t.Fatalf("error = %q, want the repo-only guard", out)
	}
	if !strings.Contains(out, "flow_node_binding") {
		t.Fatalf("error = %q, want it to point at the two-step alternative", out)
	}
	if plain, bound := rec.counts(); plain != 0 || bound != 0 {
		t.Fatalf("plainCalls=%d boundCalls=%d, want 0/0", plain, bound)
	}
}
```

- [ ] **Step 3: Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestValidateCreateNode|TestLoopback_CreateNode' 2>&1 | head -20` → FAIL mit `undefined: createNodeIn`, `undefined: validateCreateNode`, `undefined: nodeKindsForCreate`.

- [ ] **Step 4: `tools_node_create.go` schreiben** — neue Datei `cmd/flow-mcp/tools_node_create.go`:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// nodeKindsForCreate is the kind whitelist. domain.KindBranch is deliberately
// absent: it is reserved without behavior (internal/domain/node.go:24), and
// there is no domain.NodeKinds() helper to defer to.
var nodeKindsForCreate = []string{"engagement", "vorhaben", "repo"}

// createNodeIn is the only way to create a node over MCP. There is no slug
// parameter — the server derives the slug from the name, like `flow node create`;
// renaming is flow_update_node's job. There is no icon parameter either, because
// apiclient.CreateNodeFields carries none (internal/adapter/apiclient/nodes.go:27);
// set it with flow_update_node right after.
type createNodeIn struct {
	Name               string `json:"name" jsonschema:"the node's display name; the server derives the slug from it"`
	Kind               string `json:"kind" jsonschema:"engagement (always a root), vorhaben, or repo"`
	Parent             string `json:"parent,omitempty" jsonschema:"the parent node (id, slug, or name) — REQUIRED for vorhaben and repo, and rejected for engagement, which is always a root. flow_list_projects shows the tree."`
	Description        string `json:"description,omitempty" jsonschema:"one-line subtitle"`
	Color              string `json:"color,omitempty" jsonschema:"identity color name"`
	Glyph              string `json:"glyph,omitempty" jsonschema:"identity glyph"`
	Upstream           string `json:"upstream,omitempty" jsonschema:"git clone URL; valid for kind=repo only"`
	CountsTowardTarget *bool  `json:"counts_toward_target,omitempty" jsonschema:"true = Work (counts toward the daily target), false = Privat (tracked only); omit to inherit from the ancestor chain"`
	BindPath           string `json:"bind_path,omitempty" jsonschema:"optional, kind=repo ONLY: bind this directory to the new node in the same atomic command. The directory must exist; ~ and relative paths resolve against the flow-mcp process, so \".\" is its working directory. Omit for a node without any binding; to bind an engagement or vorhaben, create it first and use flow_node_binding."`
}

// validateCreateNode is the parameter-only pre-check. It turns a bad
// kind/parent/upstream combination into a precise message instead of a bare
// server 400; the server stays the authority (domain.ValidParentKind). The
// parent's KIND can only be checked after the lookup, so that rule lives in the
// handler.
func validateCreateNode(in createNodeIn) error {
	if strings.TrimSpace(in.Name) == "" {
		return errGuard{errors.New("name is required")}
	}
	kind := strings.TrimSpace(in.Kind)
	hasParent := strings.TrimSpace(in.Parent) != ""
	switch domain.NodeKind(kind) {
	case domain.KindEngagement:
		if hasParent {
			return errGuard{errors.New(`an engagement is always a root: drop "parent", or use kind "vorhaben" or "repo" to nest something under it`)}
		}
	case domain.KindVorhaben, domain.KindRepo:
		if !hasParent {
			return errGuard{fmt.Errorf(`kind %q needs a "parent": the id, slug, or name of an engagement or vorhaben (flow_list_projects shows the tree)`, kind)}
		}
	case domain.KindBranch:
		return errGuard{fmt.Errorf(`kind "branch" is reserved and has no behavior yet; use one of: %s`, strings.Join(nodeKindsForCreate, ", "))}
	default:
		return errGuard{fmt.Errorf("invalid kind %q; use one of: %s", kind, strings.Join(nodeKindsForCreate, ", "))}
	}
	if strings.TrimSpace(in.Upstream) != "" && domain.NodeKind(kind) != domain.KindRepo {
		return errGuard{fmt.Errorf(`"upstream" is only valid for kind "repo", not %q`, kind)}
	}
	// bind_path is repo-only, and this guard is NOT cosmetic: the atomic usecase
	// rejects anything else outright ("bound node must be a repo",
	// internal/usecase/create_bound_node.go:46). Note the asymmetry with the
	// separate bind endpoint, which also allows a childless vorhaben
	// (internal/usecase/bind_node.go:64-75) — so a vorhaben is bound in a second
	// step with flow_node_binding, not atomically here.
	if strings.TrimSpace(in.BindPath) != "" && domain.NodeKind(kind) != domain.KindRepo {
		return errGuard{fmt.Errorf(`"bind_path" is only valid for kind "repo", not %q; create the %s first, then bind it with flow_node_binding (action="bind")`, kind, kind)}
	}
	return nil
}

// createNode creates a node — with bind_path through CreateBoundNode so node and
// binding stay ONE atomic REST command (Finding 56 of the 2026-07-15 review),
// otherwise through plain CreateNode. Afterwards the node cache is invalidated so
// the new node is addressable on the very next call, and with a binding the
// cwd→node resolution is refreshed too, exactly as bindProject does.
func (h *handlers) createNode(ctx context.Context, req *mcp.CallToolRequest, in createNodeIn) (*mcp.CallToolResult, any, error) {
	if err := validateCreateNode(in); err != nil {
		return h.resultErr(err), nil, nil
	}
	binding := strings.TrimSpace(in.BindPath) != ""
	var tgt bindTarget
	if binding {
		env, err := liveBindEnv()
		if err != nil {
			return h.resultErr(err), nil, nil
		}
		// bind_path goes through the same target resolution as flow_node_binding,
		// so a bad path fails here — before any node is created.
		tgt, err = resolveBindTarget(bindTargetArgs{Path: in.BindPath}, env)
		if err != nil {
			return h.resultErr(err), nil, nil
		}
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		kind := domain.NodeKind(strings.TrimSpace(in.Kind))
		var parentID *string
		if ref := strings.TrimSpace(in.Parent); ref != "" {
			parent, perr := h.lookupNode(ctx, ref)
			if perr != nil {
				return prefixGuard("parent", perr)
			}
			if !domain.ValidParentKind(kind, parent.Kind) {
				return errGuard{fmt.Errorf("a %s cannot hang under %s %q (%s): a parent must be an engagement or a vorhaben",
					kind, parent.Kind, parent.Name, parent.Slug)}
			}
			parentID = &parent.ID
		}
		fields := apiclient.CreateNodeFields{
			Name: strings.TrimSpace(in.Name), Kind: string(kind), ParentID: parentID,
			Color: in.Color, Glyph: in.Glyph, Description: in.Description,
			UpstreamGit: strings.TrimSpace(in.Upstream), CountsTowardTarget: in.CountsTowardTarget,
		}
		var node domain.Node
		if binding {
			result, cerr := c.CreateBoundNode(ctx, apiclient.CreateBoundNodeInput{
				Node: fields, Binding: bindingFieldsFor(tgt),
			})
			if cerr != nil {
				return cerr
			}
			node = result.Node
		} else {
			created, cerr := c.CreateNode(ctx, fields)
			if cerr != nil {
				return cerr
			}
			node = created
		}
		if _, lerr := h.nodeList(ctx, true); lerr != nil {
			mcpLog().Warn("could not refresh the node cache after create", "err", lerr)
		}
		out = fmt.Sprintf("Created %s %q (%s), id %s.", node.Kind, node.Name, node.Slug, node.ID)
		if binding {
			h.refreshResolved(ctx, c)
			out += fmt.Sprintf(" Bound %s to it via %s binding.", bindTargetLabel(tgt), tgt.Kind)
		}
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
```

- [ ] **Step 5: Tool registrieren** — in `cmd/flow-mcp/server.go` direkt nach dem `flow_bind_project`-Block einfügen:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_create_node",
		Description: "Create a node in the flow hierarchy (engagement → vorhaben → repo). An engagement is ALWAYS a root and must not get a parent; a vorhaben or repo REQUIRES a parent that is an engagement or a vorhaben. upstream and bind_path are repo-only. Pass bind_path to bind a directory to the new repo in the same atomic command; omit it for a node without any binding (to bind an engagement or vorhaben, create it first and use flow_node_binding). The slug is derived from the name — rename with flow_update_node; icon is set afterwards with flow_update_node. Call flow_list_projects first to pick a valid parent.",
	}, h.createNode)
```

- [ ] **Step 6: Tool-Zahl auf 26 ziehen** — in `cmd/flow-mcp/loopback_test.go` bei `:353-355`:

```go
	if len(tools.Tools) != 26 {
		t.Fatalf("tool count = %d, want 26; got %v", len(tools.Tools), toolNames(tools.Tools))
	}
```

> Diese Assertion ist der Wiring-Wächter: wer ein Tool baut und die Registrierung vergisst, bekommt hier sofort rot.

- [ ] **Step 7: Tests laufen lassen, grün bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestValidateCreateNode|TestLoopback_CreateNode' -v 2>&1 | tail -30` → PASS. Dann das ganze Paket: `go test ./cmd/flow-mcp/ 2>&1 | tail -5` → ok.

- [ ] **Step 8: Commit** — `git add cmd/flow-mcp/tools_node_create.go cmd/flow-mcp/tools_node_create_test.go cmd/flow-mcp/server.go cmd/flow-mcp/loopback_test.go && git commit -m "feat(flow-mcp): add flow_create_node with kind/parent validation and optional bind_path"`

---

### Task 5: `flow_move_node` — `parent` oder `to_root`, genau eines

**Tool-Zahl 26 → 27.** JSON unterscheidet nicht zwischen „weggelassen" und „leerer String", deshalb ist das CLI-Muster `--parent ""` mit `fl.Changed` (`cmd/flow/node_subcommands.go:341`) über MCP nicht abbildbar — daher das separate `to_root` (Spec §3).

**Files:**
- Create: `cmd/flow-mcp/tools_node_lifecycle.go`
- Create: `cmd/flow-mcp/tools_node_lifecycle_test.go`
- Modify: `cmd/flow-mcp/server.go` (Registrierung `flow_move_node`)
- Modify: `cmd/flow-mcp/loopback_test.go` (`:353-355` Tool-Zahl 26 → 27)

**Interfaces:**
- Consumes: `c.MoveNode(ctx context.Context, id string, parentID *string) (domain.Node, error)`; `domain.ValidParentKind(child, parent domain.NodeKind) bool`; `domain.KindEngagement`; `h.lookupNode`, `h.nodeList`, `prefixGuard`, `mcpLog()`.
- Produces:
  - `type moveNodeIn struct{ Node, Parent string; ToRoot bool }`
  - `func validateMoveNode(in moveNodeIn) error`
  - `func (h *handlers) moveNode(ctx context.Context, req *mcp.CallToolRequest, in moveNodeIn) (*mcp.CallToolResult, any, error)`

- [ ] **Step 1: Bestand verifizieren** — diese drei Befehle laufen lassen:

```bash
rg -n "func \(c \*Client\) MoveNode" -A 6 internal/adapter/apiclient/nodes.go
rg -nF -e 'fl.Changed("parent")' -e '--parent' cmd/flow/node_subcommands.go
rg -n "func ValidParentKind" -A 12 internal/domain/node.go
rg -n "KindEngagement|NodeActive|NodePaused" internal/domain/node.go
```

Erwartet: `MoveNode(ctx, id string, parentID *string) (domain.Node, error)` (nil parentID = Root); das CLI-Muster bei `:341`, das über MCP nicht nachbaubar ist; `ValidParentKind` mit `default: return false` (nur ein Engagement darf Root sein — das begründet den Root-Guard im Handler); und die Kind-/Status-Konstanten, die Handler und Fixtures verwenden.

- [ ] **Step 2: Failing test schreiben** — neue Datei `cmd/flow-mcp/tools_node_lifecycle_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

func TestValidateMoveNode(t *testing.T) {
	cases := []struct {
		name    string
		in      moveNodeIn
		wantErr string
	}{
		{"parent only", moveNodeIn{Node: "jukebox", Parent: "alpha"}, ""},
		{"to_root only", moveNodeIn{Node: "alpha", ToRoot: true}, ""},
		{"both", moveNodeIn{Node: "jukebox", Parent: "alpha", ToRoot: true}, "exactly one destination"},
		{"neither", moveNodeIn{Node: "jukebox"}, "exactly one destination"},
		{"blank parent counts as absent", moveNodeIn{Node: "jukebox", Parent: "   "}, "exactly one destination"},
		{"blank parent plus to_root is fine", moveNodeIn{Node: "alpha", Parent: "  ", ToRoot: true}, ""},
		{"missing node", moveNodeIn{Parent: "alpha"}, "node is required"},
		{"blank node", moveNodeIn{Node: "  ", ToRoot: true}, "node is required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateMoveNode(c.in)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("validateMoveNode(%#v) = %v, want nil", c.in, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("validateMoveNode(%#v) = %v, want an error containing %q", c.in, err, c.wantErr)
			}
		})
	}
}

// lifecycleRecorder captures the move and delete calls.
type lifecycleRecorder struct {
	mu          sync.Mutex
	moveNodeID  string
	moveParent  *string
	moveCalls   int
	deleteID    string
	deleteCalls int
}

func (r *lifecycleRecorder) snapshot() lifecycleRecorder {
	r.mu.Lock()
	defer r.mu.Unlock()
	return lifecycleRecorder{
		moveNodeID: r.moveNodeID, moveParent: r.moveParent, moveCalls: r.moveCalls,
		deleteID: r.deleteID, deleteCalls: r.deleteCalls,
	}
}

// lifecycleNodes is the fixture tree used by the move and delete tests:
// e1/alpha (engagement) → v1/rebuild (vorhaben) → r1/jukebox (repo);
// e2/zeta is a second engagement, and l1/leaf is a childless repo under e2.
func lifecycleNodes() []domain.Node {
	e1, v1, e2 := "e1", "v1", "e2"
	return []domain.Node{
		{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement, Status: domain.NodeActive},
		{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben, ParentID: &e1, Status: domain.NodeActive},
		{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, ParentID: &v1, Status: domain.NodeActive},
		{ID: "e2", Name: "Zeta", Slug: "zeta", Kind: domain.KindEngagement, Status: domain.NodeActive},
		{ID: "l1", Name: "Leaf", Slug: "leaf", Kind: domain.KindRepo, ParentID: &e2, Status: domain.NodeActive},
	}
}

// fakeLifecycleBackend serves move, delete and every endpoint the delete dry run
// reads. Deleting v1 (which has the child r1) answers 409 like the real server.
func fakeLifecycleBackend(t *testing.T, rec *lifecycleRecorder) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(lifecycleNodes())
	})
	mux.HandleFunc("POST /api/v1/nodes/{id}/move", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ParentID *string `json:"parentId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		rec.mu.Lock()
		rec.moveNodeID, rec.moveParent = r.PathValue("id"), body.ParentID
		rec.moveCalls++
		rec.mu.Unlock()
		for _, n := range lifecycleNodes() {
			if n.ID == r.PathValue("id") {
				n.ParentID = body.ParentID
				_ = json.NewEncoder(w).Encode(n)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rec.mu.Lock()
		rec.deleteID = id
		rec.deleteCalls++
		rec.mu.Unlock()
		if id == "v1" { // has child r1 — mirrors worktime.go:281
			http.Error(w, "node has children; move or remove them first", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		for _, n := range lifecycleNodes() {
			if n.ID == r.PathValue("id") {
				if n.ID == "l1" {
					n.LogoRef = "sha256:abc" // the childless repo carries a logo
				}
				_ = json.NewEncoder(w).Encode(n)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/stats", func(w http.ResponseWriter, r *http.Request) {
		out := apiclient.NodeRollup{}
		if r.PathValue("id") == "l1" {
			out = apiclient.NodeRollup{TotalMin: 750, WeekMin: 120, MonthMin: 300}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/artifacts", func(w http.ResponseWriter, r *http.Request) {
		// Mirrors usecase.ListArtifacts: the node's OWN artifacts plus an
		// ancestor's plus a free (node-less) one. Only the own ones are deleted.
		_ = json.NewEncoder(w).Encode([]domain.Artifact{
			{ID: "a1", NodeID: r.PathValue("id"), Slug: "own-1", Name: "own-1.png", Mime: "image/png"},
			{ID: "a2", NodeID: r.PathValue("id"), Slug: "own-2", Name: "own-2.png", Mime: "image/png"},
			{ID: "a3", NodeID: "e2", Slug: "ancestor", Name: "ancestor.png", Mime: "image/png"},
			{ID: "a4", NodeID: "", Slug: "free", Name: "free.png", Mime: "image/png"},
		})
	})
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		pid := r.URL.Query().Get("projectId")
		var out []domain.Document
		if pid == "v1" { // the blocked node also owns a project document
			nid := "v1"
			out = append(out, domain.Document{ID: "d1", NodeID: &nid, Type: domain.DocProject, Path: "projekt/rebuild", Title: "Rebuild"})
		}
		if pid == "l1" { // a non-project document: silently detached, not blocking
			nid := "l1"
			out = append(out, domain.Document{ID: "d2", NodeID: &nid, Type: domain.DocMemory, Path: "notes/x", Title: "X"})
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux)
}

func authedLifecycleServer(t *testing.T) (*mcp.ClientSession, *lifecycleRecorder) {
	t.Helper()
	rec := &lifecycleRecorder{}
	be := fakeLifecycleBackend(t, rec)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo})
	return connect(t, h.srv), rec
}

func TestLoopback_MoveNode_Advertised(t *testing.T) {
	sess, _ := authedLifecycleServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(tools.Tools, "flow_move_node") {
		t.Fatalf("flow_move_node not advertised; got %v", toolNames(tools.Tools))
	}
}

func TestLoopback_MoveNode_ParentResolvesToIDAndSendsOneCall(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "jukebox", "parent": "zeta"})
	if res.IsError {
		t.Fatalf("move errored: %s", out)
	}
	got := rec.snapshot()
	if got.moveCalls != 1 || got.moveNodeID != "r1" {
		t.Fatalf("recorder = %+v, want exactly one move of r1", got)
	}
	if got.moveParent == nil || *got.moveParent != "e2" {
		t.Fatalf("parentId = %v, want the resolved id e2", got.moveParent)
	}
	if !strings.Contains(out, "Zeta") {
		t.Fatalf("result = %q, want it to name the destination", out)
	}
}

func TestLoopback_MoveNode_ToRootSendsNullParent(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "alpha", "to_root": true})
	if res.IsError {
		t.Fatalf("move to root errored: %s", out)
	}
	got := rec.snapshot()
	if got.moveCalls != 1 || got.moveNodeID != "e1" {
		t.Fatalf("recorder = %+v, want one move of e1", got)
	}
	if got.moveParent != nil {
		t.Fatalf("parentId = %v, want nil (to_root)", got.moveParent)
	}
	if !strings.Contains(out, "root") {
		t.Fatalf("result = %q, want it to say root", out)
	}
}

func TestLoopback_MoveNode_BothDestinationsIsAnErrorBeforeAnyCall(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_move_node", map[string]any{
		"node": "jukebox", "parent": "zeta", "to_root": true,
	})
	if !res.IsError || !strings.Contains(out, "exactly one destination") {
		t.Fatalf("both destinations = (IsError=%v, %q), want an exactly-one error", res.IsError, out)
	}
	if got := rec.snapshot(); got.moveCalls != 0 {
		t.Fatalf("moveCalls = %d, want 0", got.moveCalls)
	}
}

func TestLoopback_MoveNode_NoDestinationIsAnError(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "jukebox"})
	if !res.IsError || !strings.Contains(out, "exactly one destination") {
		t.Fatalf("no destination = (IsError=%v, %q), want an exactly-one error", res.IsError, out)
	}
	if got := rec.snapshot(); got.moveCalls != 0 {
		t.Fatalf("moveCalls = %d, want 0", got.moveCalls)
	}
}

func TestLoopback_MoveNode_RepoUnderRepoIsRejectedBeforeAnyCall(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "jukebox", "parent": "leaf"})
	if !res.IsError || !strings.Contains(out, "engagement or a vorhaben") {
		t.Fatalf("repo under repo = (IsError=%v, %q), want the kind guard", res.IsError, out)
	}
	if got := rec.snapshot(); got.moveCalls != 0 {
		t.Fatalf("moveCalls = %d, want 0", got.moveCalls)
	}
}

func TestLoopback_MoveNode_ToRootOnANonEngagementIsRejected(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "jukebox", "to_root": true})
	if !res.IsError || !strings.Contains(out, "only an engagement") {
		t.Fatalf("repo to root = (IsError=%v, %q), want the root guard (ValidParentKind allows only an engagement as root)", res.IsError, out)
	}
	if got := rec.snapshot(); got.moveCalls != 0 {
		t.Fatalf("moveCalls = %d, want 0", got.moveCalls)
	}
}

func TestLoopback_MoveNode_UnknownParentSaysParent(t *testing.T) {
	sess, _ := authedLifecycleServer(t)
	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "jukebox", "parent": "bogus"})
	if !res.IsError || !strings.Contains(out, "parent:") {
		t.Fatalf("unknown parent = (IsError=%v, %q), want a 'parent:'-prefixed message", res.IsError, out)
	}
}

// TestLoopback_MoveNode_ServerCycleConflictReachesTheModel proves the division of
// labour: the client pre-checks KINDS, the server owns cycle detection, and its
// 409 must arrive readable rather than as a bare status. A cycle cannot be
// provoked through the kind rules alone, so the fixture answers the move route
// the way nodemove.go:27 does.
func TestLoopback_MoveNode_ServerCycleConflictReachesTheModel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(lifecycleNodes())
	})
	mux.HandleFunc("POST /api/v1/nodes/{id}/move", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "move would create a cycle", http.StatusConflict)
	})
	be := httptest.NewServer(mux)
	t.Cleanup(be.Close)

	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo})
	sess := connect(t, h.srv)

	// vorhaben under engagement is kind-legal, so the client lets it through and
	// only the server's 409 stops it.
	res, out := callText(t, sess, "flow_move_node", map[string]any{"node": "rebuild", "parent": "alpha"})
	if !res.IsError {
		t.Fatalf("server 409: want IsError, got %q", out)
	}
	if !strings.Contains(out, "cycle") {
		t.Fatalf("error = %q, want the server's 'move would create a cycle' message verbatim", out)
	}
}
```

- [ ] **Step 3: Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestValidateMoveNode|TestLoopback_MoveNode' 2>&1 | head -20` → FAIL mit `undefined: moveNodeIn`, `undefined: validateMoveNode`.

- [ ] **Step 4: `tools_node_lifecycle.go` mit dem Move-Teil anlegen** — neue Datei `cmd/flow-mcp/tools_node_lifecycle.go`:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// moveNodeIn reparents a node. Exactly one of parent / to_root is required:
// JSON cannot distinguish an omitted string from an empty one, so the CLI's
// `--parent ""` + fl.Changed pattern (cmd/flow/node_subcommands.go:341) is not
// expressible over MCP — hence the separate boolean (Spec §3).
type moveNodeIn struct {
	Node   string `json:"node" jsonschema:"the node to reparent (id, slug, or name)"`
	Parent string `json:"parent,omitempty" jsonschema:"the new parent (id, slug, or name) — an engagement or vorhaben. Pass exactly one of parent or to_root."`
	ToRoot bool   `json:"to_root,omitempty" jsonschema:"true makes the node a root; only an engagement may be a root. Pass exactly one of parent or to_root."`
}

// validateMoveNode enforces exactly one destination.
func validateMoveNode(in moveNodeIn) error {
	if strings.TrimSpace(in.Node) == "" {
		return errGuard{errors.New("node is required: the id, slug, or name of the node to reparent")}
	}
	hasParent := strings.TrimSpace(in.Parent) != ""
	if hasParent == in.ToRoot {
		return errGuard{errors.New(`pass exactly one destination: "parent" (an engagement or vorhaben id/slug/name) or to_root=true (make it a root engagement)`)}
	}
	return nil
}

// moveNode reparents a node. The kind rules are pre-checked client-side for a
// precise message; cycle-freeness is the server's job (it answers 409, which
// h.resultErr surfaces verbatim).
func (h *handlers) moveNode(ctx context.Context, req *mcp.CallToolRequest, in moveNodeIn) (*mcp.CallToolResult, any, error) {
	if err := validateMoveNode(in); err != nil {
		return h.resultErr(err), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		node, err := h.lookupNode(ctx, strings.TrimSpace(in.Node))
		if err != nil {
			return err
		}
		var parentID *string
		dest := "root"
		if ref := strings.TrimSpace(in.Parent); ref != "" {
			parent, perr := h.lookupNode(ctx, ref)
			if perr != nil {
				return prefixGuard("parent", perr)
			}
			if !domain.ValidParentKind(node.Kind, parent.Kind) {
				return errGuard{fmt.Errorf("a %s cannot hang under %s %q (%s): a parent must be an engagement or a vorhaben",
					node.Kind, parent.Kind, parent.Name, parent.Slug)}
			}
			parentID = &parent.ID
			dest = fmt.Sprintf("%s %q (%s)", parent.Kind, parent.Name, parent.Slug)
		} else if node.Kind != domain.KindEngagement {
			// ValidParentKind's default case makes an engagement the only legal
			// root (internal/domain/node.go:107).
			return errGuard{fmt.Errorf("only an engagement may be a root; %s %q (%s) needs a parent",
				node.Kind, node.Name, node.Slug)}
		}
		moved, err := c.MoveNode(ctx, node.ID, parentID)
		if err != nil {
			return err
		}
		if _, lerr := h.nodeList(ctx, true); lerr != nil {
			mcpLog().Warn("could not refresh the node cache after move", "err", lerr)
		}
		out = fmt.Sprintf("Moved %s %q (%s) to %s.", moved.Kind, moved.Name, moved.Slug, dest)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
```

- [ ] **Step 5: Tool registrieren** — in `cmd/flow-mcp/server.go` nach dem `flow_create_node`-Block:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_move_node",
		Description: "Reparent a node. Pass exactly ONE destination: parent (the new parent's id/slug/name — an engagement or vorhaben) or to_root=true (make it a root, which only an engagement may be). The server rejects cycles and slug collisions.",
	}, h.moveNode)
```

- [ ] **Step 6: Tool-Zahl auf 27 ziehen** — in `cmd/flow-mcp/loopback_test.go` bei `:353-355` die `26` durch `27` ersetzen (beide Vorkommen: Bedingung und Meldung).

- [ ] **Step 7: Tests laufen lassen, grün bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestValidateMoveNode|TestLoopback_MoveNode' -v 2>&1 | tail -25` → PASS. Dann `go test ./cmd/flow-mcp/ 2>&1 | tail -5` → ok.

- [ ] **Step 8: Commit** — `git add cmd/flow-mcp/tools_node_lifecycle.go cmd/flow-mcp/tools_node_lifecycle_test.go cmd/flow-mcp/server.go cmd/flow-mcp/loopback_test.go && git commit -m "feat(flow-mcp): add flow_move_node with an explicit to_root destination"`

---

### Task 6: `flow_delete_node` — Trockenlauf, dann Löschen mit `confirm`

**Tool-Zahl 27 → 28.** Ohne `confirm` löscht das Tool nichts, sondern berichtet die Folgen. Zwei Fallstricke muss der Bericht korrekt behandeln (Spec §3): `ListArtifacts` liefert Node + Ahnenkette + owner-weite Free-Library (`internal/usecase/list_artifacts.go:21-51`), also wird auf `NodeID == node.ID` gefiltert; `NodeStats` liefert nur Minuten und rollt den Teilbaum auf, also nennt der Bericht Minuten statt Sessions.

**Files:**
- Modify: `cmd/flow-mcp/tools_node_lifecycle.go` (anfügen: `deleteNodeIn`, `deleteImpact`, `deleteImpactOf`, `deleteNode`)
- Modify: `cmd/flow-mcp/tools_node_lifecycle_test.go` (anfügen)
- Modify: `cmd/flow-mcp/format_nodes.go` (anfügen: `formatDeleteImpact`, `joinCapped`, `maxDeleteImpactItems`; Minuten kommen aus `internal/timefmt`, siehe Task 0)
- Modify: `cmd/flow-mcp/format_nodes_test.go` (anfügen)
- Modify: `cmd/flow-mcp/server.go` (Registrierung `flow_delete_node`)
- Modify: `cmd/flow-mcp/loopback_test.go` (`:353-355` Tool-Zahl 27 → 28)

**Interfaces:**
- Consumes: `timefmt.FormatMin(min int) string` aus `github.com/serverkraken/flow/internal/timefmt` (Task 0) — es gibt **kein** lokales `formatMinutes`; `c.GetNode(ctx, id string) (domain.Node, error)`; `c.DeleteNode(ctx, id string) error`; `c.ListArtifacts(ctx, nodeID string) ([]domain.Artifact, error)` mit `domain.Artifact{NodeID, Slug, Name string}`; `c.ListDocumentsScoped(ctx, nodeID *string, tags ...string) ([]domain.Document, error)` mit `domain.Document{Type domain.DocumentType, Path string}`; `domain.DocProject`; `c.NodeStats(ctx, id string) (apiclient.NodeRollup, error)` mit `NodeRollup{TotalMin, WeekMin, MonthMin int}`; `domain.Node.LogoRef`, `domain.Node.ParentID`; `h.lookupNode`, `h.nodeList`, `mcpLog()`.
- Produces:
  - `type deleteNodeIn struct{ Node string; Confirm bool }`
  - `type deleteImpact struct{ Node domain.Node; Children []domain.Node; ProjectDocs []domain.Document; OwnArtifacts int; HasLogo bool; Rollup apiclient.NodeRollup }`
  - `func (d deleteImpact) blocked() bool`
  - `func (h *handlers) deleteImpactOf(ctx context.Context, c *apiclient.Client, nodeID string) (deleteImpact, error)`
  - `func (h *handlers) deleteNode(ctx context.Context, req *mcp.CallToolRequest, in deleteNodeIn) (*mcp.CallToolResult, any, error)`
  - `func formatDeleteImpact(d deleteImpact) string`
  - `const maxDeleteImpactItems = 10`
  - `func joinCapped(items []string, max int) string`

- [ ] **Step 1: Bestand verifizieren** — diese drei Befehle:

```bash
rg -n "func \(uc ListArtifacts\) Execute" -A 32 internal/usecase/list_artifacts.go
rg -n "type NodeRollup" -A 6 internal/adapter/apiclient/nodes.go
rg -n "DocProject" internal/domain/document.go
rg -n "artifacts|ListArtifacts" internal/adapter/apiclient/artifacts.go
rg -n "func \(c \*Client\) (GetNode|DeleteNode|NodeStats|ListDocumentsScoped)" -A 5 internal/adapter/apiclient/
rg -n "type Artifact struct" -A 12 internal/domain/artifact.go
rg -n "LogoRef" internal/domain/node.go
rg -n "flow_move_doc|flow_move_node" cmd/flow-mcp/server.go
```

Erwartet: `ListArtifacts` hängt `ListFree(owner)` an (Free-Artefakte haben `NodeID == ""`), also fängt der Filter auf `NodeID == node.ID` sowohl Ahnen- als auch Free-Artefakte ab; `NodeRollup` hat **nur** `TotalMin`/`WeekMin`/`MonthMin`; `domain.DocProject` existiert; die Artefakt-Route ist `GET /api/v1/nodes/{id}/artifacts` (verifiziert: `apiclient/artifacts.go:29` und `httpserver/server.go:213`) — genau so heißt sie im Test-Fake von Task 5; `GetNode(ctx, id) (domain.Node, error)`, `DeleteNode(ctx, id) error`, `NodeStats(ctx, id) (NodeRollup, error)`, `ListDocumentsScoped(ctx, nodeID *string, tags ...string) ([]domain.Document, error)`; die `domain.Artifact`-Felder, die der Fake encodiert (`ID`, `NodeID`, `Slug`, `Name`, `Mime`); und `LogoRef` auf `domain.Node` (`node.go:42`) als einzige Quelle für „hat ein Logo". Außerdem: der Löschbericht verweist Nutzer auf `flow_move_doc` und `flow_move_node` — beide müssen als registrierte Tool-Namen in `server.go` auftauchen (`flow_move_doc` ist Bestand bei `:95`, `flow_move_node` hat Task 5 registriert), sonst nennt die Meldung ein Tool, das es nicht gibt. Weicht etwas ab, gewinnt der Bestand.

- [ ] **Step 2: Failing Renderer-Test schreiben** — an `cmd/flow-mcp/format_nodes_test.go` anfügen:

```go
func TestFormatDeleteImpact_DeletableNodeReportsMinutesAndOwnArtifactsOnly(t *testing.T) {
	out := formatDeleteImpact(deleteImpact{
		Node:         domain.Node{ID: "l1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, LogoRef: "sha256:abc"},
		OwnArtifacts: 3,
		HasLogo:      true,
		Rollup:       apiclient.NodeRollup{TotalMin: 750},
	})
	for _, want := range []string{`Would delete repo "Jukebox" (jukebox)`, "12h 30m", "3 artifact", "1 logo",
		"No children, no project documents", "confirm=true"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "session") {
		t.Errorf("report must speak of minutes, not sessions (NodeStats has no session count):\n%s", out)
	}
}

func TestFormatDeleteImpact_NoWorktimeAndNoLogoReadCleanly(t *testing.T) {
	out := formatDeleteImpact(deleteImpact{
		Node: domain.Node{ID: "l1", Name: "Leer", Slug: "leer", Kind: domain.KindRepo},
	})
	if !strings.Contains(out, "No booked worktime") {
		t.Errorf("report must state the empty worktime case:\n%s", out)
	}
	if !strings.Contains(out, "no logo") {
		t.Errorf("report must state the no-logo case instead of '0 logo':\n%s", out)
	}
	if strings.Contains(out, "0h 00m of booked") {
		t.Errorf("report must not print a zero duration:\n%s", out)
	}
}

func TestFormatDeleteImpact_BlockedByChildrenAndProjectDocs(t *testing.T) {
	out := formatDeleteImpact(deleteImpact{
		Node:        domain.Node{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben},
		Children:    []domain.Node{{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo}},
		ProjectDocs: []domain.Document{{ID: "d1", Path: "projekt/rebuild", Type: domain.DocProject}},
	})
	for _, want := range []string{"Cannot delete", "rebuild", "jukebox", "flow_move_node",
		"projekt/rebuild", "flow_move_doc"} {
		if !strings.Contains(out, want) {
			t.Errorf("blocked report missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "confirm=true") {
		t.Errorf("a blocked report must not invite confirm=true:\n%s", out)
	}
}

// TestFormatDeleteImpact_ManyChildrenAreCappedNotDumped is the "long" state for a
// text surface: the count stays exact, the enumeration stays bounded.
func TestFormatDeleteImpact_ManyChildrenAreCappedNotDumped(t *testing.T) {
	children := make([]domain.Node, 42)
	for i := range children {
		children[i] = domain.Node{ID: fmt.Sprintf("c%d", i), Slug: fmt.Sprintf("child-%02d", i), Kind: domain.KindRepo}
	}
	out := formatDeleteImpact(deleteImpact{
		Node:     domain.Node{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben},
		Children: children,
	})
	if !strings.Contains(out, "42 child node(s)") {
		t.Errorf("the exact count must survive the cap:\n%s", out)
	}
	if !strings.Contains(out, "and 32 more") {
		t.Errorf("the enumeration must be capped at %d with a remainder:\n%s", maxDeleteImpactItems, out)
	}
	if strings.Contains(out, "child-41") {
		t.Errorf("the enumeration must not dump every child:\n%s", out)
	}
	if strings.Count(out, "\n") > 3 {
		t.Errorf("a blocked report must stay a few lines, got:\n%s", out)
	}
}

func TestJoinCapped(t *testing.T) {
	if got := joinCapped([]string{"a", "b"}, 10); got != "a, b" {
		t.Errorf("joinCapped under the cap = %q, want a plain join", got)
	}
	if got := joinCapped([]string{"a", "b", "c"}, 2); got != "a, b … and 1 more" {
		t.Errorf("joinCapped over the cap = %q", got)
	}
	if got := joinCapped(nil, 3); got != "" {
		t.Errorf("joinCapped(nil) = %q, want empty", got)
	}
}

func TestDeleteImpactBlocked(t *testing.T) {
	if (deleteImpact{}).blocked() {
		t.Error("an empty impact must not be blocked")
	}
	if !(deleteImpact{Children: []domain.Node{{ID: "x"}}}).blocked() {
		t.Error("children must block")
	}
	if !(deleteImpact{ProjectDocs: []domain.Document{{ID: "d"}}}).blocked() {
		t.Error("project documents must block")
	}
}
```

> `format_nodes_test.go` braucht dafür zusätzlich die Imports `"fmt"` und `"github.com/serverkraken/flow/internal/adapter/apiclient"`.

- [ ] **Step 3: Renderer-Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestFormatDeleteImpact|TestJoinCapped|TestDeleteImpactBlocked' 2>&1 | head -20` → FAIL mit `undefined: formatDeleteImpact`, `undefined: deleteImpact`, `undefined: joinCapped`, `undefined: maxDeleteImpactItems`. Die Minuten-Formatierung wird hier **nicht** getestet — sie ist `timefmt.FormatMin` und hat ihre Tests in `internal/timefmt/timefmt_test.go` (Task 0).

- [ ] **Step 4: Renderer an `format_nodes.go` anfügen** — am Ende von `cmd/flow-mcp/format_nodes.go`. Der Import `github.com/serverkraken/flow/internal/timefmt` kommt hier hinzu (Task 0 hat das Package angelegt); `apiclient` kommt hinzu, sobald `deleteImpact` in `tools_node_lifecycle.go` definiert ist — der Renderer selbst braucht ihn nicht:

```go
// maxDeleteImpactItems caps how many child slugs / document paths the report
// enumerates. A node can legitimately have hundreds of children, and a single
// unbounded comma-joined line is the text-output equivalent of an unbreakable
// string: it drowns the actionable part of the message and burns model context.
const maxDeleteImpactItems = 10

// joinCapped joins at most max items and appends "… and N more" for the rest, so
// the count in the surrounding sentence stays authoritative.
func joinCapped(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s … and %d more", strings.Join(items[:max], ", "), len(items)-max)
}

// formatDeleteImpact renders the dry run. A node with children or project
// documents is reported as NOT deletable, because the database refuses it anyway
// (nodes.parent_id is ON DELETE RESTRICT, migration 0016; project documents are
// checked explicitly in internal/adapter/pgstore/nodes.go:144-151). Everything
// else is the silent damage made visible: sessions and non-project documents are
// set to NULL (migration 0012), bindings, artifacts and the logo CASCADE.
func formatDeleteImpact(d deleteImpact) string {
	var b strings.Builder
	if d.blocked() {
		fmt.Fprintf(&b, "Cannot delete %s %q (%s) — the server would refuse it:\n", d.Node.Kind, d.Node.Name, d.Node.Slug)
		if len(d.Children) > 0 {
			slugs := make([]string, len(d.Children))
			for i, c := range d.Children {
				slugs[i] = c.Slug
			}
			fmt.Fprintf(&b, "  %d child node(s): %s — move them with flow_move_node first.\n",
				len(d.Children), joinCapped(slugs, maxDeleteImpactItems))
		}
		if len(d.ProjectDocs) > 0 {
			paths := make([]string, len(d.ProjectDocs))
			for i, doc := range d.ProjectDocs {
				paths[i] = doc.Path
			}
			fmt.Fprintf(&b, "  %d project document(s): %s — move or reclassify them with flow_move_doc first.\n",
				len(d.ProjectDocs), joinCapped(paths, maxDeleteImpactItems))
		}
		return strings.TrimRight(b.String(), "\n")
	}
	fmt.Fprintf(&b, "Would delete %s %q (%s).\n", d.Node.Kind, d.Node.Name, d.Node.Slug)
	if d.Rollup.TotalMin > 0 {
		// NodeStats rolls up the SUBTREE, but a node with children is never
		// deletable — so in exactly the deletable case the number is exact.
		fmt.Fprintf(&b, "  %s of booked worktime would lose its node.\n", timefmt.FormatMin(d.Rollup.TotalMin))
	} else {
		b.WriteString("  No booked worktime.\n")
	}
	logo := "no logo"
	if d.HasLogo {
		logo = "1 logo"
	}
	fmt.Fprintf(&b, "  %d artifact(s) and %s would be deleted, along with every binding of this node.\n",
		d.OwnArtifacts, logo)
	b.WriteString("  Other documents in its scope would lose their node but survive.\n")
	b.WriteString("  No children, no project documents — delete is possible.\n")
	b.WriteString("  Pass confirm=true to proceed.")
	return b.String()
}
```

- [ ] **Step 5: Failing Handler-Test schreiben** — an `cmd/flow-mcp/tools_node_lifecycle_test.go` anfügen:

```go
func TestLoopback_DeleteNode_Advertised(t *testing.T) {
	sess, _ := authedLifecycleServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(tools.Tools, "flow_delete_node") {
		t.Fatalf("flow_delete_node not advertised; got %v", toolNames(tools.Tools))
	}
}

func TestLoopback_DeleteNode_WithoutConfirmReportsAndDeletesNothing(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_delete_node", map[string]any{"node": "leaf"})
	if res.IsError {
		t.Fatalf("dry run errored: %s", out)
	}
	if got := rec.snapshot(); got.deleteCalls != 0 {
		t.Fatalf("deleteCalls = %d, want 0 — a dry run must not delete", got.deleteCalls)
	}
	// The fixture gives l1 two own artifacts, one ancestor artifact, one free
	// artifact, a logo, and 750 minutes.
	if !strings.Contains(out, "2 artifact") {
		t.Fatalf("report = %q, want 2 own artifacts — the ancestor's and the free one must not be counted", out)
	}
	for _, want := range []string{"Would delete", "Leaf", "leaf", "12h 30m", "1 logo", "confirm=true"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(strings.ToLower(out), "session") {
		t.Errorf("report must speak of minutes, not sessions:\n%s", out)
	}
}

func TestLoopback_DeleteNode_NonProjectDocumentDoesNotBlock(t *testing.T) {
	sess, _ := authedLifecycleServer(t)
	// l1 owns a memory document; only project documents block deletion.
	res, out := callText(t, sess, "flow_delete_node", map[string]any{"node": "leaf"})
	if res.IsError {
		t.Fatalf("dry run errored: %s", out)
	}
	if strings.Contains(out, "Cannot delete") {
		t.Fatalf("a non-project document must not block deletion:\n%s", out)
	}
}

func TestLoopback_DeleteNode_ConfirmDeletesAndReportsIt(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_delete_node", map[string]any{"node": "leaf", "confirm": true})
	if res.IsError {
		t.Fatalf("confirmed delete errored: %s", out)
	}
	got := rec.snapshot()
	if got.deleteCalls != 1 || got.deleteID != "l1" {
		t.Fatalf("recorder = %+v, want exactly one delete of l1", got)
	}
	if !strings.Contains(out, "Deleted") || !strings.Contains(out, "leaf") {
		t.Fatalf("result = %q, want it to confirm the deletion", out)
	}
}

func TestLoopback_DeleteNode_ChildrenAndProjectDocsBlockTheDryRun(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	// v1/rebuild has the child r1 AND a project document.
	res, out := callText(t, sess, "flow_delete_node", map[string]any{"node": "rebuild"})
	if res.IsError {
		t.Fatalf("dry run of a blocked node must still be a normal report, got IsError: %s", out)
	}
	for _, want := range []string{"Cannot delete", "jukebox", "projekt/rebuild"} {
		if !strings.Contains(out, want) {
			t.Errorf("blocked report missing %q in:\n%s", want, out)
		}
	}
	if got := rec.snapshot(); got.deleteCalls != 0 {
		t.Fatalf("deleteCalls = %d, want 0", got.deleteCalls)
	}
}

// TestLoopback_DeleteNode_ConfirmOnABlockedNodeSurfacesTheServer409 is the
// error-path regression from Spec §7: the client-side report is advisory, the
// server is the authority, and its 409 must reach the model readably.
func TestLoopback_DeleteNode_ConfirmOnABlockedNodeSurfacesTheServer409(t *testing.T) {
	sess, rec := authedLifecycleServer(t)

	res, out := callText(t, sess, "flow_delete_node", map[string]any{"node": "rebuild", "confirm": true})
	if !res.IsError {
		t.Fatalf("confirmed delete of a node with children: want IsError, got %q", out)
	}
	if !strings.Contains(out, "children") {
		t.Fatalf("error = %q, want the server's 'node has children' message verbatim", out)
	}
	if got := rec.snapshot(); got.deleteCalls != 1 {
		t.Fatalf("deleteCalls = %d, want 1 (confirm must actually attempt it)", got.deleteCalls)
	}
}

// TestLoopback_DeleteNode_ProjectDocumentConflictSurfaces completes the 409
// matrix: besides the children conflict, the store refuses a node that still
// owns project documents (ports.ErrNodeHasProjectDocuments →
// internal/adapter/httpserver/worktime.go:284). The client report already warns,
// but a confirmed delete must surface the server's reason too.
func TestLoopback_DeleteNode_ProjectDocumentConflictSurfaces(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(lifecycleNodes())
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		for _, n := range lifecycleNodes() {
			if n.ID == r.PathValue("id") {
				_ = json.NewEncoder(w).Encode(n)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/stats", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.NodeRollup{})
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Artifact{})
	})
	// l1/leaf owns a project document but has no children.
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) {
		var out []domain.Document
		if r.URL.Query().Get("projectId") == "l1" {
			nid := "l1"
			out = append(out, domain.Document{ID: "d9", NodeID: &nid, Type: domain.DocProject, Path: "projekt/leaf", Title: "Leaf"})
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "node has project documents; move or reclassify them first", http.StatusConflict)
	})
	be := httptest.NewServer(mux)
	t.Cleanup(be.Close)

	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "l1", Name: "Leaf", Slug: "leaf", Kind: domain.KindRepo})
	sess := connect(t, h.srv)

	// The dry run must already call it out, without asking for confirm.
	res, out := callText(t, sess, "flow_delete_node", map[string]any{"node": "leaf"})
	if res.IsError {
		t.Fatalf("dry run errored: %s", out)
	}
	if !strings.Contains(out, "Cannot delete") || !strings.Contains(out, "projekt/leaf") {
		t.Fatalf("dry run = %q, want the project-document block named", out)
	}
	if strings.Contains(out, "confirm=true") {
		t.Fatalf("a blocked report must not invite confirm=true: %q", out)
	}

	// A confirmed delete anyway must surface the server's 409 verbatim.
	res, out = callText(t, sess, "flow_delete_node", map[string]any{"node": "leaf", "confirm": true})
	if !res.IsError {
		t.Fatalf("confirmed delete of a node with project documents: want IsError, got %q", out)
	}
	if !strings.Contains(out, "project documents") {
		t.Fatalf("error = %q, want the server's message verbatim", out)
	}
}

func TestLoopback_DeleteNode_MissingNodeIsAnError(t *testing.T) {
	sess, rec := authedLifecycleServer(t)
	res, out := callText(t, sess, "flow_delete_node", map[string]any{})
	if !res.IsError || !strings.Contains(out, "node is required") {
		t.Fatalf("missing node = (IsError=%v, %q), want 'node is required'", res.IsError, out)
	}
	if got := rec.snapshot(); got.deleteCalls != 0 {
		t.Fatalf("deleteCalls = %d, want 0", got.deleteCalls)
	}
}
```

- [ ] **Step 6: Handler-Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run TestLoopback_DeleteNode 2>&1 | head -20` → FAIL mit `undefined: deleteNodeIn` bzw. `flow_delete_node not advertised`.

- [ ] **Step 7: Delete-Teil an `tools_node_lifecycle.go` anfügen** — am Ende der Datei:

```go
// deleteNodeIn reports the consequences of deleting a node, and deletes only
// with confirm=true. `node` is deliberately required: silently deleting whatever
// this directory happens to resolve to is too dangerous to default into.
type deleteNodeIn struct {
	Node    string `json:"node" jsonschema:"the node to delete (id, slug, or name)"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"true actually deletes; omit or false only reports what deletion would destroy"`
}

// deleteImpact is everything the dry run learned about a node's deletion.
type deleteImpact struct {
	Node         domain.Node
	Children     []domain.Node
	ProjectDocs  []domain.Document
	OwnArtifacts int // ListArtifacts filtered to NodeID == Node.ID
	HasLogo      bool
	Rollup       apiclient.NodeRollup
}

// blocked reports whether the database would refuse this deletion outright.
func (d deleteImpact) blocked() bool {
	return len(d.Children) > 0 || len(d.ProjectDocs) > 0
}

// deleteImpactOf gathers the dry run from owner-scoped endpoints only.
//
// The artifact count is filtered to this node on purpose: ListArtifacts returns
// the node's own artifacts PLUS its whole ancestor chain PLUS the owner's free
// (node-less) library (internal/usecase/list_artifacts.go:21-51). Unfiltered, the
// report would threaten artifacts that deletion never touches — including the
// owner's entire free library.
func (h *handlers) deleteImpactOf(ctx context.Context, c *apiclient.Client, nodeID string) (deleteImpact, error) {
	node, err := c.GetNode(ctx, nodeID) // authoritative, and the only source of LogoRef
	if err != nil {
		return deleteImpact{}, err
	}
	impact := deleteImpact{Node: node, HasLogo: node.LogoRef != ""}

	nodes, err := h.nodeList(ctx, true) // refresh: a just-created child must be seen
	if err != nil {
		return deleteImpact{}, err
	}
	for _, n := range nodes {
		if n.ParentID != nil && *n.ParentID == node.ID {
			impact.Children = append(impact.Children, n)
		}
	}

	arts, err := c.ListArtifacts(ctx, node.ID)
	if err != nil {
		return deleteImpact{}, err
	}
	for _, a := range arts {
		if a.NodeID == node.ID {
			impact.OwnArtifacts++
		}
	}

	docs, err := c.ListDocumentsScoped(ctx, &node.ID)
	if err != nil {
		return deleteImpact{}, err
	}
	for _, d := range docs {
		if d.Type == domain.DocProject {
			impact.ProjectDocs = append(impact.ProjectDocs, d)
		}
	}

	rollup, err := c.NodeStats(ctx, node.ID)
	if err != nil {
		return deleteImpact{}, err
	}
	impact.Rollup = rollup
	return impact, nil
}

// deleteNode reports first, deletes only on confirm. The client-side report is
// advisory: the server stays the authority, and its 409 reaches the model
// verbatim through h.resultErr.
func (h *handlers) deleteNode(ctx context.Context, req *mcp.CallToolRequest, in deleteNodeIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Node) == "" {
		return h.resultErr(errGuard{errors.New("node is required: the id, slug, or name of the node to delete")}), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		ref, err := h.lookupNode(ctx, strings.TrimSpace(in.Node))
		if err != nil {
			return err
		}
		impact, err := h.deleteImpactOf(ctx, c, ref.ID)
		if err != nil {
			return err
		}
		if !in.Confirm {
			out = formatDeleteImpact(impact)
			return nil
		}
		if err := c.DeleteNode(ctx, impact.Node.ID); err != nil {
			return err
		}
		if _, lerr := h.nodeList(ctx, true); lerr != nil {
			mcpLog().Warn("could not refresh the node cache after delete", "err", lerr)
		}
		out = fmt.Sprintf("Deleted %s %q (%s).", impact.Node.Kind, impact.Node.Name, impact.Node.Slug)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
```

- [ ] **Step 8: Tool registrieren** — in `cmd/flow-mcp/server.go` nach dem `flow_move_node`-Block:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_delete_node",
		Description: "Report what deleting a node would destroy, and delete it only with confirm=true — without confirm NOTHING is deleted. A node with children or project documents cannot be deleted at all. Booked worktime and non-project documents lose their node but survive; bindings, artifacts and the logo are deleted with it.",
	}, h.deleteNode)
```

- [ ] **Step 9: Tool-Zahl auf 28 ziehen** — in `cmd/flow-mcp/loopback_test.go` bei `:353-355` die `27` durch `28` ersetzen (Bedingung und Meldung).

- [ ] **Step 10: Tests laufen lassen, grün bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestFormatMinutes|TestFormatDeleteImpact|TestJoinCapped|TestDeleteImpactBlocked|TestLoopback_DeleteNode' -v 2>&1 | tail -35` → PASS. Dann `go test ./cmd/flow-mcp/ 2>&1 | tail -5` → ok.

- [ ] **Step 11: Commit** — `git add cmd/flow-mcp/tools_node_lifecycle.go cmd/flow-mcp/tools_node_lifecycle_test.go cmd/flow-mcp/format_nodes.go cmd/flow-mcp/format_nodes_test.go cmd/flow-mcp/server.go cmd/flow-mcp/loopback_test.go && git commit -m "feat(flow-mcp): add flow_delete_node with a dry-run consequence report"`

---

### Task 7: `flow_get_node` — das MCP-Gegenstück zu `flow node show`

**Tool-Zahl 28 → 29.**

**Files:**
- Create: `cmd/flow-mcp/tools_node_inspect.go`
- Create: `cmd/flow-mcp/tools_node_inspect_test.go`
- Modify: `cmd/flow-mcp/format_nodes.go` (anfügen: `nodeDetail`, `formatNodeDetail`)
- Modify: `cmd/flow-mcp/format_nodes_test.go` (anfügen)
- Modify: `cmd/flow-mcp/server.go` (Registrierung `flow_get_node`)
- Modify: `cmd/flow-mcp/loopback_test.go` (`:353-355` Tool-Zahl 28 → 29)

**Interfaces:**
- Consumes: `h.nodeTarget(ctx context.Context, ref string) (domain.Node, error)` (Task 1); `c.GetNode(ctx, id string) (domain.Node, error)`; `c.Ancestors(ctx, id string) ([]domain.Node, error)` — **leaf→root**; `c.NodeTags(ctx, id string) ([]domain.Tag, error)` mit `domain.Tag{Slug, Display string}`; `c.ListBindings(ctx) ([]domain.ProjectBinding, error)`; `c.NodeStats(ctx, id string) (apiclient.NodeRollup, error)`; `domain.ProjectBinding{NodeID string; Kind domain.BindingKind; RemoteSlug, MachineID, MachineLabel, Path string}`; `domain.BindingRemote`; `timefmt.FormatMin` aus `github.com/serverkraken/flow/internal/timefmt` (Task 0), `nodeKindGlyph` (Task 2).
- Produces:
  - `type getNodeIn struct{ Node string }`
  - `func (h *handlers) getNode(ctx context.Context, req *mcp.CallToolRequest, in getNodeIn) (*mcp.CallToolResult, any, error)`
  - `func bindingsForNode(bs []domain.ProjectBinding, nodeID string) []domain.ProjectBinding`
  - `type nodeDetail struct{ Node domain.Node; Chain []domain.Node; Tags []domain.Tag; Bindings []domain.ProjectBinding; Rollup apiclient.NodeRollup }`
  - `func formatNodeDetail(d nodeDetail) string`

- [ ] **Step 1: Bestand verifizieren** — diese drei Befehle laufen lassen:

```bash
rg -n "func \(c \*Client\) (Ancestors|NodeTags|ListBindings|GetNode|NodeStats)" -A 6 internal/adapter/apiclient/
rg -n "type Tag struct" -A 8 internal/domain/tag.go
rg -n "type ProjectBinding struct" -A 8 internal/domain/projectbinding.go
```

Erwartet: `Ancestors(ctx, id) ([]domain.Node, error)` liefert **leaf→root**, `ListBindings(ctx)` nimmt **keine** Filterparameter, `GetNode`/`NodeStats` wie in Task 6; `domain.Tag` hat `Slug` und `Display`; `domain.ProjectBinding` hat **keine json-Tags** — der Fake-Backend encodiert deshalb `domain.ProjectBinding` direkt, damit die Go-Feldnamen auf der Leitung mit dem übereinstimmen, was `apiclient` dekodiert.

- [ ] **Step 2: Failing Renderer-Test schreiben** — an `cmd/flow-mcp/format_nodes_test.go` anfügen:

```go
func TestFormatNodeDetail_ShowsChainRootToLeafTagsBindingsAndRollup(t *testing.T) {
	out := formatNodeDetail(nodeDetail{
		Node: domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo,
			Status: domain.NodeActive, Description: "der Plattenspieler",
			UpstreamGit: "git@github.com:serverkraken/jukebox.git"},
		// Ancestors returns leaf→root; the breadcrumb must print root→leaf.
		Chain: []domain.Node{
			{ID: "r1", Name: "Jukebox"}, {ID: "v1", Name: "Rebuild"}, {ID: "e1", Name: "Alpha"},
		},
		Tags: []domain.Tag{{Slug: "go", Display: "go"}, {Slug: "audio", Display: "audio"}},
		Bindings: []domain.ProjectBinding{
			{NodeID: "r1", Kind: domain.BindingRemote, RemoteSlug: "github.com/serverkraken/jukebox"},
			{NodeID: "r1", Kind: domain.BindingPath, MachineID: "m1", MachineLabel: "notebook-a", Path: "/work/jukebox"},
		},
		Rollup: apiclient.NodeRollup{TotalMin: 750, WeekMin: 120, MonthMin: 300},
	})
	for _, want := range []string{"Jukebox", "jukebox", "repo", "active", "r1",
		"der Plattenspieler", "github.com/serverkraken/jukebox",
		"Alpha / Rebuild / Jukebox", "go", "audio",
		"12h 30m", "2h 00m", "5h 00m",
		"notebook-a", "m1", "/work/jukebox"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Jukebox / Rebuild / Alpha") {
		t.Errorf("breadcrumb is leaf→root; it must be printed root→leaf:\n%s", out)
	}
}

func TestFormatNodeDetail_EmptyTagsAndBindingsAreStatedNotOmitted(t *testing.T) {
	out := formatNodeDetail(nodeDetail{
		Node: domain.Node{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement, Status: domain.NodeActive},
	})
	if !strings.Contains(out, "bindings: none") {
		t.Errorf("detail must state that there are no bindings:\n%s", out)
	}
	if !strings.Contains(out, "tags: —") {
		t.Errorf("detail must state that there are no tags:\n%s", out)
	}
	if strings.Contains(out, "description:") || strings.Contains(out, "upstream:") {
		t.Errorf("unset optional fields must be omitted entirely:\n%s", out)
	}
}

func TestBindingsForNode_FiltersClientSide(t *testing.T) {
	all := []domain.ProjectBinding{
		{ID: "b1", NodeID: "r1", Kind: domain.BindingRemote, RemoteSlug: "a/b"},
		{ID: "b2", NodeID: "other", Kind: domain.BindingPath, MachineID: "m1", Path: "/x"},
		{ID: "b3", NodeID: "r1", Kind: domain.BindingPath, MachineID: "m2", Path: "/y"},
	}
	got := bindingsForNode(all, "r1")
	if len(got) != 2 || got[0].ID != "b1" || got[1].ID != "b3" {
		t.Fatalf("bindingsForNode = %+v, want b1 and b3 in order", got)
	}
	if bindingsForNode(all, "nobody") != nil {
		t.Error("no match must yield nil, not an empty non-nil slice")
	}
}
```

- [ ] **Step 3: Renderer-Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestFormatNodeDetail|TestBindingsForNode' 2>&1 | head -20` → FAIL mit `undefined: formatNodeDetail`, `undefined: nodeDetail`, `undefined: bindingsForNode`.

- [ ] **Step 4: `formatNodeDetail` an `format_nodes.go` anfügen** — am Ende der Datei; der Import `"github.com/serverkraken/flow/internal/adapter/apiclient"` kommt jetzt hinzu:

```go
// nodeDetail is everything flow_get_node shows about one node.
type nodeDetail struct {
	Node     domain.Node
	Chain    []domain.Node           // leaf→root, as apiclient.Ancestors returns
	Tags     []domain.Tag
	Bindings []domain.ProjectBinding // already filtered to this node
	Rollup   apiclient.NodeRollup
}

// formatNodeDetail renders one node in full. The breadcrumb is printed root→leaf
// even though Ancestors delivers leaf→root, matching `flow node show`
// (cmd/flow/node_subcommands.go:166-170). Empty tags and empty bindings are
// STATED rather than omitted, so a model never mistakes silence for absence.
func formatNodeDetail(d nodeDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %q (%s)\nid: %s\nstatus: %s\n",
		nodeKindGlyph(d.Node.Kind), d.Node.Kind, d.Node.Name, d.Node.Slug, d.Node.ID, d.Node.Status)
	if d.Node.Description != "" {
		fmt.Fprintf(&b, "description: %s\n", d.Node.Description)
	}
	if d.Node.UpstreamGit != "" {
		fmt.Fprintf(&b, "upstream: %s\n", d.Node.UpstreamGit)
	}
	if len(d.Chain) > 0 {
		crumbs := make([]string, 0, len(d.Chain))
		for i := len(d.Chain) - 1; i >= 0; i-- {
			crumbs = append(crumbs, d.Chain[i].Name)
		}
		fmt.Fprintf(&b, "path: %s\n", strings.Join(crumbs, " / "))
	}
	tags := "—"
	if len(d.Tags) > 0 {
		names := make([]string, len(d.Tags))
		for i, t := range d.Tags {
			names[i] = t.Slug
		}
		tags = strings.Join(names, ", ")
	}
	fmt.Fprintf(&b, "tags: %s\n", tags)
	fmt.Fprintf(&b, "worktime (subtree): total %s · this week %s · this month %s\n",
		timefmt.FormatMin(d.Rollup.TotalMin), timefmt.FormatMin(d.Rollup.WeekMin), timefmt.FormatMin(d.Rollup.MonthMin))
	if len(d.Bindings) == 0 {
		b.WriteString("bindings: none\n")
	} else {
		b.WriteString("bindings:\n")
		for _, bd := range d.Bindings {
			if bd.Kind == domain.BindingRemote {
				fmt.Fprintf(&b, "- remote %s\n", bd.RemoteSlug)
				continue
			}
			fmt.Fprintf(&b, "- path %s on machine %s [%s]\n", bd.Path, bd.MachineLabel, bd.MachineID)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
```

- [ ] **Step 5: Failing Handler-Test schreiben** — neue Datei `cmd/flow-mcp/tools_node_inspect_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// fakeInspectBackend serves the five reads flow_get_node performs.
// Tree: e1/alpha → v1/rebuild → r1/jukebox.
func fakeInspectBackend(t *testing.T) *httptest.Server {
	t.Helper()
	e1, v1 := "e1", "v1"
	nodes := []domain.Node{
		{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement, Status: domain.NodeActive},
		{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben, ParentID: &e1, Status: domain.NodeActive},
		{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, ParentID: &v1, Status: domain.NodeActive,
			Description: "der Plattenspieler"},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(nodes)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/ancestors", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "r1" {
			_ = json.NewEncoder(w).Encode([]domain.Node{})
			return
		}
		_ = json.NewEncoder(w).Encode([]domain.Node{nodes[2], nodes[1], nodes[0]}) // leaf→root
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/tags", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Tag{{ID: "t1", Slug: "go", Display: "go"}})
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/stats", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.NodeRollup{TotalMin: 750, WeekMin: 120, MonthMin: 300})
	})
	mux.HandleFunc("GET /api/v1/nodes/bindings", func(w http.ResponseWriter, _ *http.Request) {
		// Owner-wide across ALL devices — including one that belongs to another node.
		_ = json.NewEncoder(w).Encode([]domain.ProjectBinding{
			{ID: "b1", NodeID: "r1", Kind: domain.BindingPath, MachineID: "m1", MachineLabel: "notebook-a", Path: "/work/jukebox"},
			{ID: "b2", NodeID: "e1", Kind: domain.BindingPath, MachineID: "m2", MachineLabel: "notebook-b", Path: "/work/alpha"},
		})
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		for _, n := range nodes {
			if n.ID == r.PathValue("id") {
				_ = json.NewEncoder(w).Encode(n)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	return httptest.NewServer(mux)
}

// authedInspectServer binds the session to r1/jukebox so the omitted-node path
// is exercisable; unbound is a separate helper.
func authedInspectServer(t *testing.T, bound bool) *mcp.ClientSession {
	t.Helper()
	be := fakeInspectBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo}
	mgr := newAuthManager(func(context.Context) (*apiclient.Client, error) { return client, nil }, nil)
	_, h := newServerH(mgr)
	mgr.onAuth = nil
	if bound {
		h.projMu.Lock()
		h.proj, h.matched = proj, true
		h.projMu.Unlock()
	}
	return connect(t, h.srv)
}

func TestLoopback_GetNode_Advertised(t *testing.T) {
	sess := authedInspectServer(t, true)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(tools.Tools, "flow_get_node") {
		t.Fatalf("flow_get_node not advertised; got %v", toolNames(tools.Tools))
	}
}

func TestLoopback_GetNode_ExplicitNodeShowsChainTagsBindingsAndRollup(t *testing.T) {
	sess := authedInspectServer(t, true)

	res, out := callText(t, sess, "flow_get_node", map[string]any{"node": "jukebox"})
	if res.IsError {
		t.Fatalf("get_node errored: %s", out)
	}
	for _, want := range []string{"Jukebox", "jukebox", "repo", "der Plattenspieler",
		"Alpha / Rebuild / Jukebox", "go", "12h 30m", "notebook-a", "/work/jukebox"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q in:\n%s", want, out)
		}
	}
	// The other node's binding must not leak into this node's detail.
	if strings.Contains(out, "/work/alpha") || strings.Contains(out, "notebook-b") {
		t.Errorf("detail leaked another node's binding (ListBindings is owner-wide and must be filtered):\n%s", out)
	}
}

func TestLoopback_GetNode_OmittedUsesTheBoundNode(t *testing.T) {
	sess := authedInspectServer(t, true)
	res, out := callText(t, sess, "flow_get_node", map[string]any{})
	if res.IsError {
		t.Fatalf("get_node without node errored: %s", out)
	}
	if !strings.Contains(out, "Jukebox") {
		t.Fatalf("result = %q, want the directory-bound node", out)
	}
}

func TestLoopback_GetNode_OmittedAndUnboundPointsAtTheBindingTools(t *testing.T) {
	sess := authedInspectServer(t, false)
	res, out := callText(t, sess, "flow_get_node", map[string]any{})
	if !res.IsError {
		t.Fatalf("unbound get_node: want IsError, got %q", out)
	}
	if !strings.Contains(out, "flow_node_binding") {
		t.Fatalf("error = %q, want it to point at flow_node_binding", out)
	}
}

func TestLoopback_GetNode_UnknownNodeErrors(t *testing.T) {
	sess := authedInspectServer(t, true)
	res, out := callText(t, sess, "flow_get_node", map[string]any{"node": "bogus"})
	if !res.IsError || !strings.Contains(out, "unknown project") {
		t.Fatalf("unknown node = (IsError=%v, %q), want IsError + 'unknown project'", res.IsError, out)
	}
}
```

> `newAuthManager` und `newServerH` sind Bestand: `newAuthManager(build func(context.Context) (*apiclient.Client, error), onAuth func(context.Context, *apiclient.Client)) *authManager` (`cmd/flow-mcp/auth_manager.go:38`, verifiziert) und `newServerH(mgr *authManager) (*mcp.Server, *handlers)` (`server.go:47`). Der Helfer baut den unbound-Fall selbst, weil `managerFor` immer `matched = true` setzt (`loopback_test.go:62-64`); `mgr.onAuth = nil` danach ist derselbe Trick, den `managerFor` anwendet, weil die Fixtures die V0-Resolution nicht bedienen.

- [ ] **Step 6: Handler-Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run TestLoopback_GetNode 2>&1 | head -20` → FAIL mit `flow_get_node not advertised`.

- [ ] **Step 7: `tools_node_inspect.go` schreiben** — neue Datei `cmd/flow-mcp/tools_node_inspect.go`:

```go
package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// getNodeIn shows one node in full.
type getNodeIn struct {
	Node string `json:"node,omitempty" jsonschema:"the node to show (id, slug, or name); omit to use the node bound to this directory"`
}

// bindingsForNode narrows an owner-wide binding list to one node. ListBindings
// takes no filter parameters (internal/adapter/apiclient/projectbindings.go:103),
// so the filtering is client-side — and it MUST happen, because the list spans
// every device of this owner.
func bindingsForNode(bs []domain.ProjectBinding, nodeID string) []domain.ProjectBinding {
	var out []domain.ProjectBinding
	for _, b := range bs {
		if b.NodeID == nodeID {
			out = append(out, b)
		}
	}
	return out
}

// getNode is the MCP counterpart to `flow node show`: detail, ancestor chain,
// node tags, this node's bindings and the worktime rollup. The node reference is
// re-read with GetNode so Description, LogoRef and Status are authoritative even
// when the omitted-node branch used the auth-time snapshot.
func (h *handlers) getNode(ctx context.Context, req *mcp.CallToolRequest, in getNodeIn) (*mcp.CallToolResult, any, error) {
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		ref, err := h.nodeTarget(ctx, in.Node)
		if err != nil {
			return err
		}
		node, err := c.GetNode(ctx, ref.ID)
		if err != nil {
			return err
		}
		chain, err := c.Ancestors(ctx, node.ID)
		if err != nil {
			return err
		}
		tags, err := c.NodeTags(ctx, node.ID)
		if err != nil {
			return err
		}
		allBindings, err := c.ListBindings(ctx)
		if err != nil {
			return err
		}
		rollup, err := c.NodeStats(ctx, node.ID)
		if err != nil {
			return err
		}
		out = formatNodeDetail(nodeDetail{
			Node: node, Chain: chain, Tags: tags,
			Bindings: bindingsForNode(allBindings, node.ID), Rollup: rollup,
		})
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
```

- [ ] **Step 8: Tool registrieren** — in `cmd/flow-mcp/server.go` nach dem `flow_delete_node`-Block:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_get_node",
		Description: "Show one node in full: kind, status, description, upstream, its ancestor path, its node tags, its bindings (with machine label per path binding) and its worktime rollup. Omit node to use the node bound to this directory.",
	}, h.getNode)
```

- [ ] **Step 9: Tool-Zahl auf 29 ziehen** — in `cmd/flow-mcp/loopback_test.go` bei `:353-355` die `28` durch `29` ersetzen (Bedingung und Meldung).

- [ ] **Step 10: Tests laufen lassen, grün bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestFormatNodeDetail|TestBindingsForNode|TestLoopback_GetNode' -v 2>&1 | tail -25` → PASS. Dann `go test ./cmd/flow-mcp/ 2>&1 | tail -5` → ok.

- [ ] **Step 11: Commit** — `git add cmd/flow-mcp/tools_node_inspect.go cmd/flow-mcp/tools_node_inspect_test.go cmd/flow-mcp/format_nodes.go cmd/flow-mcp/format_nodes_test.go cmd/flow-mcp/server.go cmd/flow-mcp/loopback_test.go && git commit -m "feat(flow-mcp): add flow_get_node with chain, tags, bindings and rollup"`

---

### Task 8: `flow_set_node_tags` — die Tag-Menge komplett ersetzen

**Tool-Zahl 29 → 30.** Dieselbe Replace-Semantik wie `flow_create_doc` für Dokument-Tags; die Description sagt es ausdrücklich, damit ein Modell keine Tags wegwirft (Spec §3).

**Files:**
- Create: `cmd/flow-mcp/tools_node_tags.go`
- Create: `cmd/flow-mcp/tools_node_tags_test.go`
- Modify: `cmd/flow-mcp/format_nodes.go` (anfügen: `formatNodeTags`)
- Modify: `cmd/flow-mcp/format_nodes_test.go` (anfügen)
- Modify: `cmd/flow-mcp/server.go` (Registrierung `flow_set_node_tags`)
- Modify: `cmd/flow-mcp/loopback_test.go` (`:353-355` Tool-Zahl 29 → 30)

**Interfaces:**
- Consumes: `c.SetNodeTags(ctx context.Context, id string, tags []string) ([]domain.Tag, error)`; `h.nodeTarget(ctx, ref string) (domain.Node, error)` (Task 1); `domain.Tag{Slug, Display string}`.
- Produces:
  - `type setNodeTagsIn struct{ Node string; Tags []string }`
  - `func (h *handlers) setNodeTags(ctx context.Context, req *mcp.CallToolRequest, in setNodeTagsIn) (*mcp.CallToolResult, any, error)`
  - `func formatNodeTags(node domain.Node, tags []domain.Tag) string`

- [ ] **Step 1: Bestand verifizieren** — `rg -n "func \(c \*Client\) SetNodeTags" -A 6 internal/adapter/apiclient/nodes.go`. Erwartet: `SetNodeTags(ctx, id string, tags []string) ([]domain.Tag, error)` — der Rückgabewert ist die resultierende Menge, die der Bericht nennt.

- [ ] **Step 2: Failing Renderer-Test schreiben** — an `cmd/flow-mcp/format_nodes_test.go` anfügen:

```go
func TestFormatNodeTags(t *testing.T) {
	node := domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo}
	out := formatNodeTags(node, []domain.Tag{{Slug: "go"}, {Slug: "audio"}})
	for _, want := range []string{"Jukebox", "jukebox", "go", "audio", "now has"} {
		if !strings.Contains(out, want) {
			t.Errorf("tag result missing %q in: %s", want, out)
		}
	}
	empty := formatNodeTags(node, nil)
	if !strings.Contains(empty, "no tags") {
		t.Errorf("cleared tags = %q, want it to state the empty result", empty)
	}
}
```

- [ ] **Step 3: Renderer-Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run TestFormatNodeTags 2>&1 | head -10` → FAIL mit `undefined: formatNodeTags`.

- [ ] **Step 4: `formatNodeTags` an `format_nodes.go` anfügen** — am Ende der Datei:

```go
// formatNodeTags names the resulting set, so the caller can see exactly what the
// replace produced (Spec §3 flow_set_node_tags).
func formatNodeTags(node domain.Node, tags []domain.Tag) string {
	if len(tags) == 0 {
		return fmt.Sprintf("%s %q (%s) now has no tags.", node.Kind, node.Name, node.Slug)
	}
	names := make([]string, len(tags))
	for i, t := range tags {
		names[i] = t.Slug
	}
	return fmt.Sprintf("%s %q (%s) now has tags: %s.", node.Kind, node.Name, node.Slug, strings.Join(names, ", "))
}
```

- [ ] **Step 5: Failing Handler-Test schreiben** — neue Datei `cmd/flow-mcp/tools_node_tags_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// tagRecorder captures the PUT body so the replace semantics are provable.
type tagRecorder struct {
	mu     sync.Mutex
	nodeID string
	tags   []string
	calls  int
	sent   bool
}

func (r *tagRecorder) snapshot() (string, []string, int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nodeID, r.tags, r.calls, r.sent
}

func fakeNodeTagBackend(t *testing.T, rec *tagRecorder) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Node{
			{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, Status: domain.NodeActive},
		})
	})
	mux.HandleFunc("PUT /api/v1/nodes/{id}/tags", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tags []string `json:"tags"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		rec.mu.Lock()
		rec.nodeID, rec.tags, rec.calls = r.PathValue("id"), body.Tags, rec.calls+1
		rec.sent = strings.Contains(string(raw), `"tags"`)
		rec.mu.Unlock()
		out := make([]domain.Tag, 0, len(body.Tags))
		for i, tg := range body.Tags {
			out = append(out, domain.Tag{ID: fmt.Sprintf("t%d", i), Slug: tg, Display: tg})
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	return httptest.NewServer(mux)
}

func authedNodeTagServer(t *testing.T) (*mcp.ClientSession, *tagRecorder) {
	t.Helper()
	rec := &tagRecorder{}
	be := fakeNodeTagBackend(t, rec)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo})
	return connect(t, h.srv), rec
}

func TestLoopback_SetNodeTags_Advertised(t *testing.T) {
	sess, _ := authedNodeTagServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(tools.Tools, "flow_set_node_tags") {
		t.Fatalf("flow_set_node_tags not advertised; got %v", toolNames(tools.Tools))
	}
	// The description must warn that this REPLACES the set.
	for _, tool := range tools.Tools {
		if tool.Name == "flow_set_node_tags" && !strings.Contains(strings.ToUpper(tool.Description), "REPLACE") {
			t.Errorf("description must state the replace semantics: %q", tool.Description)
		}
	}
}

func TestLoopback_SetNodeTags_ReplacesAndReportsTheResultingSet(t *testing.T) {
	sess, rec := authedNodeTagServer(t)

	res, out := callText(t, sess, "flow_set_node_tags", map[string]any{
		"node": "jukebox", "tags": []any{"go", "audio"},
	})
	if res.IsError {
		t.Fatalf("set tags errored: %s", out)
	}
	nodeID, tags, calls, _ := rec.snapshot()
	if calls != 1 || nodeID != "r1" {
		t.Fatalf("calls=%d nodeID=%q, want one PUT on r1", calls, nodeID)
	}
	if len(tags) != 2 || tags[0] != "go" || tags[1] != "audio" {
		t.Fatalf("sent tags = %v, want [go audio]", tags)
	}
	for _, want := range []string{"go", "audio", "now has"} {
		if !strings.Contains(out, want) {
			t.Errorf("result missing %q in: %s", want, out)
		}
	}
}

func TestLoopback_SetNodeTags_EmptyListClearsTheSet(t *testing.T) {
	sess, rec := authedNodeTagServer(t)

	res, out := callText(t, sess, "flow_set_node_tags", map[string]any{
		"node": "jukebox", "tags": []any{},
	})
	if res.IsError {
		t.Fatalf("clearing tags errored: %s", out)
	}
	_, tags, calls, sent := rec.snapshot()
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 — [] is a real clear, not a no-op", calls)
	}
	if len(tags) != 0 {
		t.Fatalf("sent tags = %v, want an empty list", tags)
	}
	if !sent {
		t.Fatal(`the request body must carry a "tags" key even when clearing`)
	}
	if !strings.Contains(out, "no tags") {
		t.Fatalf("result = %q, want it to state the empty result", out)
	}
}

func TestLoopback_SetNodeTags_OmittedTagsIsAnErrorNotAnAccidentalClear(t *testing.T) {
	sess, rec := authedNodeTagServer(t)

	res, out := callText(t, sess, "flow_set_node_tags", map[string]any{"node": "jukebox"})
	if !res.IsError {
		t.Fatalf("omitted tags: want IsError, got %q", out)
	}
	if !strings.Contains(out, "tags is required") {
		t.Fatalf("error = %q, want 'tags is required'", out)
	}
	if _, _, calls, _ := rec.snapshot(); calls != 0 {
		t.Fatalf("calls = %d, want 0 — an omitted list must never silently clear", calls)
	}
}

func TestLoopback_SetNodeTags_OmittedNodeUsesTheBoundNode(t *testing.T) {
	sess, rec := authedNodeTagServer(t)

	res, out := callText(t, sess, "flow_set_node_tags", map[string]any{"tags": []any{"go"}})
	if res.IsError {
		t.Fatalf("set tags on the bound node errored: %s", out)
	}
	if nodeID, _, _, _ := rec.snapshot(); nodeID != "r1" {
		t.Fatalf("nodeID = %q, want the directory-bound node r1", nodeID)
	}
}
```

> Der Test braucht den Import `"io"`. Vor dem Tippen mit `rg -n "PUT /api/v1/nodes/{id}/tags" internal/adapter/httpserver/server.go` bestätigen, dass die Route so heißt.

- [ ] **Step 6: Handler-Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run TestLoopback_SetNodeTags 2>&1 | head -20` → FAIL mit `flow_set_node_tags not advertised`.

- [ ] **Step 7: `tools_node_tags.go` schreiben** — neue Datei `cmd/flow-mcp/tools_node_tags.go`:

```go
package main

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

// setNodeTagsIn REPLACES a node's complete tag set — the same semantics
// flow_create_doc has for document tags. Tags is a plain []string so an omitted
// list arrives as nil and can be told apart from an explicit [] (a real clear):
// silently wiping a node's tags because a model forgot the field would be the
// worst possible default (Spec §3 flow_set_node_tags).
type setNodeTagsIn struct {
	Node string   `json:"node,omitempty" jsonschema:"the node whose tags to replace (id, slug, or name); omit to use the node bound to this directory"`
	Tags []string `json:"tags" jsonschema:"the COMPLETE tag set. This REPLACES the node's tags — every tag you omit is REMOVED. Pass [] to clear them."`
}

// setNodeTags replaces the tag set and reports the result.
func (h *handlers) setNodeTags(ctx context.Context, req *mcp.CallToolRequest, in setNodeTagsIn) (*mcp.CallToolResult, any, error) {
	if in.Tags == nil {
		return h.resultErr(errGuard{errors.New(`tags is required: pass the COMPLETE tag set (it REPLACES the node's tags), or [] to clear them`)}), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		ref, err := h.nodeTarget(ctx, in.Node)
		if err != nil {
			return err
		}
		tags, err := c.SetNodeTags(ctx, ref.ID, in.Tags)
		if err != nil {
			return err
		}
		out = formatNodeTags(ref, tags)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
```

- [ ] **Step 8: Tool registrieren** — in `cmd/flow-mcp/server.go` nach dem `flow_get_node`-Block:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_set_node_tags",
		Description: "REPLACE a node's complete tag set. This is NOT an add: every tag you omit is removed, so always pass the full set you want (read the current one with flow_get_node first). Pass [] to clear. Omit node to use the node bound to this directory. Returns the resulting set.",
	}, h.setNodeTags)
```

- [ ] **Step 9: Tool-Zahl auf 30 ziehen** — in `cmd/flow-mcp/loopback_test.go` bei `:353-355` die `29` durch `30` ersetzen (Bedingung und Meldung).

- [ ] **Step 10: Tests laufen lassen, grün bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestFormatNodeTags|TestLoopback_SetNodeTags' -v 2>&1 | tail -25` → PASS. Dann `go test ./cmd/flow-mcp/ 2>&1 | tail -5` → ok.

- [ ] **Step 11: Commit** — `git add cmd/flow-mcp/tools_node_tags.go cmd/flow-mcp/tools_node_tags_test.go cmd/flow-mcp/format_nodes.go cmd/flow-mcp/format_nodes_test.go cmd/flow-mcp/server.go cmd/flow-mcp/loopback_test.go && git commit -m "feat(flow-mcp): add flow_set_node_tags with explicit replace semantics"`

---

### Task 9: `flow_node_binding` — bind, unbind, list, resolve

**Tool-Zahl 30 → 31.** Vier Aktionen auf derselben Ziel-Adressierung. `resolve` ist kein eigenes Tool, weil es dieselbe Frage wie `list` ist, nur von der anderen Seite gestellt (Spec §3).

**Files:**
- Modify: `cmd/flow-mcp/tools_bindings.go` (anfügen: `nodeBindingIn`, `nodeBindingActions`, `validateNodeBinding`, `nodeBinding`, `bindingRows`)
- Modify: `cmd/flow-mcp/tools_bindings_test.go` (anfügen)
- Modify: `cmd/flow-mcp/format_nodes.go` (anfügen: `bindingRow`, `formatBindingRows`, `formatResolvedTarget`)
- Modify: `cmd/flow-mcp/format_nodes_test.go` (anfügen)
- Modify: `cmd/flow-mcp/server.go` (Registrierung `flow_node_binding`)
- Modify: `cmd/flow-mcp/loopback_test.go` (`:353-355` Tool-Zahl 30 → 31)

**Interfaces:**
- Consumes: `bindTargetTo(ctx, c *apiclient.Client, nodeID string, tgt bindTarget) error`, `unbindTarget(ctx, c *apiclient.Client, tgt bindTarget) error` (Task 3); `resolveBindTarget`, `liveBindEnv`, `bindTargetLabel`, `bindTarget` (Task 1); `bindingsForNode(bs []domain.ProjectBinding, nodeID string) []domain.ProjectBinding` (Task 7); `c.ListBindings(ctx) ([]domain.ProjectBinding, error)`; `c.ResolveNode(ctx, remoteSlug, machineID, cwd string) (domain.Node, bool, error)`; `c.ResolveEngagement(ctx, remoteSlug, machineID, cwd string) (domain.Node, bool, error)`; `h.lookupNode`, `h.nodeList`, `h.refreshResolved`; `domain.BindingRemote`.
- Produces:
  - `type nodeBindingIn struct{ Action, Node, Path, Remote, Kind string }`
  - `var nodeBindingActions = []string{"bind", "unbind", "list", "resolve"}`
  - `func validateNodeBinding(in nodeBindingIn) (string, error)`
  - `func (h *handlers) nodeBinding(ctx context.Context, req *mcp.CallToolRequest, in nodeBindingIn) (*mcp.CallToolResult, any, error)`
  - `func (h *handlers) bindingRows(ctx context.Context, c *apiclient.Client, nodeRef string) ([]bindingRow, error)`
  - `type bindingRow struct{ Binding domain.ProjectBinding; NodeName, NodeSlug string }`
  - `func formatBindingRows(rows []bindingRow, label string) string`
  - `func formatResolvedTarget(target string, node, engagement domain.Node, engagementOK bool) string`

- [ ] **Step 1: Bestand verifizieren** — diese drei Befehle laufen lassen:

```bash
rg -n "func \(c \*Client\) (BindRemote|BindPath|UnbindRemote|UnbindPath|ResolveNode|ResolveEngagement|ListBindings)" -A 4 internal/adapter/apiclient/projectbindings.go
rg -n "Upsert" -A 3 internal/usecase/bind_node.go
rg -n "ON CONFLICT" -B 3 internal/adapter/pgstore/projectbindings.go
```

Erwartet: `UnbindRemote(ctx, remoteSlug)` und `UnbindPath(ctx, machineID, path)` nehmen **keinen** Node, `BindRemote`/`BindPath` dagegen schon; `ResolveNode`/`ResolveEngagement` nehmen `(remoteSlug, machineID, cwd)` und liefern `(domain.Node, bool, error)` mit 404 → `ok=false, err=nil`; `ListBindings` nimmt keinen Filter. Daraus folgt: `node` ist bei `unbind` und `resolve` ein Fehler. Der Upsert-Nachweis (`bind_node.go:54`, `projectbindings.go:49`) begründet die Rebind-Warnung in der Description — bestätigen, dass wirklich nur `node_id` ersetzt wird.

- [ ] **Step 2: Failing Renderer-Test schreiben** — an `cmd/flow-mcp/format_nodes_test.go` anfügen:

```go
func TestFormatBindingRows_ShowsMachineLabelAndID(t *testing.T) {
	rows := []bindingRow{
		{Binding: domain.ProjectBinding{ID: "b1", NodeID: "r1", Kind: domain.BindingRemote,
			RemoteSlug: "github.com/serverkraken/jukebox"}, NodeName: "Jukebox", NodeSlug: "jukebox"},
		{Binding: domain.ProjectBinding{ID: "b2", NodeID: "e1", Kind: domain.BindingPath,
			MachineID: "m2", MachineLabel: "notebook-b", Path: "/work/alpha"}, NodeName: "Alpha", NodeSlug: "alpha"},
	}
	out := formatBindingRows(rows, "for this owner across all devices")
	for _, want := range []string{"2 binding", "github.com/serverkraken/jukebox", "Jukebox",
		"/work/alpha", "notebook-b", "m2", "Alpha"} {
		if !strings.Contains(out, want) {
			t.Errorf("binding list missing %q in:\n%s", want, out)
		}
	}
	if empty := formatBindingRows(nil, "for node jukebox"); !strings.HasPrefix(empty, "No bindings") {
		t.Errorf("empty list = %q, want a 'No bindings' message", empty)
	}
}

func TestFormatBindingRows_UnknownNodeIsLabelledNotBlank(t *testing.T) {
	out := formatBindingRows([]bindingRow{
		{Binding: domain.ProjectBinding{ID: "b1", NodeID: "gone", Kind: domain.BindingRemote, RemoteSlug: "a/b"},
			NodeName: "(unknown node)", NodeSlug: "gone"},
	}, "for this owner")
	if !strings.Contains(out, "(unknown node)") || !strings.Contains(out, "gone") {
		t.Errorf("a binding whose node is not in the cache must still be identifiable:\n%s", out)
	}
}

// TestFormatBindingRowsAndResolve_LongPathsPassThroughVerbatim: a binding path is
// an address, so it is never shortened — same contract as the tree renderer.
func TestFormatBindingRowsAndResolve_LongPathsPassThroughVerbatim(t *testing.T) {
	longPath := "/Users/dev/" + strings.Repeat("sehr/tief/verschachtelt/", 12) + "repo"
	out := formatBindingRows([]bindingRow{
		{Binding: domain.ProjectBinding{ID: "b1", NodeID: "r1", Kind: domain.BindingPath,
			MachineID: "m1", MachineLabel: "notebook-a", Path: longPath}, NodeName: "Jukebox", NodeSlug: "jukebox"},
	}, "for node jukebox")
	if !strings.Contains(out, longPath) {
		t.Errorf("a long binding path must not be truncated:\n%s", out)
	}

	resolved := formatResolvedTarget("path "+longPath,
		domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo},
		domain.Node{}, false)
	if !strings.Contains(resolved, longPath) {
		t.Errorf("resolve must echo the full target:\n%s", resolved)
	}
}

func TestFormatResolvedTarget(t *testing.T) {
	node := domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo}
	eng := domain.Node{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement}

	withEng := formatResolvedTarget("path /work/jukebox", node, eng, true)
	for _, want := range []string{"/work/jukebox", "resolves to", "Jukebox", "jukebox", "r1", "Alpha", "alpha"} {
		if !strings.Contains(withEng, want) {
			t.Errorf("resolve result missing %q in: %s", want, withEng)
		}
	}
	without := formatResolvedTarget("remote a/b", node, domain.Node{}, false)
	if !strings.Contains(without, "No engagement") {
		t.Errorf("missing engagement must be stated, got: %s", without)
	}
}
```

- [ ] **Step 3: Renderer-Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestFormatBindingRows|TestFormatResolvedTarget' 2>&1 | head -15` → FAIL mit `undefined: bindingRow`, `undefined: formatBindingRows`, `undefined: formatResolvedTarget`.

- [ ] **Step 4: Renderer an `format_nodes.go` anfügen** — am Ende der Datei:

```go
// bindingRow is one binding plus the identity of the node it points at, so the
// renderer never has to look anything up itself.
type bindingRow struct {
	Binding  domain.ProjectBinding
	NodeName string
	NodeSlug string
}

// formatBindingRows renders the binding list. Every path binding names its
// machine LABEL and machine ID, because ListBindings is owner-scoped and returns
// the bindings of ALL of this owner's devices — without the machine an agent
// confuses notebook A with notebook B (Spec §3 flow_node_binding).
func formatBindingRows(rows []bindingRow, label string) string {
	if len(rows) == 0 {
		return "No bindings " + label + "."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d binding(s) %s:\n", len(rows), label)
	for _, r := range rows {
		if r.Binding.Kind == domain.BindingRemote {
			fmt.Fprintf(&b, "- remote %s → %s (%s)\n", r.Binding.RemoteSlug, r.NodeName, r.NodeSlug)
			continue
		}
		fmt.Fprintf(&b, "- path %s on machine %s [%s] → %s (%s)\n",
			r.Binding.Path, r.Binding.MachineLabel, r.Binding.MachineID, r.NodeName, r.NodeSlug)
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatResolvedTarget reports which node a target currently resolves to, plus
// its engagement, without binding anything.
func formatResolvedTarget(target string, node, engagement domain.Node, engagementOK bool) string {
	line := fmt.Sprintf("%s resolves to %s %q (%s), id %s.", target, node.Kind, node.Name, node.Slug, node.ID)
	if engagementOK {
		return line + fmt.Sprintf(" Engagement: %s (%s).", engagement.Name, engagement.Slug)
	}
	return line + " No engagement in its ancestor chain."
}
```

- [ ] **Step 5: Failing Handler-Test schreiben** — an `cmd/flow-mcp/tools_bindings_test.go` anfügen. Der Fake-Backend aus Task 3 wird um `GET /api/v1/nodes/bindings`, `GET /api/v1/nodes/resolve` und `GET /api/v1/nodes/resolve-engagement` erweitert; diese drei Routen zuerst in `fakeBindingBackend` ergänzen:

```go
	mux.HandleFunc("GET /api/v1/nodes/bindings", func(w http.ResponseWriter, _ *http.Request) {
		// Owner-wide across ALL devices — two machines, two nodes.
		_ = json.NewEncoder(w).Encode([]domain.ProjectBinding{
			{ID: "b1", NodeID: "r1", Kind: domain.BindingPath, MachineID: "m1", MachineLabel: "notebook-a", Path: "/work/jukebox"},
			{ID: "b2", NodeID: "e1", Kind: domain.BindingPath, MachineID: "m2", MachineLabel: "notebook-b", Path: "/work/alpha"},
			{ID: "b3", NodeID: "r1", Kind: domain.BindingRemote, RemoteSlug: "github.com/serverkraken/jukebox"},
		})
	})
	mux.HandleFunc("GET /api/v1/nodes/resolve", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("slug") == "github.com/serverkraken/jukebox" {
			_ = json.NewEncoder(w).Encode(nodes[2]) // r1/jukebox
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("GET /api/v1/nodes/resolve-engagement", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("slug") == "github.com/serverkraken/jukebox" {
			_ = json.NewEncoder(w).Encode(nodes[0]) // e1/alpha
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
```

Dann die Tests anfügen:

```go
func TestValidateNodeBinding(t *testing.T) {
	cases := []struct {
		name    string
		in      nodeBindingIn
		wantErr string
	}{
		{"bind with node", nodeBindingIn{Action: "bind", Node: "jukebox"}, ""},
		{"bind without node", nodeBindingIn{Action: "bind"}, `needs "node"`},
		{"unbind without node", nodeBindingIn{Action: "unbind"}, ""},
		{"unbind with node", nodeBindingIn{Action: "unbind", Node: "jukebox"}, "by its target only"},
		{"resolve without node", nodeBindingIn{Action: "resolve"}, ""},
		{"resolve with node", nodeBindingIn{Action: "resolve", Node: "jukebox"}, "by its target only"},
		{"list without node", nodeBindingIn{Action: "list"}, ""},
		{"list with node is a filter", nodeBindingIn{Action: "list", Node: "jukebox"}, ""},
		{"unknown action", nodeBindingIn{Action: "attach"}, "invalid action"},
		{"missing action", nodeBindingIn{}, "invalid action"},
		{"kind on bind", nodeBindingIn{Action: "bind", Node: "jukebox", Kind: "path"}, ""},
		{"kind on unbind", nodeBindingIn{Action: "unbind", Kind: "path"}, ""},
		{"kind on resolve", nodeBindingIn{Action: "resolve", Kind: "path"}, `does not take "kind"`},
		{"kind on list", nodeBindingIn{Action: "list", Kind: "remote"}, `does not take "kind"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := validateNodeBinding(c.in)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("validateNodeBinding(%#v) = %v, want nil", c.in, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("validateNodeBinding(%#v) = %v, want an error containing %q", c.in, err, c.wantErr)
			}
		})
	}
}

func TestValidateNodeBinding_InvalidActionListsThem(t *testing.T) {
	_, err := validateNodeBinding(nodeBindingIn{Action: "attach"})
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range nodeBindingActions {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must list the valid action %q", err.Error(), want)
		}
	}
}

func TestLoopback_NodeBinding_Advertised(t *testing.T) {
	sess, _ := authedBindingServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(tools.Tools, "flow_node_binding") {
		t.Fatalf("flow_node_binding not advertised; got %v", toolNames(tools.Tools))
	}
}

func TestLoopback_NodeBinding_BindAttachesTheTargetToTheNode(t *testing.T) {
	sess, rec := authedBindingServer(t)
	dir := t.TempDir()

	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "bind", "node": "jukebox", "path": dir, "kind": "path",
	})
	if res.IsError {
		t.Fatalf("bind errored: %s", out)
	}
	got := rec.snapshot()
	if got.bindCalls != 1 || got.bindNodeID != "r1" || got.bindPath != dir {
		t.Fatalf("recorder = %+v, want one path bind of %q on r1", got, dir)
	}
}

func TestLoopback_NodeBinding_UnbindAddressesTheTargetAndRejectsNode(t *testing.T) {
	sess, rec := authedBindingServer(t)

	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "unbind", "remote": "github.com/serverkraken/jukebox",
	})
	if res.IsError {
		t.Fatalf("unbind errored: %s", out)
	}
	got := rec.snapshot()
	if got.unbindCalls != 1 {
		t.Fatalf("unbindCalls = %d, want 1", got.unbindCalls)
	}
	if !strings.Contains(got.unbindQuery, "kind=remote") || !strings.Contains(got.unbindQuery, "jukebox") {
		t.Fatalf("unbind query = %q, want kind=remote plus the slug", got.unbindQuery)
	}

	// A node argument is a hard error: the apiclient unbind calls take none, so
	// passing one would be silently ignored.
	resNode, outNode := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "unbind", "node": "jukebox", "remote": "github.com/serverkraken/jukebox",
	})
	if !resNode.IsError || !strings.Contains(outNode, "by its target only") {
		t.Fatalf("unbind with node = (IsError=%v, %q), want a rejection", resNode.IsError, outNode)
	}
	if got := rec.snapshot(); got.unbindCalls != 1 {
		t.Fatalf("unbindCalls = %d, want still 1 (the rejected call must not reach the server)", got.unbindCalls)
	}
}

func TestLoopback_NodeBinding_ListShowsEveryDeviceWithItsMachine(t *testing.T) {
	sess, _ := authedBindingServer(t)

	res, out := callText(t, sess, "flow_node_binding", map[string]any{"action": "list"})
	if res.IsError {
		t.Fatalf("list errored: %s", out)
	}
	for _, want := range []string{"3 binding", "notebook-a", "m1", "notebook-b", "m2",
		"/work/jukebox", "/work/alpha", "Jukebox", "Alpha"} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing %q in:\n%s", want, out)
		}
	}
}

func TestLoopback_NodeBinding_ListWithNodeFiltersClientSide(t *testing.T) {
	sess, _ := authedBindingServer(t)

	res, out := callText(t, sess, "flow_node_binding", map[string]any{"action": "list", "node": "jukebox"})
	if res.IsError {
		t.Fatalf("filtered list errored: %s", out)
	}
	if !strings.Contains(out, "2 binding") {
		t.Fatalf("filtered list = %q, want the 2 bindings of r1 only", out)
	}
	if strings.Contains(out, "/work/alpha") || strings.Contains(out, "notebook-b") {
		t.Fatalf("filter leaked another node's binding:\n%s", out)
	}
}

func TestLoopback_NodeBinding_ListNeedsNoFilesystem(t *testing.T) {
	sess, _ := authedBindingServer(t)
	// A path that does not exist must NOT break list: list reports what the
	// server knows and never touches the filesystem.
	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "list", "path": "/definitely/not/here",
	})
	if res.IsError {
		t.Fatalf("list must ignore a target argument, got IsError: %s", out)
	}
	if !strings.Contains(out, "3 binding") {
		t.Fatalf("list = %q, want all three bindings", out)
	}
}

// TestLoopback_NodeBinding_ResolveRejectsKind: resolve reports what the server's
// resolution chain already decided, and that chain prefers a remote binding over
// a path binding (domain.ResolveBinding). A kind argument would read like an
// override and change nothing, so it is a hard error rather than a silent no-op.
func TestLoopback_NodeBinding_ResolveRejectsKind(t *testing.T) {
	sess, _ := authedBindingServer(t)
	for _, action := range []string{"resolve", "list"} {
		res, out := callText(t, sess, "flow_node_binding", map[string]any{
			"action": action, "kind": "path",
		})
		if !res.IsError || !strings.Contains(out, `does not take "kind"`) {
			t.Errorf("%s with kind = (IsError=%v, %q), want a rejection", action, res.IsError, out)
		}
	}
}

func TestLoopback_NodeBinding_ResolveReportsNodeAndEngagementWithoutBinding(t *testing.T) {
	sess, rec := authedBindingServer(t)

	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "resolve", "remote": "github.com/serverkraken/jukebox",
	})
	if res.IsError {
		t.Fatalf("resolve errored: %s", out)
	}
	for _, want := range []string{"resolves to", "Jukebox", "jukebox", "Alpha"} {
		if !strings.Contains(out, want) {
			t.Errorf("resolve missing %q in: %s", want, out)
		}
	}
	got := rec.snapshot()
	if got.bindCalls != 0 || got.unbindCalls != 0 {
		t.Fatalf("recorder = %+v, want resolve to mutate nothing", got)
	}
}

func TestLoopback_NodeBinding_ResolveWithNothingBoundSaysSoAndSuggestsBind(t *testing.T) {
	sess, _ := authedBindingServer(t)

	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "resolve", "remote": "github.com/serverkraken/unbound",
	})
	if res.IsError {
		t.Fatalf("an unresolved target is a normal answer, not an error: %s", out)
	}
	if !strings.Contains(out, "Nothing is bound") || !strings.Contains(out, "bind") {
		t.Fatalf("result = %q, want it to state the miss and suggest binding", out)
	}
}

func TestLoopback_NodeBinding_ResolveRejectsNode(t *testing.T) {
	sess, _ := authedBindingServer(t)
	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "resolve", "node": "jukebox", "remote": "github.com/serverkraken/jukebox",
	})
	if !res.IsError || !strings.Contains(out, "by its target only") {
		t.Fatalf("resolve with node = (IsError=%v, %q), want a rejection", res.IsError, out)
	}
}

func TestLoopback_NodeBinding_BindWithoutNodeIsAnError(t *testing.T) {
	sess, rec := authedBindingServer(t)
	res, out := callText(t, sess, "flow_node_binding", map[string]any{"action": "bind", "path": t.TempDir()})
	if !res.IsError || !strings.Contains(out, `needs "node"`) {
		t.Fatalf("bind without node = (IsError=%v, %q), want a rejection", res.IsError, out)
	}
	if got := rec.snapshot(); got.bindCalls != 0 {
		t.Fatalf("bindCalls = %d, want 0", got.bindCalls)
	}
}

// TestLoopback_NodeBinding_ServerRejectsAnUnbindableKind covers the error path
// the client deliberately does NOT pre-check: whether a node may carry a binding
// at all depends on its kind AND on whether it is a leaf, which only the server
// knows (usecase.ErrInvalidBindTarget → 400, internal/adapter/httpserver/projectbindings.go:101).
// The message must reach the model instead of a bare status.
func TestLoopback_NodeBinding_ServerRejectsAnUnbindableKind(t *testing.T) {
	e1, v1 := "e1", "v1"
	nodes := []domain.Node{
		{ID: "e1", Name: "Alpha", Slug: "alpha", Kind: domain.KindEngagement, Status: domain.NodeActive},
		{ID: "v1", Name: "Rebuild", Slug: "rebuild", Kind: domain.KindVorhaben, ParentID: &e1, Status: domain.NodeActive},
		{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo, ParentID: &v1, Status: domain.NodeActive},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(nodes)
	})
	mux.HandleFunc("PUT /api/v1/nodes/{id}/bindings", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "binding target has the wrong kind (remote→repo, path→repo or leaf vorhaben)", http.StatusBadRequest)
	})
	be := httptest.NewServer(mux)
	t.Cleanup(be.Close)

	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "r1", Name: "Jukebox", Slug: "jukebox", Kind: domain.KindRepo})
	sess := connect(t, h.srv)

	res, out := callText(t, sess, "flow_node_binding", map[string]any{
		"action": "bind", "node": "alpha", "path": t.TempDir(), "kind": "path",
	})
	if !res.IsError {
		t.Fatalf("binding an engagement: want IsError, got %q", out)
	}
	if !strings.Contains(out, "wrong kind") {
		t.Fatalf("error = %q, want the server's bind-target message verbatim", out)
	}
}

func TestLoopback_NodeBinding_InvalidActionListsTheValidOnes(t *testing.T) {
	sess, _ := authedBindingServer(t)
	res, out := callText(t, sess, "flow_node_binding", map[string]any{"action": "attach"})
	if !res.IsError {
		t.Fatalf("invalid action: want IsError, got %q", out)
	}
	for _, want := range []string{"bind", "unbind", "list", "resolve"} {
		if !strings.Contains(out, want) {
			t.Errorf("error %q must list the valid action %q", out, want)
		}
	}
}
```

- [ ] **Step 6: Handler-Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestValidateNodeBinding|TestLoopback_NodeBinding' 2>&1 | head -20` → FAIL mit `undefined: nodeBindingIn`, `undefined: validateNodeBinding`, `undefined: nodeBindingActions`.

- [ ] **Step 7: `flow_node_binding` an `tools_bindings.go` anfügen** — am Ende der Datei:

```go
// nodeBindingActions is the action whitelist; every error message lists it.
var nodeBindingActions = []string{"bind", "unbind", "list", "resolve"}

// nodeBindingIn manages the bindings that map a directory or a git remote to a
// node. resolve is an action of this family rather than its own tool: it is the
// same question as list, asked from the other side (Spec §3).
type nodeBindingIn struct {
	Action string `json:"action" jsonschema:"bind, unbind, list, or resolve"`
	Node   string `json:"node,omitempty" jsonschema:"the node to bind to (id, slug, or name) — REQUIRED for bind, an optional filter for list, and REJECTED for unbind and resolve, which address a binding by its target alone"`
	Path   string `json:"path,omitempty" jsonschema:"an existing directory; ~ and relative paths resolve against the flow-mcp process. Mutually exclusive with remote; omit both to use the process's working directory. Ignored by action=list."`
	Remote string `json:"remote,omitempty" jsonschema:"a git clone URL or host/path slug; no local checkout needed. Mutually exclusive with path. Ignored by action=list."`
	Kind   string `json:"kind,omitempty" jsonschema:"binding kind override for bind and unbind only: 'remote' (git origin) or 'path' (this device); omit to auto-detect. Rejected for list and resolve, which report what the server already resolved."`
}

// validateNodeBinding checks the action and the node/action pairing, returning
// the trimmed action. unbind and resolve address a binding purely by its target:
// UnbindRemote, UnbindPath and ResolveNode take no node id at all
// (internal/adapter/apiclient/projectbindings.go:82,96,39), so a passed node
// would be silently ignored — which is why it is rejected instead.
func validateNodeBinding(in nodeBindingIn) (string, error) {
	action := strings.TrimSpace(in.Action)
	hasNode := strings.TrimSpace(in.Node) != ""
	switch action {
	case "bind":
		if !hasNode {
			return "", errGuard{errors.New(`action "bind" needs "node": the id, slug, or name of the node to bind the target to`)}
		}
	case "unbind", "resolve":
		if hasNode {
			return "", errGuard{fmt.Errorf(`action %q addresses a binding by its target only — drop "node" and pass "path" or "remote"`, action)}
		}
	case "list":
		// node is an optional client-side filter here.
	default:
		return "", errGuard{fmt.Errorf("invalid action %q; use one of: %s", action, strings.Join(nodeBindingActions, ", "))}
	}
	// kind steers WHICH binding is written or deleted, so it is meaningful only
	// for bind and unbind. resolve asks the server the same question every other
	// tool asks, and that chain is remote-first by definition
	// (domain.ResolveBinding, internal/domain/projectbinding.go:31-38): a remote
	// match wins over a path match. Accepting kind here would look like an
	// override and change nothing — so it is rejected rather than ignored.
	if strings.TrimSpace(in.Kind) != "" && (action == "resolve" || action == "list") {
		return "", errGuard{fmt.Errorf(`action %q does not take "kind": it reports what the server already resolved, and that chain always prefers a remote binding over a path binding`, action)}
	}
	return action, nil
}

// nodeBinding runs one of the four binding actions.
func (h *handlers) nodeBinding(ctx context.Context, req *mcp.CallToolRequest, in nodeBindingIn) (*mcp.CallToolResult, any, error) {
	action, err := validateNodeBinding(in)
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	// list never touches the filesystem: it reports what the server already
	// knows, so a stale or absent path argument must not make it fail.
	var tgt bindTarget
	if action != "list" {
		env, eerr := liveBindEnv()
		if eerr != nil {
			return h.resultErr(eerr), nil, nil
		}
		tgt, eerr = resolveBindTarget(bindTargetArgs{Path: in.Path, Remote: in.Remote, Kind: in.Kind}, env)
		if eerr != nil {
			return h.resultErr(eerr), nil, nil
		}
	}
	var out string
	derr := h.do(ctx, req, func(c *apiclient.Client) error {
		switch action {
		case "bind":
			node, lerr := h.lookupNode(ctx, strings.TrimSpace(in.Node))
			if lerr != nil {
				return lerr
			}
			if berr := bindTargetTo(ctx, c, node.ID, tgt); berr != nil {
				return berr
			}
			h.refreshResolved(ctx, c)
			out = fmt.Sprintf("Bound %s to %s %q (%s) via %s binding.",
				bindTargetLabel(tgt), node.Kind, node.Name, node.Slug, tgt.Kind)
		case "unbind":
			if uerr := unbindTarget(ctx, c, tgt); uerr != nil {
				return uerr
			}
			h.refreshResolved(ctx, c)
			out = fmt.Sprintf("Unbound %s (%s binding).", bindTargetLabel(tgt), tgt.Kind)
		case "list":
			rows, rerr := h.bindingRows(ctx, c, strings.TrimSpace(in.Node))
			if rerr != nil {
				return rerr
			}
			label := "for this owner across all devices"
			if ref := strings.TrimSpace(in.Node); ref != "" {
				label = "for node " + ref
			}
			out = formatBindingRows(rows, label)
		default: // resolve
			node, ok, rerr := c.ResolveNode(ctx, tgt.RemoteSlug, tgt.MachineID, tgt.Path)
			if rerr != nil {
				return rerr
			}
			if !ok {
				out = fmt.Sprintf(`Nothing is bound to %s. Bind it with flow_node_binding action="bind".`, bindTargetLabel(tgt))
				return nil
			}
			eng, engOK, eerr := c.ResolveEngagement(ctx, tgt.RemoteSlug, tgt.MachineID, tgt.Path)
			if eerr != nil {
				return eerr
			}
			out = formatResolvedTarget(bindTargetLabel(tgt), node, eng, engOK)
		}
		return nil
	})
	if derr != nil {
		return h.resultErr(derr), nil, nil
	}
	return textResult(out), nil, nil
}

// bindingRows fetches this owner's bindings and joins each to its node's
// identity. The optional nodeRef filter is applied client-side because
// ListBindings takes no filter parameters
// (internal/adapter/apiclient/projectbindings.go:103).
func (h *handlers) bindingRows(ctx context.Context, c *apiclient.Client, nodeRef string) ([]bindingRow, error) {
	bs, err := c.ListBindings(ctx)
	if err != nil {
		return nil, err
	}
	if nodeRef != "" {
		node, lerr := h.lookupNode(ctx, nodeRef)
		if lerr != nil {
			return nil, lerr
		}
		bs = bindingsForNode(bs, node.ID)
	}
	nodes, err := h.nodeList(ctx, false)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]domain.Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	rows := make([]bindingRow, 0, len(bs))
	for _, b := range bs {
		// A binding whose node is missing from the cache stays visible and
		// identifiable by its id rather than silently rendering blank.
		row := bindingRow{Binding: b, NodeName: "(unknown node)", NodeSlug: b.NodeID}
		if n, ok := byID[b.NodeID]; ok {
			row.NodeName, row.NodeSlug = n.Name, n.Slug
		}
		rows = append(rows, row)
	}
	return rows, nil
}
```

Die Imports von `tools_bindings.go` ergänzen: `errors`, `strings` und `domain` kommen hinzu (`context`, `fmt`, `mcp`, `apiclient` sind schon da).

- [ ] **Step 8: Tool registrieren** — in `cmd/flow-mcp/server.go` nach dem `flow_set_node_tags`-Block:

```go
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_node_binding",
		Description: "Manage the bindings that map a directory or a git remote to a node. action=bind attaches the target to node (required) — a target can only be bound to ONE node, so binding an already-bound target MOVES it to the new node; check with action=resolve first. action=unbind detaches the target (a binding is addressed by its target alone, so do NOT pass node); action=list shows this owner's bindings across ALL devices with the machine label per path binding, optionally filtered by node; action=resolve reports which node a target currently resolves to, plus its engagement, without binding anything (no node here either). Address the target with path (a directory that must exist; ~ and relative paths resolve against the flow-mcp process) or remote (a clone URL or host/path slug, no local checkout needed); omit both for the process's working directory.",
	}, h.nodeBinding)
```

- [ ] **Step 9: Tool-Zahl auf 31 ziehen** — in `cmd/flow-mcp/loopback_test.go` bei `:353-355` die `30` durch `31` ersetzen (Bedingung und Meldung).

- [ ] **Step 10: Tests laufen lassen, grün bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestFormatBindingRows|TestFormatResolvedTarget|TestValidateNodeBinding|TestLoopback_NodeBinding' -v 2>&1 | tail -35` → PASS. Dann `go test ./cmd/flow-mcp/ 2>&1 | tail -5` → ok.

- [ ] **Step 11: Commit** — `git add cmd/flow-mcp/tools_bindings.go cmd/flow-mcp/tools_bindings_test.go cmd/flow-mcp/format_nodes.go cmd/flow-mcp/format_nodes_test.go cmd/flow-mcp/server.go cmd/flow-mcp/loopback_test.go && git commit -m "feat(flow-mcp): add flow_node_binding with bind, unbind, list and resolve"`

---

### Task 10: Wiring-Gate — vollständige Tool-Fläche, Schema-Smoke, Loopback-Kette

Der Task, ohne den dieser Plan Usecases shippen könnte, die niemand ruft. Hier entsteht **ein benannter Test**, auf den ein Reviewer zeigen kann: alle 31 Tools werden angeboten, die sechs neuen `inputSchema`s haben die versprochenen Felder, und die Kette aus Spec §7 läuft end-to-end gegen ein echtes `httptest`-Backend.

**Files:**
- Create: `cmd/flow-mcp/loopback_nodes_test.go`
- Modify: keine Produktionsdatei (dieser Task deckt Wiring-Fehler auf; findet er einen, wird er im betroffenen Task-Commit gefixt)

**Interfaces:**
- Consumes: `connect(t *testing.T, srv *mcp.Server) *mcp.ClientSession`, `managerFor(t *testing.T, client *apiclient.Client, proj domain.Node) (*authManager, *handlers)`, `callText(t *testing.T, sess *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, string)`, `hasTool([]*mcp.Tool, string) bool`, `toolNames([]*mcp.Tool) []string` — alle `cmd/flow-mcp/loopback_test.go`.
- Consumes zusätzlich: `degradedSession(t *testing.T) *mcp.ClientSession` (`cmd/flow-mcp/loopback_test.go:69`), `domain.ResolveBinding(bs []domain.ProjectBinding, remoteSlug, machineID, cwd string) (domain.ProjectBinding, bool)` (`internal/domain/projectbinding.go:31`).
- Produces: `func TestLoopback_NodeToolSurfaceIsComplete(t *testing.T)`, `func TestLoopback_NewNodeToolSchemas(t *testing.T)`, `func TestLoopback_NodeTools_DegradedRequireLogin(t *testing.T)`, `func TestLoopback_NodeManagementChain(t *testing.T)`, `func fakeNodeChainBackend(t *testing.T) *httptest.Server`, `func authedNodeChainServer(t *testing.T) *mcp.ClientSession`, `func tagsOf(slugs []string) []domain.Tag`, `var newNodeTools []string`.

- [ ] **Step 1: Bestand verifizieren** — diese drei Befehle laufen lassen:

```bash
rg -n "func (connect|managerFor|callText|hasTool|toolNames|degradedSession|text)\(" cmd/flow-mcp/loopback_test.go
rg -n "InputSchema" cmd/flow-mcp/tools_artifacts_test.go
rg -n "func ResolveBinding" -A 26 internal/domain/projectbinding.go
```

Erwartet: die sieben Test-Helfer (`connect`, `managerFor`, `callText`, `hasTool`, `toolNames`, `degradedSession`, `text`) mit den Signaturen aus dem Interfaces-Block; das Schema-Zugriffsmuster `tool.InputSchema.(map[string]any)` → `schema["properties"].(map[string]any)`; und `ResolveBinding(bs []domain.ProjectBinding, remoteSlug, machineID, cwd string) (domain.ProjectBinding, bool)` — der Chain-Fixture ruft es, damit sein `resolve` genau so priorisiert wie der echte Server (Remote vor Pfad), statt eine eigene Auflösung zu erfinden.

- [ ] **Step 2: Failing test schreiben** — neue Datei `cmd/flow-mcp/loopback_nodes_test.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// newNodeTools is the complete set this slice adds. The surface test asserts all
// of them, so forgetting a registration in server.go can never pass review.
var newNodeTools = []string{
	"flow_create_node", "flow_move_node", "flow_delete_node",
	"flow_get_node", "flow_set_node_tags", "flow_node_binding",
}

// TestLoopback_NodeToolSurfaceIsComplete is the wiring gate: 31 tools, every new
// name advertised, and the two changed tools still present.
func TestLoopback_NodeToolSurfaceIsComplete(t *testing.T) {
	sess := authedNodeChainServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 31 {
		t.Fatalf("tool count = %d, want 31 (25 before this slice + 6 new); got %v",
			len(tools.Tools), toolNames(tools.Tools))
	}
	for _, name := range newNodeTools {
		if !hasTool(tools.Tools, name) {
			t.Errorf("%s is implemented but NOT registered in server.go; got %v", name, toolNames(tools.Tools))
		}
	}
	for _, name := range []string{"flow_list_projects", "flow_bind_project", "flow_update_node"} {
		if !hasTool(tools.Tools, name) {
			t.Errorf("%s must survive this slice; got %v", name, toolNames(tools.Tools))
		}
	}
}

// TestLoopback_NewNodeToolSchemas is the schema smoke over all six new tools:
// the properties a caller needs must be present, and nothing may be required
// that the handler treats as optional.
func TestLoopback_NewNodeToolSchemas(t *testing.T) {
	sess := authedNodeChainServer(t)
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]*mcp.Tool, len(tools.Tools))
	for _, tool := range tools.Tools {
		byName[tool.Name] = tool
	}

	want := map[string]struct {
		props    []string
		notProps []string
	}{
		"flow_create_node":   {props: []string{"name", "kind", "parent", "description", "color", "glyph", "upstream", "counts_toward_target", "bind_path"}, notProps: []string{"slug", "icon"}},
		"flow_move_node":     {props: []string{"node", "parent", "to_root"}},
		"flow_delete_node":   {props: []string{"node", "confirm"}},
		"flow_get_node":      {props: []string{"node"}},
		"flow_set_node_tags": {props: []string{"node", "tags"}},
		"flow_node_binding":  {props: []string{"action", "node", "path", "remote", "kind"}},
		"flow_bind_project":  {props: []string{"project", "path", "remote", "kind"}, notProps: []string{"create_name", "create_parent"}},
	}
	for name, expect := range want {
		tool, ok := byName[name]
		if !ok {
			t.Errorf("%s not advertised", name)
			continue
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Errorf("%s: input schema type = %T, want map", name, tool.InputSchema)
			continue
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Errorf("%s: schema has no properties: %#v", name, schema)
			continue
		}
		for _, p := range expect.props {
			if props[p] == nil {
				t.Errorf("%s: schema is missing property %q", name, p)
			}
		}
		for _, p := range expect.notProps {
			if props[p] != nil {
				t.Errorf("%s: schema must NOT offer property %q", name, p)
			}
		}
	}

	// Optional-by-handler fields must not be schema-required.
	optional := map[string][]string{
		"flow_create_node":   {"parent", "bind_path", "counts_toward_target", "upstream"},
		"flow_move_node":     {"parent", "to_root"},
		"flow_delete_node":   {"confirm"},
		"flow_get_node":      {"node"},
		"flow_set_node_tags": {"node"},
		"flow_node_binding":  {"node", "path", "remote", "kind"},
		"flow_bind_project":  {"path", "remote", "kind"},
	}
	for name, fields := range optional {
		tool, ok := byName[name]
		if !ok {
			continue
		}
		schema, _ := tool.InputSchema.(map[string]any)
		required, _ := schema["required"].([]any)
		for _, raw := range required {
			for _, f := range fields {
				if raw == f {
					t.Errorf("%s: %q must not be schema-required (the handler treats it as optional)", name, f)
				}
			}
		}
	}
}

// TestLoopback_NodeTools_DegradedRequireLogin is the cross-cut the existing
// suite has for the read tools (loopback_test.go:634): a logged-out caller must
// get the actionable login message from EVERY new tool, never a silent success
// and never a confusing validation error. Arguments are chosen valid on purpose,
// so the only thing that can fail is authentication.
func TestLoopback_NodeTools_DegradedRequireLogin(t *testing.T) {
	sess := degradedSession(t)
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"flow_create_node", map[string]any{"name": "X", "kind": "engagement"}},
		{"flow_move_node", map[string]any{"node": "x", "to_root": true}},
		{"flow_delete_node", map[string]any{"node": "x"}},
		{"flow_get_node", map[string]any{"node": "x"}},
		{"flow_set_node_tags", map[string]any{"node": "x", "tags": []any{"a"}}},
		{"flow_node_binding", map[string]any{"action": "list"}},
		{"flow_bind_project", map[string]any{"project": "x", "remote": "github.com/a/b"}},
	} {
		res, got := callText(t, sess, tc.name, tc.args)
		if !res.IsError || !strings.Contains(got, "Login required") {
			t.Errorf("%s degraded = (IsError=%v, %q), want IsError + 'Login required'", tc.name, res.IsError, got)
		}
	}
}

// chainState is the mutable fixture the chain test drives: a real little node
// store with bindings, so the chain proves cause and effect rather than
// asserting against canned answers.
type chainState struct {
	mu       sync.Mutex
	nodes    []domain.Node
	bindings []domain.ProjectBinding
	tags     map[string][]string
	nextID   int
}

func (s *chainState) find(ref string) (domain.Node, bool) {
	for _, n := range s.nodes {
		if n.ID == ref || n.Slug == ref {
			return n, true
		}
	}
	return domain.Node{}, false
}

// fakeNodeChainBackend serves a small but honest node store: create, move,
// delete, tags, bindings, resolve, stats, ancestors, artifacts, documents.
func fakeNodeChainBackend(t *testing.T) *httptest.Server {
	t.Helper()
	st := &chainState{tags: map[string][]string{}, nextID: 1}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		st.mu.Lock()
		defer st.mu.Unlock()
		out := append([]domain.Node{}, st.nodes...)
		_ = json.NewEncoder(w).Encode(out)
	})
	create := func(f apiclient.CreateNodeFields) domain.Node {
		st.mu.Lock()
		defer st.mu.Unlock()
		id := fmt.Sprintf("n%d", st.nextID)
		st.nextID++
		n := domain.Node{
			ID: id, Name: f.Name, Slug: strings.ToLower(strings.ReplaceAll(f.Name, " ", "-")),
			Kind: domain.NodeKind(f.Kind), ParentID: f.ParentID, Status: domain.NodeActive,
			Description: f.Description, UpstreamGit: f.UpstreamGit, CountsTowardTarget: f.CountsTowardTarget,
		}
		st.nodes = append(st.nodes, n)
		return n
	}
	mux.HandleFunc("POST /api/v1/nodes", func(w http.ResponseWriter, r *http.Request) {
		var f apiclient.CreateNodeFields
		_ = json.NewDecoder(r.Body).Decode(&f)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(create(f))
	})
	mux.HandleFunc("POST /api/v1/nodes/create-bound", func(w http.ResponseWriter, r *http.Request) {
		var in apiclient.CreateBoundNodeInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		n := create(in.Node)
		st.mu.Lock()
		st.bindings = append(st.bindings, domain.ProjectBinding{
			ID: "b" + n.ID, NodeID: n.ID, Kind: domain.BindingKind(in.Binding.Kind),
			RemoteSlug: in.Binding.RemoteSlug, MachineID: in.Binding.MachineID,
			MachineLabel: in.Binding.MachineLabel, Path: in.Binding.Path,
		})
		st.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.CreateBoundNodeResult{Node: n})
	})
	mux.HandleFunc("POST /api/v1/nodes/{id}/move", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ParentID *string `json:"parentId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		st.mu.Lock()
		defer st.mu.Unlock()
		for i := range st.nodes {
			if st.nodes[i].ID == r.PathValue("id") {
				st.nodes[i].ParentID = body.ParentID
				_ = json.NewEncoder(w).Encode(st.nodes[i])
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		st.mu.Lock()
		defer st.mu.Unlock()
		for _, n := range st.nodes {
			if n.ParentID != nil && *n.ParentID == id {
				http.Error(w, "node has children; move or remove them first", http.StatusConflict)
				return
			}
		}
		kept := st.nodes[:0]
		for _, n := range st.nodes {
			if n.ID != id {
				kept = append(kept, n)
			}
		}
		st.nodes = kept
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /api/v1/nodes/{id}/bindings", func(w http.ResponseWriter, r *http.Request) {
		var f apiclient.BindingFields
		_ = json.NewDecoder(r.Body).Decode(&f)
		st.mu.Lock()
		defer st.mu.Unlock()
		b := domain.ProjectBinding{
			ID: "b" + f.Kind + f.Path + f.RemoteSlug, NodeID: r.PathValue("id"),
			Kind: domain.BindingKind(f.Kind), RemoteSlug: f.RemoteSlug,
			MachineID: f.MachineID, MachineLabel: f.MachineLabel, Path: f.Path,
		}
		// UPSERT on the target, not append: the real store conflicts on
		// (owner_id, remote_slug) resp. (owner_id, machine_id, path) and replaces
		// only node_id (internal/adapter/pgstore/projectbindings.go:49). A fake
		// that appended would hide the silent re-point this models.
		replaced := false
		for i := range st.bindings {
			cur := st.bindings[i]
			sameRemote := b.Kind == domain.BindingRemote && cur.Kind == domain.BindingRemote && cur.RemoteSlug == b.RemoteSlug
			samePath := b.Kind == domain.BindingPath && cur.Kind == domain.BindingPath && cur.MachineID == b.MachineID && cur.Path == b.Path
			if sameRemote || samePath {
				st.bindings[i].NodeID = b.NodeID
				replaced = true
				break
			}
		}
		if !replaced {
			st.bindings = append(st.bindings, b)
		}
		_ = json.NewEncoder(w).Encode(b)
	})
	mux.HandleFunc("DELETE /api/v1/nodes/bindings", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		st.mu.Lock()
		defer st.mu.Unlock()
		kept := st.bindings[:0]
		for _, b := range st.bindings {
			match := (q.Get("kind") == "remote" && b.Kind == domain.BindingRemote && b.RemoteSlug == q.Get("slug")) ||
				(q.Get("kind") == "path" && b.Kind == domain.BindingPath && b.MachineID == q.Get("machine") && b.Path == q.Get("path"))
			if !match {
				kept = append(kept, b)
			}
		}
		st.bindings = kept
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v1/nodes/bindings", func(w http.ResponseWriter, _ *http.Request) {
		st.mu.Lock()
		defer st.mu.Unlock()
		out := append([]domain.ProjectBinding{}, st.bindings...)
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("GET /api/v1/nodes/resolve", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		st.mu.Lock()
		bs := append([]domain.ProjectBinding{}, st.bindings...)
		st.mu.Unlock()
		if b, ok := domain.ResolveBinding(bs, q.Get("slug"), q.Get("machine"), q.Get("path")); ok {
			if n, found := st.find(b.NodeID); found {
				_ = json.NewEncoder(w).Encode(n)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("GET /api/v1/nodes/resolve-engagement", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		st.mu.Lock()
		bs := append([]domain.ProjectBinding{}, st.bindings...)
		st.mu.Unlock()
		b, ok := domain.ResolveBinding(bs, q.Get("slug"), q.Get("machine"), q.Get("path"))
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		cur, found := st.find(b.NodeID)
		for found && cur.ParentID != nil {
			cur, found = st.find(*cur.ParentID)
		}
		if !found || cur.Kind != domain.KindEngagement {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(cur)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/ancestors", func(w http.ResponseWriter, r *http.Request) {
		var chain []domain.Node
		cur, ok := st.find(r.PathValue("id"))
		for ok {
			chain = append(chain, cur) // leaf→root
			if cur.ParentID == nil {
				break
			}
			cur, ok = st.find(*cur.ParentID)
		}
		_ = json.NewEncoder(w).Encode(chain)
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/tags", func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		defer st.mu.Unlock()
		_ = json.NewEncoder(w).Encode(tagsOf(st.tags[r.PathValue("id")]))
	})
	mux.HandleFunc("PUT /api/v1/nodes/{id}/tags", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tags []string `json:"tags"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		st.mu.Lock()
		st.tags[r.PathValue("id")] = body.Tags
		st.mu.Unlock()
		_ = json.NewEncoder(w).Encode(tagsOf(body.Tags))
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/stats", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.NodeRollup{TotalMin: 750, WeekMin: 120, MonthMin: 300})
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Artifact{})
	})
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Document{})
	})
	mux.HandleFunc("GET /api/v1/nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		if n, ok := st.find(r.PathValue("id")); ok {
			_ = json.NewEncoder(w).Encode(n)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	return httptest.NewServer(mux)
}

func tagsOf(slugs []string) []domain.Tag {
	out := make([]domain.Tag, 0, len(slugs))
	for _, s := range slugs {
		out = append(out, domain.Tag{ID: "t-" + s, Slug: s, Display: s})
	}
	return out
}

func authedNodeChainServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	be := fakeNodeChainBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	_, h := managerFor(t, client, domain.Node{ID: "seed", Name: "Seed", Slug: "seed", Kind: domain.KindRepo})
	return connect(t, h.srv)
}

// TestLoopback_NodeManagementChain walks the exact chain from Spec §7: create an
// engagement, a vorhaben under it, a repo under that with bind_path, resolve it,
// list it with its machine, unbind, bind again to a foreign directory, read the
// detail, replace the tags, move the node, dry-run the delete, then delete.
func TestLoopback_NodeManagementChain(t *testing.T) {
	sess := authedNodeChainServer(t)
	repoDir := t.TempDir()
	foreignDir := t.TempDir()

	// 1. engagement (root, no parent)
	res, out := callText(t, sess, "flow_create_node", map[string]any{"name": "Chain Eng", "kind": "engagement"})
	if res.IsError {
		t.Fatalf("1 create engagement: %s", out)
	}

	// 2. vorhaben under it
	res, out = callText(t, sess, "flow_create_node", map[string]any{
		"name": "Chain Vor", "kind": "vorhaben", "parent": "chain-eng",
	})
	if res.IsError {
		t.Fatalf("2 create vorhaben: %s", out)
	}

	// 3. repo under the vorhaben, bound to a real directory in one atomic command
	res, out = callText(t, sess, "flow_create_node", map[string]any{
		"name": "Chain Repo", "kind": "repo", "parent": "chain-vor", "bind_path": repoDir,
	})
	if res.IsError {
		t.Fatalf("3 create bound repo: %s", out)
	}
	if !strings.Contains(out, repoDir) {
		t.Fatalf("3 result = %q, want it to name the bound directory", out)
	}

	// 4. resolve finds it through the path binding
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "resolve", "path": repoDir})
	if res.IsError {
		t.Fatalf("4 resolve: %s", out)
	}
	if !strings.Contains(out, "Chain Repo") || !strings.Contains(out, "Chain Eng") {
		t.Fatalf("4 resolve = %q, want the repo and its engagement", out)
	}

	// 5. list shows it with its machine
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "list", "node": "chain-repo"})
	if res.IsError {
		t.Fatalf("5 list: %s", out)
	}
	if !strings.Contains(out, repoDir) || !strings.Contains(out, "machine") {
		t.Fatalf("5 list = %q, want the path and its machine", out)
	}

	// 6. unbind that target
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "unbind", "path": repoDir})
	if res.IsError {
		t.Fatalf("6 unbind: %s", out)
	}
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "resolve", "path": repoDir})
	if res.IsError || !strings.Contains(out, "Nothing is bound") {
		t.Fatalf("6 resolve after unbind = (IsError=%v, %q), want 'Nothing is bound'", res.IsError, out)
	}

	// 7. bind again — to a DIFFERENT, foreign directory
	res, out = callText(t, sess, "flow_node_binding", map[string]any{
		"action": "bind", "node": "chain-repo", "path": foreignDir, "kind": "path",
	})
	if res.IsError {
		t.Fatalf("7 rebind: %s", out)
	}
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "resolve", "path": foreignDir})
	if res.IsError || !strings.Contains(out, "Chain Repo") {
		t.Fatalf("7 resolve foreign dir = (IsError=%v, %q), want the repo", res.IsError, out)
	}

	// 7b. re-binding an ALREADY-bound target MOVES it: the store upserts on
	// (owner_id, machine_id, path) and only replaces node_id
	// (internal/adapter/pgstore/projectbindings.go:49). This is silent by design
	// on the server, so the tool descriptions warn about it — and the chain
	// proves the behaviour rather than assuming a conflict.
	res, out = callText(t, sess, "flow_node_binding", map[string]any{
		"action": "bind", "node": "chain-vor", "path": foreignDir, "kind": "path",
	})
	if res.IsError {
		t.Fatalf("7b rebind to another node: %s", out)
	}
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "resolve", "path": foreignDir})
	if res.IsError || !strings.Contains(out, "Chain Vor") {
		t.Fatalf("7b resolve after rebind = (IsError=%v, %q), want the target moved to Chain Vor", res.IsError, out)
	}
	res, out = callText(t, sess, "flow_node_binding", map[string]any{"action": "list", "node": "chain-repo"})
	if res.IsError {
		t.Fatalf("7b list: %s", out)
	}
	if strings.Contains(out, foreignDir) {
		t.Fatalf("7b the moved target still shows on the old node:\n%s", out)
	}
	// Put it back so the rest of the chain reads naturally.
	res, out = callText(t, sess, "flow_node_binding", map[string]any{
		"action": "bind", "node": "chain-repo", "path": foreignDir, "kind": "path",
	})
	if res.IsError {
		t.Fatalf("7b rebind back: %s", out)
	}

	// 8. flow_get_node shows the ancestor chain
	res, out = callText(t, sess, "flow_get_node", map[string]any{"node": "chain-repo"})
	if res.IsError {
		t.Fatalf("8 get_node: %s", out)
	}
	if !strings.Contains(out, "Chain Eng / Chain Vor / Chain Repo") {
		t.Fatalf("8 get_node = %q, want the root→leaf breadcrumb", out)
	}

	// 9. flow_set_node_tags replaces the set
	res, out = callText(t, sess, "flow_set_node_tags", map[string]any{
		"node": "chain-repo", "tags": []any{"go", "audio"},
	})
	if res.IsError {
		t.Fatalf("9 set tags: %s", out)
	}
	res, out = callText(t, sess, "flow_set_node_tags", map[string]any{
		"node": "chain-repo", "tags": []any{"go"},
	})
	if res.IsError {
		t.Fatalf("9 replace tags: %s", out)
	}
	res, out = callText(t, sess, "flow_get_node", map[string]any{"node": "chain-repo"})
	if res.IsError {
		t.Fatalf("9 get_node after tags: %s", out)
	}
	if strings.Contains(out, "audio") {
		t.Fatalf("9 tags were added, not replaced: %q", out)
	}

	// 10. move the repo up to the engagement
	res, out = callText(t, sess, "flow_move_node", map[string]any{"node": "chain-repo", "parent": "chain-eng"})
	if res.IsError {
		t.Fatalf("10 move: %s", out)
	}
	res, out = callText(t, sess, "flow_get_node", map[string]any{"node": "chain-repo"})
	if res.IsError {
		t.Fatalf("10 get_node after move: %s", out)
	}
	if !strings.Contains(out, "Chain Eng / Chain Repo") {
		t.Fatalf("10 breadcrumb after move = %q, want the vorhaben gone from the chain", out)
	}

	// 11. dry run reports and deletes nothing
	res, out = callText(t, sess, "flow_delete_node", map[string]any{"node": "chain-repo"})
	if res.IsError {
		t.Fatalf("11 dry run: %s", out)
	}
	if !strings.Contains(out, "Would delete") || !strings.Contains(out, "confirm=true") {
		t.Fatalf("11 dry run = %q, want a report inviting confirm=true", out)
	}
	res, out = callText(t, sess, "flow_get_node", map[string]any{"node": "chain-repo"})
	if res.IsError {
		t.Fatalf("11 the node must still exist after a dry run: %s", out)
	}

	// 12. confirm deletes it
	res, out = callText(t, sess, "flow_delete_node", map[string]any{"node": "chain-repo", "confirm": true})
	if res.IsError {
		t.Fatalf("12 confirmed delete: %s", out)
	}
	res, out = callText(t, sess, "flow_get_node", map[string]any{"node": "chain-repo"})
	if !res.IsError {
		t.Fatalf("12 the node still resolves after deletion: %q", out)
	}

	// 13. the vorhaben still holds nothing and the tree renders it
	res, out = callText(t, sess, "flow_list_projects", map[string]any{})
	if res.IsError {
		t.Fatalf("13 list_projects: %s", out)
	}
	for _, want := range []string{"Chain Eng", "engagement", "Chain Vor", "vorhaben"} {
		if !strings.Contains(out, want) {
			t.Errorf("13 tree missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Chain Repo") {
		t.Errorf("13 tree still shows the deleted repo:\n%s", out)
	}
}
```

> Beachte: `flow_create_node` hat **kein** `kind`-Argument für das Binding (der Kind wird aus dem Verzeichnis abgeleitet) — nur `flow_bind_project` und `flow_node_binding` kennen `kind`. Wer hier `"kind"` mitschickt, bekommt vom SDK eine Schema-Verletzung.

- [ ] **Step 3: Test laufen lassen, Fehlschlag bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestLoopback_NodeToolSurfaceIsComplete|TestLoopback_NewNodeToolSchemas|TestLoopback_NodeTools_DegradedRequireLogin|TestLoopback_NodeManagementChain' -v 2>&1 | tail -40`. Der Fixture-Backend ist neu, also erst so lange am Fake arbeiten, bis der Fehlschlag **ausschließlich** aus echten Wiring- oder Verhaltenslücken besteht (ein Fake-Fehler zeigt sich als 404/500 in der Tool-Antwort, eine echte Lücke als falscher Text oder fehlendes Tool).

- [ ] **Step 4: Jeden verbleibenden Fehlschlag im ursächlichen Task-Commit fixen** — dieser Task ändert bewusst keine Produktionsdatei „nebenbei". Meldet der Surface-Test ein fehlendes Tool, gehört die `mcp.AddTool`-Zeile in `server.go`; meldet der Chain-Test ein falsches Verhalten, gehört der Fix in die Handler-Datei des betroffenen Tools. Jede solche Korrektur wird hier mit committet und im Commit-Text benannt.

- [ ] **Step 5: Test laufen lassen, grün bestätigen** — `go test ./cmd/flow-mcp/ -run 'TestLoopback_NodeToolSurfaceIsComplete|TestLoopback_NewNodeToolSchemas|TestLoopback_NodeTools_DegradedRequireLogin|TestLoopback_NodeManagementChain' -v 2>&1 | tail -20` → alle vier PASS. Dann das ganze Paket mit Race: `go test -race ./cmd/flow-mcp/ 2>&1 | tail -5` → ok.

- [ ] **Step 6: Leichen-Sweep** — diese fünf Befehle laufen lassen; jeder muss leer bleiben (bzw. nur die genannten Treffer liefern):

```bash
rg -n "create_name|CreateName|create_parent|CreateParent" cmd/flow-mcp/   # muss LEER sein
rg -n "formatProjects|validateBindRef" cmd/flow-mcp/                      # muss LEER sein
rg -n "kind_ignored" cmd/flow-mcp/                                        # muss LEER sein
rg -n "TODO|FIXME|XXX" cmd/flow-mcp/                                      # keine neuen Marker
go vet ./cmd/flow-mcp/                                                    # muss LEER sein
```

- [ ] **Step 7: Commit** — `git add cmd/flow-mcp/ && git commit -m "test(flow-mcp): gate the complete 31-tool surface, the new schemas and the node-management chain"`

---

### Task 11: Done-Gate

Der Task, nach dem „fertig" gesagt werden darf — und nicht vorher. Spec §8.

**Files:**
- Modify: keine Code-Datei
- Modify: das flow-Kontrakt-Doc `claude-code-flow-kontrakt` (über MCP, nicht im Repo)

**Interfaces:**
- Consumes: die vollständige, in Task 1-10 gebaute Tool-Fläche.
- Produces: ein grünes `make ci`, ein Live-Beweis gegen den Dev-Stack, ein nachgezogenes Kontrakt-Doc.

- [ ] **Step 1: `make ci` grün bekommen** — `make ci 2>&1 | tail -30`. Alle sechs Ziele müssen durchlaufen: `lint verify-generate verify-css verify-no-popups cover build`. Erwartete Besonderheiten: `verify-generate` und `verify-css` sind No-ops (dieser Slice ändert kein `.templ` und kein Tailwind); `cover` misst `./internal/...` und ist von diesem Slice unberührt, führt aber `go test ./...` aus und muss deshalb grün sein. Bei einem `golangci-lint`-Fund: fixen, **nie** `make fmt` laufen lassen.

- [ ] **Step 2: Binary bauen** — `go build ./cmd/flow-mcp && echo BUILD-OK`.

- [ ] **Step 3: Schema-Smoke isoliert nachweisen** — `go test ./cmd/flow-mcp/ -run 'TestLoopback_NewNodeToolSchemas|TestLoopback_NodeToolSurfaceIsComplete' -v 2>&1 | tail -15` → beide PASS mit `tool count = 31`.

- [ ] **Step 4: Dev-Stack starten** — `make dev-up`, dann in einem zweiten Terminal `make dev-run` (Server auf `https://localhost:8080`, selbstsigniert), dann `make dev-token` für ein Bearer-Token. Danach `flow login` bzw. das Token so ablegen, dass `flow-mcp` sich authentifizieren kann (dieselbe Prozedur wie bei den vorangegangenen MCP-Slices).

- [ ] **Step 5: Live-Gate durchspielen** — gegen den echten Server, mit dem gebauten `flow-mcp`, in dieser Reihenfolge. Jeder Schritt wird mit seiner Antwort protokolliert. **Die SSE-Gegenprobe aus Step 6 gehört zwischen 5.8 und 5.9** — das Aufräumen in 5.12 zerstört sonst die Objekte, deren Live-Wirkung zu beobachten ist. Nummerierung unten = 5.1 bis 5.12:

1. `flow_create_node(name="Live Eng", kind="engagement")` → angelegt, Root ohne Parent.
2. `flow_create_node(name="Live Vor", kind="vorhaben", parent="live-eng")` → angelegt.
3. `flow_create_node(name="Live Repo", kind="repo", parent="live-vor", bind_path="<ein echtes, FREMDES Verzeichnis>")` → angelegt UND gebunden; der Pfad ist bewusst **nicht** das cwd des MCP-Prozesses, denn genau das war die Lücke, die diesen Slice ausgelöst hat (Spec §1).
4. `flow_node_binding(action="resolve", path="<dasselbe Verzeichnis>")` → findet Live Repo und nennt Live Eng als Engagement.
5. `flow_node_binding(action="list")` → zeigt das Binding mit Maschinen-Label und Maschinen-ID.
6. `flow_node_binding(action="unbind", path="<dasselbe Verzeichnis>")` → gelöst; ein erneutes `resolve` sagt „Nothing is bound".
7. `flow_move_node(node="live-repo", parent="live-eng")` → umgehängt; `flow_get_node(node="live-repo")` zeigt die verkürzte Ahnenkette.
8. `flow_set_node_tags(node="live-repo", tags=["live"])` → resultierende Menge ist genau `live`.
9. `flow_delete_node(node="live-vor")` → Trockenlauf meldet „Cannot delete" nur, falls noch Kinder hängen; nach Schritt 7 ist Live Vor kinderlos, also ein normaler Bericht.
10. `flow_delete_node(node="live-repo", confirm=true)` → gelöscht.
11. `flow_list_projects()` → Baum ohne Live Repo, mit Live Eng und Live Vor, jede Zeile mit `kind`.
12. Abschließend `flow_delete_node(node="live-vor", confirm=true)` und `flow_delete_node(node="live-eng", confirm=true)` → der Live-Test räumt hinter sich auf.

- [ ] **Step 6: SSE-Gegenprobe — VOR dem Aufräumen, mit offener WebUI** — dieser Schritt läuft **zwischen Step 5.8 und Step 5.9**, nicht danach: nach dem Cleanup existiert nichts mehr, dessen Mutation man beobachten könnte. Die Nodes-/Cockpit-Seite im Browser offen halten und jede der vier emittierenden Mutationen einmal auslösen, jeweils ohne Reload:

| Aktion | Erwartetes Event | Quelle | Live-Wirkung |
|---|---|---|---|
| `flow_create_node(name="SSE Probe", kind="engagement")` | `node.created` | `worktime.go:237` | neuer Node erscheint in der Liste |
| `flow_create_node(kind="repo", parent="live-eng", bind_path=…)` | `node.created` | `projectbindings.go:54` (create-bound) | neuer Node erscheint |
| `flow_move_node(node="sse-probe", …)` bzw. der Move aus Step 5.7 | `node.moved` | `nodemove.go:37` | Baum hängt den Node um |
| `flow_set_node_tags(node="live-repo", tags=["live"])` aus Step 5.8 | `node.updated` | `nodetags.go:51` | Tag-Anzeige aktualisiert sich |
| `flow_delete_node(node="sse-probe", confirm=true)` | `node.deleted` | `worktime.go:294` | Node verschwindet aus der Liste |

Zusätzlich den zweiten Konsumenten prüfen: `flow` (TUI) im Nodetree-Screen offen halten und eine der Mutationen wiederholen — der Baum muss ohne Neustart nachziehen.

**Erwartete Ausnahme, kein Fehler:** `flow_node_binding` mit `action="bind"` bzw. `"unbind"` aktualisiert die Bindings-Anzeige NICHT live, weil `handleBindNode`/`handleUnbindNode` kein Event emittieren (`internal/adapter/httpserver/projectbindings.go:83,113`). Das ist die in Offene Entscheidungen #1 dokumentierte Bestandslücke — im Protokoll als bekannt vermerken, hier **nicht** fixen (das wäre Backend-Arbeit, die Spec §2 ausschließt). Der `SSE Probe`-Node wird in dieser Tabelle bereits wieder gelöscht und hinterlässt nichts.

- [ ] **Step 7: Kontrakt-Doc nachziehen** — das flow-instruction-Doc `claude-code-flow-kontrakt` beschreibt heute `flow_bind_project` als den Weg zum Anlegen. Über MCP: erst `flow_search_docs(query="claude-code-flow-kontrakt")`, dann `flow_get_doc`, dann `flow_patch_doc` (`operation="replace_section"`) auf den betroffenen Abschnitt. Der neue Text muss sagen: Anlegen ist `flow_create_node`; `flow_bind_project` bindet nur; Umhängen/Löschen/Detail/Tags/Bindings sind `flow_move_node`, `flow_delete_node`, `flow_get_node`, `flow_set_node_tags`, `flow_node_binding`. **Nicht im Repo committen** — das Doc lebt in flow.

- [ ] **Step 8: Backlog-Nachtrag** — die offenen Enden aus Spec §9 in `notes/backlog-offene-slices` (flow-Doc) ergänzen: Worktime per MCP, Day-offs und Settings per MCP, `ImportDocument` mit `path`, Logo-Upload (braucht erst eine `apiclient`-Methode), `NodeMRU` als Sortierhilfe für die Baum-Ausgabe, und Tilde-Expansion für `flow_upload_artifact`. **Nicht** in den Backlog gehören die fehlenden SSE-Events für bind/unbind — die schließt Task 12.

- [ ] **Step 9: Abschluss-Verifikation und Commit** — `make ci 2>&1 | tail -5` noch einmal grün, dann `git status --short` prüfen (es darf nichts Ungewolltes offen sein). Falls Task 11 Code berührt hat (etwa einen Lint-Fund aus Step 1): `git add -A && git commit -m "chore(flow-mcp): close the node-management done gate"`. Hat er nur Dokumentation außerhalb des Repos berührt, gibt es hier keinen Commit.

- [ ] **Step 10: Weiter zu Task 12** — der Slice selbst ist hier fertig. Es folgt **Task 12** (SSE für bind/unbind), Soennes Entscheidung vom 2026-07-25: nach dem Done-Gate, in einem eigenen Commit. **Merge** `node-mgmt` → `rebuild` erst danach und erst nach Soennes Review. Der Plan-Ausführende merged nicht selbst.

---

### Task 12: SSE für bind und unbind (nach dem Done-Gate, eigener Commit)

**Warum dieser Task existiert.** `PUT /api/v1/nodes/{id}/bindings` und `DELETE /api/v1/nodes/bindings` emittieren heute kein Event, während create, create-bound, move, delete und tags es tun. Bis zu diesem Slice fiel das kaum auf, weil Bindings selten geändert wurden; mit `flow_node_binding` werden sie zur Alltagsoperation, und die WebUI-Bindings-Anzeige würde ohne Reload veralten. CLAUDE.md macht das zur Regel: wer eine Mutation ergänzt, emittiert das passende Event, sonst aktualisiert die UI nicht live.

**Dies ist die zweite bewusste Ausnahme von „keine Backend-Änderung"** (siehe Global Constraints) und liegt deshalb hinter dem Done-Gate in einem eigenen Commit: die Tasks 1 bis 11 bleiben ein reiner MCP-Slice, und diese Änderung ist in Review und Historie klar davon getrennt.

**Warum `node.updated` und kein neuer Event-Typ.** `domain` kennt genau vier Node-Events — `node.created`, `node.updated`, `node.moved`, `node.deleted` (`internal/domain/event.go`). Ein Binding ist kein neuer Node und kein Umhängen, also bleibt `node.updated`. Das ist semantisch etwas grob, weil der Node selbst unverändert bleibt — aber die Konsumenten triggern ausschließlich auf den Event-**Typ** und holen ihr Fragment neu (`hx-trigger="sse:node.updated"` in `internal/adapter/webui/nodes.templ:23` und `cockpit.templ:40,50,59`), lesen also kein Feld aus `Data`. Ein fünfter Event-Typ würde jeden dieser vier `hx-trigger`-Listen einen Eintrag hinzufügen, ohne dass ein Konsument etwas davon hätte.

**Files:**
- Modify: `internal/adapter/httpserver/projectbindings.go` (`handleBindNode` default-Zweig `:105-107`, `handleUnbindNode` Erfolgspfad `:123-127`)
- Modify: `internal/adapter/httpserver/projectbindings_test.go` (zwei Tests anfügen)
- Modify: `internal/adapter/httpserver/webui_nodes_test.go` (Lese-Helfer `all()` auf `captureEmitter` anfügen)

**Interfaces:**
- Consumes: `s.Emitter.Emit(ctx context.Context, ev domain.Event)` (`internal/ports/ports.go:539-541`); `domain.Event{Type domain.EventType; UserID string; Data map[string]any}`; `domain.EventNodeUpdated` (`internal/domain/event.go`, Wert `"node.updated"`); das Test-Harness `newBindingsSrvFull(t) *bindingsSrv` mit Feld `emitter *captureEmitter` (`projectbindings_test.go:26,95`); `captureEmitter{mu sync.Mutex; events []domain.Event}` (`webui_nodes_test.go:28-37`).
- Produces: `func (e *captureEmitter) all() []domain.Event` — gesperrte Kopie der bisher emittierten Events, damit Tests sie ohne Datenrennen lesen können. Kein Produktionscode-Symbol; die zwei `Emit`-Aufrufe sind Verhalten, keine neue API.

- [ ] **Step 1: Bestand verifizieren** — diese Befehle:

```bash
rg -n "EventNode" internal/domain/event.go
rg -n -A6 "func \(s \*Server\) handleBindNode" internal/adapter/httpserver/projectbindings.go
rg -n -A6 "func \(s \*Server\) handleUnbindNode" internal/adapter/httpserver/projectbindings.go
rg -n "Emitter.Emit" internal/adapter/httpserver/nodetags.go internal/adapter/httpserver/nodemove.go
rg -n "type captureEmitter" -A 10 internal/adapter/httpserver/webui_nodes_test.go
rg -n "emitter" internal/adapter/httpserver/projectbindings_test.go
```

Erwartet: die vier `EventNode*`-Konstanten; `handleBindNode` schreibt im default-Zweig nur `writeJSON(w, http.StatusOK, b)`; `handleUnbindNode` schreibt nur `w.WriteHeader(http.StatusNoContent)`; die Nachbar-Handler emittieren im Muster `s.Emitter.Emit(r.Context(), domain.Event{Type: …, UserID: u.ID, Data: map[string]any{"id": id}})`; `captureEmitter` hat `mu` und `events`; das Binding-Harness hält `emitter` bereits als `*captureEmitter`. Weicht etwas ab, gewinnt der Bestand.

- [ ] **Step 2: Lese-Helfer und zwei fehlschlagende Tests schreiben** — zuerst an `internal/adapter/httpserver/webui_nodes_test.go` direkt hinter die `Emit`-Methode anfügen:

```go
// all returns a copy of the captured events under the lock, so assertions in
// tests never race with a concurrent Emit.
func (e *captureEmitter) all() []domain.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]domain.Event(nil), e.events...)
}
```

Dann an `internal/adapter/httpserver/projectbindings_test.go` anfügen:

```go
func TestBindNode_EmitsNodeUpdated(t *testing.T) {
	s := newBindingsSrvFull(t)
	if _, err := s.ps.Create(context.Background(), domain.Node{
		ID: "n1", OwnerID: "u1", Name: "Flow", Slug: "flow", Kind: domain.KindRepo,
	}); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	res := s.do(http.MethodPut, "/api/v1/nodes/n1/bindings",
		`{"kind":"remote","remoteSlug":"github.com/serverkraken/flow"}`)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	events := s.emitter.all()
	if len(events) != 1 {
		t.Fatalf("emitted %d event(s), want exactly 1: %+v", len(events), events)
	}
	if events[0].Type != domain.EventNodeUpdated {
		t.Errorf("event type = %q, want %q", events[0].Type, domain.EventNodeUpdated)
	}
	if events[0].UserID != "u1" {
		t.Errorf("event UserID = %q, want %q", events[0].UserID, "u1")
	}
	if got := events[0].Data["id"]; got != "n1" {
		t.Errorf("event Data[id] = %v, want %q", got, "n1")
	}
}

func TestUnbindNode_EmitsNodeUpdated(t *testing.T) {
	s := newBindingsSrvFull(t)
	if _, err := s.ps.Create(context.Background(), domain.Node{
		ID: "n1", OwnerID: "u1", Name: "Flow", Slug: "flow", Kind: domain.KindRepo,
	}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	bindRes := s.do(http.MethodPut, "/api/v1/nodes/n1/bindings",
		`{"kind":"remote","remoteSlug":"github.com/serverkraken/flow"}`)
	_ = bindRes.Body.Close()

	res := s.do(http.MethodDelete,
		"/api/v1/nodes/bindings?kind=remote&slug=github.com/serverkraken/flow", "")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}

	events := s.emitter.all()
	if len(events) != 2 {
		t.Fatalf("emitted %d event(s), want 2 (bind + unbind): %+v", len(events), events)
	}
	// The unbind handler is addressed by binding target, not by node — UnbindNode.
	// Execute returns only an error (internal/usecase/unbind_node.go), so the node
	// id is genuinely unavailable here. Consumers trigger on the event TYPE and
	// refetch, so an id-less node.updated still drives the live update.
	if events[1].Type != domain.EventNodeUpdated {
		t.Errorf("unbind event type = %q, want %q", events[1].Type, domain.EventNodeUpdated)
	}
	if events[1].UserID != "u1" {
		t.Errorf("unbind event UserID = %q, want %q", events[1].UserID, "u1")
	}
}
```

- [ ] **Step 3: Tests laufen lassen, Fehlschlag bestätigen** — `go test ./internal/adapter/httpserver/ -run 'TestBindNode_EmitsNodeUpdated|TestUnbindNode_EmitsNodeUpdated' -v 2>&1 | tail -20` → beide FAIL mit `emitted 0 event(s), want exactly 1` beziehungsweise `emitted 1 event(s), want 2 (bind + unbind)`. Ein Compile-Fehler an dieser Stelle heißt, der Helfer aus Step 2 fehlt.

- [ ] **Step 4: Die zwei `Emit`-Aufrufe ergänzen** — in `internal/adapter/httpserver/projectbindings.go`. Im default-Zweig von `handleBindNode`:

```go
	default:
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID, Data: map[string]any{"id": nodeID}})
		writeJSON(w, http.StatusOK, b)
	}
```

Und im Erfolgspfad von `handleUnbindNode`, direkt vor dem `WriteHeader`:

```go
	if err := s.UnbindNode.Execute(r.Context(), u.ID, key); err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	// Addressed by binding target, so the node id is not available here (see
	// usecase.UnbindNode). Consumers trigger on the event type and refetch.
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventNodeUpdated, UserID: u.ID})
	w.WriteHeader(http.StatusNoContent)
```

- [ ] **Step 5: Tests laufen lassen, grün bestätigen** — `go test ./internal/adapter/httpserver/ -run 'TestBindNode_EmitsNodeUpdated|TestUnbindNode_EmitsNodeUpdated' -v 2>&1 | tail -15` → beide PASS. Dann das ganze Paket mit Race, weil `captureEmitter` einen Mutex hat und viele Handler-Tests daneben laufen: `go test -race ./internal/adapter/httpserver/ 2>&1 | tail -5` → ok.

- [ ] **Step 6: `make ci` grün bestätigen** — `make ci 2>&1 | tail -20`. Dieser Task berührt `internal/`, also fließt er anders als die Tasks 1 bis 11 wirklich ins Coverage-Gate ein (`COVER_PKG := ./internal/...`); die zwei Tests deckeln die zwei neuen Zeilen ab. Bei einem `golangci-lint`-Fund fixen, **nie** `make fmt`.

- [ ] **Step 7: Commit**

```bash
git add internal/adapter/httpserver/projectbindings.go internal/adapter/httpserver/projectbindings_test.go internal/adapter/httpserver/webui_nodes_test.go
git commit -m "fix(httpserver): emit node.updated on bind and unbind so the WebUI live-updates"
```

- [ ] **Step 8: Live-Gegenprobe** — mit laufendem Dev-Stack aus Task 11 und offener Nodes-Seite im Browser: über `flow_node_binding` einmal `bind` und einmal `unbind` auslösen und beobachten, dass die Bindings-Anzeige **ohne Reload** nachzieht. Das ist der Beweis, den die Handler-Tests nicht führen können.

---

## Offene Entscheidungen

Alles hier war Soennes Wahl. **Die Punkte 1, 2 und 6 hat er am 2026-07-25 im Pre-Flight-Scan entschieden — die Entscheidung steht jeweils fett am Anfang, und der Plan ist entsprechend geändert.** Die übrigen Punkte setzt der Plan wie empfohlen um; eine andere Entscheidung ändert nur die genannte Stelle.

**1. bind/unbind emittieren kein SSE-Event (Bestandslücke). — ENTSCHIEDEN: schließen, direkt nach Task 11, eigener Commit → das ist jetzt Task 12.** Soenne hat die Alternative gewählt, nicht die Empfehlung: die Lücke wird geschlossen, aber außerhalb des Slices, damit die Tasks 1 bis 11 ein reiner MCP-Slice bleiben. Event-Typ ist `node.updated`, Begründung in Task 12.
`PUT /api/v1/nodes/{id}/bindings` und `DELETE /api/v1/nodes/bindings` rufen den `Emitter` nicht (`internal/adapter/httpserver/projectbindings.go:83,113`), während `create-bound`, create, move, delete und tags es tun. Folge: ein MCP-`bind`/`unbind` aktualisiert die Bindings-Anzeige der WebUI nicht live; die Seite braucht einen Reload.
*Empfehlung:* in diesem Slice **nicht** schließen. Spec §2 sagt „Keine Backend-Änderung", und das wäre eine — plus die Frage, welcher Event-Typ passt (`node.updated` wäre naheliegend, aber semantisch schief, denn der Node ändert sich nicht). *Trade-off:* Die Lücke wird durch diesen Slice sichtbarer, weil Bindings jetzt viel leichter zu ändern sind. Sie steht als Nachtrag im Backlog (Task 11, Step 8). *Alternative:* jetzt einen Zweizeiler ins Backend (zwei `Emit`-Aufrufe mit `node.updated`) — bricht die Slice-Grenze, kostet aber ~10 Minuten und macht das Live-Verhalten konsistent.

**2. `formatMinutes` dupliziert `wtfmt.FormatMin`. — ENTSCHIEDEN: `wtfmt` hochziehen → das ist jetzt Task 0.** Soenne hat die Alternative gewählt, nicht die Empfehlung: keine Duplikation, stattdessen `internal/tui/screen/worktime/wtfmt` → `internal/timefmt` als Leaf-Package, das TUI und MCP gleichermaßen importieren dürfen. Task 6 und Task 7 rufen `timefmt.FormatMin`; ein lokales `formatMinutes` existiert nicht mehr.
`internal/tui/screen/worktime/wtfmt/wtfmt.go:9` liefert bitgleich `"12h 30m"` und ist ein reines Leaf-Package (importiert nur `fmt`).
*Empfehlung:* die lokale Vierzeiler-Kopie in `format_nodes.go` (so im Plan, Task 6). Grund: `cmd/flow-mcp` → `internal/tui/...` wäre ein Adapter→Adapter-Import und dreht die hexagonale Abhängigkeitsrichtung (AGENTS.md „Architecture"). *Trade-off:* eine 4-Zeilen-Duplikation. *Alternative:* `wtfmt` nach `internal/timefmt` (oder `internal/domain`) hochziehen und beide Stellen darauf ziehen — sauberer, aber ein Refactor an einem TUI-Package, den dieser Slice nicht angekündigt hat.

**3. Kind-Glyphen in der Textausgabe.**
Der Plan setzt `● ◆ ⬡ ▶` (AGENTS.md-Satz) plus das Kind-Wort in jeder Zeile, also strenggenommen redundant.
*Empfehlung:* so lassen — Spec §3 verlangt ausdrücklich einen „Kind-Glyph", und das Kind-Wort daneben sorgt dafür, dass kein Modell eine Glyph-Legende braucht. *Trade-off:* etwas breitere Zeilen. *Alternative:* nur das Kind-Wort (schmaler, aber vom Spec-Wortlaut abweichend) oder nur den Glyph (kompakt, aber ein Modell muss raten).

**4. `flow_create_node` kann `icon` nicht setzen.**
`apiclient.CreateNodeFields` hat kein `Icon`-Feld (`internal/adapter/apiclient/nodes.go:27`), obwohl der Server-Request-Typ eins kennt.
*Empfehlung:* akzeptieren; `color` und `glyph` gehen, `icon` setzt man direkt danach mit `flow_update_node` (das kennt es). *Trade-off:* zwei Tool-Calls für einen Node mit Icon. *Alternative:* ein Feld an `CreateNodeFields` ergänzen — eine `apiclient`-Änderung, die Spec §2 ausschließt.

**5. `flow_delete_node` verlangt `node` zwingend, `flow_get_node`/`flow_set_node_tags` nicht.**
Spec §3 schreibt `flow_delete_node(node, confirm?)` mit Pflicht-`node`, während die Lese- und Tag-Tools bei weggelassenem `node` den gebundenen Node nehmen.
*Empfehlung:* genau so umsetzen (im Plan so). Löschen ist die einzige irreversible Aktion; „was auch immer dieses Verzeichnis gerade auflöst" ist als Default zu gefährlich. *Trade-off:* eine kleine Asymmetrie in der Tool-Familie, die ein Modell erklärt bekommen muss (die Description tut das). *Alternative:* auch hier `nodeTarget` nutzen und stattdessen auf `confirm` vertrauen.

**6. `nodeTarget` und `prefixGuard` sitzen in `scope.go`, das Spec §6 nicht als MOD listet. — ENTSCHIEDEN: `scope.go` bleibt, und die Spec wurde nachgezogen.** Soenne folgte der Empfehlung; `cmd/flow-mcp/scope.go` steht jetzt im MOD-Block von Spec §6, damit Plan und Spec nicht auseinanderlaufen.
*Empfehlung:* so (Task 1). `scope.go` ist die Node-Referenz-Auflösung dieses Pakets (`lookupNode`, `resolveScope`, `nodeList`), und fünf der sechs neuen Tools brauchen den Helfer — ihn in eine Tool-Datei zu legen, würde die anderen vier auf eine fremde Datei zeigen lassen. *Trade-off:* eine Datei mehr im Änderungsumfang als die Spec vorsah. *Alternative:* eine siebte neue Datei `nodetarget.go` — strenger am Spec-Dateiplan, aber eine Datei für 20 Zeilen.

**7. `bind_path` bleibt beim Anlegen repo-only, obwohl ein blattförmiges Vorhaben bindbar wäre.**
Der atomare Weg lehnt alles außer `repo` ab („bound node must be a repo", `internal/usecase/create_bound_node.go:46`), der separate Bind-Endpunkt erlaubt zusätzlich ein kinderloses Vorhaben (`internal/usecase/bind_node.go:64-75`). Der Plan spiegelt diese Asymmetrie und verweist für ein Vorhaben auf den Zweischritt (anlegen, dann `flow_node_binding`).
*Empfehlung:* so lassen. *Trade-off:* Für ein gebundenes Vorhaben braucht ein Agent zwei Tool-Calls, und die Regel muss erklärt werden (die Description tut das). *Alternative:* die Kind-Prüfung in `CreateBoundNode` auf die Regel von `BindNode` ziehen — Backend-Arbeit, von Spec §2 ausgeschlossen, und die Blatt-Bedingung ist für einen gerade angelegten Node ohnehin trivial erfüllt, was die Sache subtiler macht als sie aussieht.

**8. Der Node-Ref-Cache wird bei jedem Client-Neuaufbau verworfen.**
Neu in Task 1 (Berater-Finding C1): `refreshResolved` setzt `h.projects, h.projFetched = nil, false`, weil ein Neuaufbau eine andere Identität tragen kann und `lookupNode`s Miss-Meldung sonst fremde Slugs auflistet.
*Empfehlung:* so. Ein Tenant-Informationsleck ist in AGENTS.md ein Critical-Finding, ein zusätzlicher `ListNodes`-Aufruf ist es nicht. *Trade-off:* nach jedem `bind`/`unbind`/`create` mit Binding kostet der nächste `lookupNode` eine HTTP-Runde mehr. *Alternative:* den Cache an die Owner-ID schlüsseln statt ihn zu leeren — korrekter bei häufigem Identitätswechsel, aber deutlich mehr Zustand für einen Prozess, der praktisch immer genau einen Owner bedient.

**9. Die Tool-Zahl-Assertion wandert in sechs Ein-Zeilen-Schritten von 25 auf 31.**
*Empfehlung:* so (Tasks 4-9). Jeder Schritt ist ein Wiring-Beweis: wer ein Tool baut und `mcp.AddTool` vergisst, bekommt sofort rot statt erst in Task 10. *Trade-off:* sechs triviale Edits an derselben Zeile. *Alternative:* die Assertion bis Task 10 auf einer Namensliste statt einer Zahl basieren lassen — weniger Churn, aber ein Task ohne eigenen Namenstest könnte durchrutschen.

---

## Self-Review-Appendix

### Grounding-Herkunft

Das Ist-Stand-Dossier wurde **selbst per `Read`/`rg`** erstellt (jede Signatur an der genannten Datei:Zeile gelesen) und anschließend gegen ein `agy`-Dossier über dieselbe Dateiliste gekreuzt. Beide stimmten überein; `agy` bestätigte zusätzlich die Test-Helfer-Signaturen in `cmd/flow-mcp/*_test.go` und meldete korrekt „NICHT VORHANDEN" für json-Tags auf `domain.ProjectBinding`. Kein Bestandsname in diesem Plan ist geraten.

Während des Schreibens zusätzlich verifiziert (über das Dossier hinaus): `internal/usecase/bind_node.go:54` (Upsert), `internal/adapter/pgstore/projectbindings.go:49` (`ON CONFLICT … DO UPDATE SET node_id`), `internal/adapter/pgstore/nodes.go:126-166` (Delete-Guards), `internal/adapter/httpserver/worktime.go:277-295` + `nodemove.go:18-39` + `nodetags.go:32-51` + `projectbindings.go:83,113` (Statuscodes und Emit-Stellen), `internal/domain/user.go:6-12`, `internal/domain/artifact.go:19-35`, `internal/domain/nodestyle.go:16` (leere Farbe/Glyph erlaubt), `Makefile:4-5,76` (Coverage-Scope und ci-Ziele), `internal/adapter/apiclient/artifacts.go:29` + `httpserver/server.go:213` (Artefakt-Route).

**Degradations-Hinweis:** Die Berater-Runde lief nur teilweise. `agy` scheiterte zunächst („no output produced — a tool required the `command` permission that headless mode cannot prompt for") und lieferte erst mit vollständig in den Prompt eingebettetem Inhalt ein Ergebnis. `gemini` (Fallback) brach mit einer interaktiven Browser-Auth-Aufforderung ab und war headless nicht nutzbar. `codex exec` lief vollständig und mit Repo-Zugriff — es ist die tragende Berater-Stimme dieser Runde. Weil `agy` bei eingebettetem 238-KB-Prompt zwei Findings meldete, die im Plan nachweislich vorhanden sind (siehe A1/A2), ist bei seinen `(a)`-Findings von Kontext-Abschneidung auszugehen — die Spec-Abdeckung wurde daraufhin von mir Absatz für Absatz selbst nachgeprüft (Mapping unten).

**Dissens zwischen den Beratern:** Keiner im engeren Sinne — die beiden Runden überschnitten sich nur an einer Stelle, und dort ergänzten sie sich: agy vermutete bei einer Pfad-Kollision einen 409 (A3), codex arbeitete stattdessen die Kind-Beschränkung des atomaren Usecases heraus (C2). Beide Spuren führten zu unterschiedlichen, jeweils echten Defekten; beide sind eingearbeitet. Wo ich einem Berater-Vorschlag nicht gefolgt bin (C3, C5, C8, C10), steht die Begründung oben am Finding — die Entscheidung liegt jeweils bei mir, nicht beim Berater.

### Spec-Abdeckung (jeder Spec-Absatz → Task)

| Spec | Anforderung | Task |
|---|---|---|
| §3 Kopf | Tool-Fläche 25 → 31 | 4, 5, 6, 7, 8, 9 (je +1) · Gate in 10 |
| §3 | `flow_create_node` inkl. Kind-/Parent-Regeln, `upstream` nur repo, kein `slug`, `bind_path` via `CreateBoundNode`, dreiwertiges `counts_toward_target`, Cache-Invalidierung + `refreshResolved` | 4 |
| §3 | `flow_move_node`, genau eines von `parent`/`to_root`, Zyklen serverseitig | 5 |
| §3 | `flow_delete_node`, Trockenlauf, `confirm`, Artefakt-Filter, Minuten statt Sessions, Kinder/Projekt-Docs blocken | 6 |
| §3 | `flow_get_node` inkl. Ahnenkette, Tags, Bindings, Rollup, Fallback auf gebundenen Node | 7 |
| §3 | `flow_set_node_tags`, Replace-Semantik in der Description, resultierende Menge | 8 |
| §3 | `flow_node_binding` mit bind/unbind/list/resolve, `node` bei unbind/resolve unzulässig, clientseitiger Filter, Maschinen-Ausweis | 9 |
| §3 | `flow_bind_project` + `path` + `remote`, − `create_name`/`create_parent`, cwd bleibt Default | 3 |
| §3 | `flow_list_projects` als Baum mit kind/parent | 2 |
| §4 | Ziel-Auflösung komplett (Ausschluss, Existenzprüfung, relativ, Tilde, Origin→remote, `kind`-Override, `remote` ohne Verzeichnis, cwd-Default, Maschinen-ID-Pflicht) | 1 |
| §5 | `errGuard`/`h.resultErr`, Meldungen nennen gültige Werte, `h.lookupNode` mit Cache-Refresh, Mandantentrennung über `h.do` | 1 (`nodeTarget`, `prefixGuard`) + jeder Handler-Task; Global Constraints |
| §6 | Dateiplan (7 neu + 4 mod) | File Structure; Tasks 1-9 |
| §7 | Alle Unit-Test-Punkte | 1 (Ziel-Auflösung), 4 (Kind/Parent), 5 (Move), 9 (Bindings-Adressierung), 2/6/7/9 (Renderer) |
| §7 | Loopback: Tool-Zahl, Kette, `create_name`-Regression, Hierarchie-Regression, Lösch-Fehlerfall | 10 (Kette, Surface, Schemas) · 3 (Schema-Regression) · 6 (409-Regression) |
| §8 | Done-Gate: `make ci`, `go build`, Schema-Smoke, Live-Gate, Kontrakt-Doc, Merge | 11 |
| §2 Ausnahme 1 | `wtfmt` → `internal/timefmt` hochziehen (Vorarbeit für die Minuten-Formatierung in Tasks 6 und 7) | 0 |
| §2 Ausnahme 2 | SSE `node.updated` für bind und unbind (nach dem Done-Gate, eigener Commit) | 12 |
| §9 | Offene Enden als Backlog-Nachtrag | 11, Step 8 |

Keine Nichtzuordnung.

### Berater-Findings und ihr Verbleib

**codex exec** — 12 Findings, davon 10 eingearbeitet, 1 teilweise, 1 abgelehnt. Deutlich stärker als die agy-Runde; zwei Findings (C1, C2) haben echte Defekte aufgedeckt.

- **C1 — „Owner-Wechsel lässt den Node-Cache des vorherigen Owners stehen": EINGEARBEITET, das schwerste Finding der Runde.** Verifiziert: `projFetched` wird ausschließlich in `nodeList` gesetzt und nirgends zurückgesetzt (`rg -n "projFetched" cmd/flow-mcp/` → nur `scope.go:95,103`, `server.go:36`); `refreshResolved` läuft nach **jedem** authentifizierten Client-Neuaufbau, überschreibt aber nur `h.proj`/`h.matched`. Ein Neuaufbau kann eine andere Identität tragen — dann listet `lookupNode`s Miss-Meldung („known slugs: …", `scope.go:87`) die Slugs des vorherigen Owners auf. Ein Cross-Tenant-**Write** ist ausgeschlossen (jede Mutation läuft owner-scoped serverseitig und würde 404en), ein Cross-Tenant-**Informationsleck** nicht — und dieser Slice vervielfacht die Stellen, an denen es auftritt, weil jedes der sechs neuen Tools über `lookupNode` geht. AGENTS.md wertet das als Critical. Eingearbeitet als Task 1, Steps 10-13: `TestRefreshResolved_DropsTheNodeCache` (Owner A → Owner B, prüft explizit, dass die Miss-Meldung Owner As Slug nicht mehr enthält) plus die Invalidierung `h.projects, h.projFetched = nil, false` in `refreshResolved`; `cmd/flow-mcp/server.go` ist damit in Task 1 eine `Modify`-Datei. Kein Backend-Eingriff.
- **C2 — „`bind_path` für engagement/vorhaben erlaubt, der Usecase akzeptiert nur repo": EINGEARBEITET.** Verifiziert: `internal/usecase/create_bound_node.go:46-48` bricht mit `"bound node must be a repo"` ab. Mein Plan hätte `flow_create_node(kind="vorhaben", bind_path=…)` in einen unverständlichen Server-400 laufen lassen. Bemerkenswert ist die Asymmetrie, die dabei sichtbar wurde: der separate Bind-Endpunkt erlaubt zusätzlich ein **blattförmiges Vorhaben** (`bind_node.go:64-75`), der atomare Create-Bound-Weg nicht. Eingearbeitet: Repo-only-Guard in `validateCreateNode` (Task 4) mit Verweis auf den Zweischritt über `flow_node_binding`, drei neue Tabellenfälle in `TestValidateCreateNode`, `TestLoopback_CreateNode_BindPathOnANonRepoIsRejectedBeforeAnyWrite`, und die Einschränkung im `jsonschema`-Tag von `bind_path` sowie in der Tool-Description.
- **C3 — „`kind=path` überschreibt einen Origin beim Auflösen nicht": EINGEARBEITET, anders als vorgeschlagen.** Der Befund stimmt: `bindTarget` trägt `RemoteSlug` auch bei `Kind == "path"` (absichtlich, damit `resolve` alle drei Tiers hat), und `domain.ResolveBinding` priorisiert Remote (`projectbinding.go:31-38`) — `kind=path` hätte also bei `resolve` **keine** Wirkung. Codex wollte einen Konflikttest; ich habe stattdessen die Ursache beseitigt: `kind` ist bei `resolve` und `list` jetzt ein **Fehler** statt eines wirkungslosen Arguments (`validateNodeBinding` in Task 9, `TestLoopback_NodeBinding_ResolveRejectsKind`, plus präzisierter `jsonschema`-Tag). Begründung: `resolve` beantwortet „welchen Node würden die anderen Tools hier auflösen?", und diese Kette ist per Definition Remote-first. Ein akzeptiertes `kind` wäre eine Lüge; ein Test, der die Lüge dokumentiert, hätte sie nur festgeschrieben.
- **C4 — „Whitespace-`remote` widerspricht zwischen Test und Implementierung": EINGEARBEITET.** Echter interner Widerspruch: der Test erwartete für `remote="   "` einen Fehler, die Implementierung trimmte zuerst und wäre auf den cwd-Zweig gefallen. Behoben zugunsten des strengeren Vertrags — ein *vorhandenes, aber leeres* Argument ist ein Fehler, kein stiller cwd-Fallback (`resolveBindTarget` prüft jetzt `trimmed == "" && raw != ""` für `path` und `remote`). Der ursprüngliche Test wurde auf einen echten Parse-Fehlerfall umgestellt (`"not a url at all"`), und `TestResolveBindTarget_BlankArgumentsAreErrorsNotSilentCwdFallbacks` deckt beide Blank-Fälle plus den weiterhin gültigen Omitted-Fall ab.
- **C5 — „Der echte Fehlerpfad von `OriginSlug` wird verworfen": EINGEARBEITET, chirurgisch.** Zutreffend, dass `_` den Fall „git nicht ausführbar" (`gitremote.go:31`) verschluckt. Nicht übernommen wurde die Weitergabe in allen Fällen: Bestand (`bindProject`) degradiert bei fehlendem git zum Pfad-Binding, und das bedingungslos zu ändern würde auf einer Maschine ohne git jedes Binding brechen. Eingearbeitet daher genau dort, wo die Verwechslung schadet: bei **explizitem `kind="remote"`** kommt jetzt „cannot run git in …" statt der irreführenden Bestandsmeldung „needs a git origin in this directory". Test: `TestResolveBindTarget_GitExecFailure` prüft beide Hälften (Auto-Detect degradiert, expliziter Override erklärt).
- **C6 — „Der dreiwertige Bool ist nur zweifach getestet": EINGEARBEITET.** `TestLoopback_CreateNode_CountsTowardTargetIsThreeValued` deckt jetzt alle drei Zustände in einem Test ab (`false` → Privat, `true` → Work, weggelassen → `nil`, damit der Server-Default überlebt).
- **C7 — „Rückwärtskompatibilität von `flow_bind_project(project=…)` nicht exakt getestet": EINGEARBEITET.** Berechtigt — alle meine Task-3-Tests gaben zusätzlich `kind`, `path` oder `remote` mit. Neu: `TestLoopback_BindProject_ProjectOnlyStaysTheHistoricalCall` mit ausschließlich `project`, das cwd-Nutzung und Auto-Erkennung nachweist und ausschließt, dass create-bound angefasst wird.
- **C8 — „Die zulässige Binding-Matrix ist nur negativ angeschnitten": TEILWEISE EINGEARBEITET.** Abgelehnt für die Unit-Ebene: welcher Node ein Binding tragen darf, entscheidet `usecase.BindNode.validateTarget` (`bind_node.go:58-75`) inklusive einer `Children`-Abfrage — der MCP-Client prüft das bewusst nicht vor, ein Fake, der die Blattbedingung modelliert, würde also den Server testen, nicht diesen Slice, und bei einer Server-Änderung falsch-grün bleiben. Eingearbeitet auf der Ebene, die die Matrix wirklich beweist: der positive `path`→Blatt-Vorhaben-Fall ist über C2 ohnehin dokumentiert (er ist genau der Grund, warum `bind_path` beim Anlegen repo-only ist, ein Vorhaben aber im zweiten Schritt bindbar bleibt), und der negative Fall bleibt durch `TestLoopback_NodeBinding_ServerRejectsAnUnbindableKind` abgedeckt.
- **C9 — „Lange Einzelwerte sind nicht abgedeckt": EINGEARBEITET.** Meine eigene Kappung (S2) betraf nur Listen. Für Einzelwerte ist jetzt eine ausdrückliche Regel in den Global Constraints formuliert — **Einzelwerte werden nie gekürzt**, weil Name, Slug, ID und Pfad Adressen sind, die ein Modell in den nächsten Tool-Call zurückschreibt, und ein abgeschnittener Slug eine kaputte Adresse ist. Zwei Vertragstests halten das fest: `TestFormatNodeTree_LongNamesAndSlugsPassThroughVerbatim` und `TestFormatBindingRowsAndResolve_LongPathsPassThroughVerbatim` (beide prüfen zusätzlich, dass kein Ellipsis-Zeichen auftaucht).
- **C10 — „Die 404/409-Matrix ist unvollständig": EINGEARBEITET (409), NOTIERT (404).** Der Projekt-Dokument-409 ist jetzt abgedeckt: `TestLoopback_DeleteNode_ProjectDocumentConflictSurfaces` prüft beides — dass der Trockenlauf blockt *und* dass ein trotzdem bestätigtes Löschen die Servermeldung wörtlich durchreicht. Der TOCTOU-404 (Node verschwindet zwischen `lookupNode` und Mutation) ist **nicht** als eigener Test aufgenommen: er wird bereits durch die bestehende Bestandszusicherung `TestLoopback_UpdateNode_OwnerScope404` (`tools_project_test.go:254`) für genau dieses `h.do`+`resultErr`-Muster gedeckt, und alle sechs neuen Tools benutzen denselben Pfad. Ein sechsfach kopierter Test hätte dieselbe eine Zeile geprüft.
- **C11 — „Das SSE-Gate prüft nicht alle Mutationen und ist nicht ausführbar": EINGEARBEITET, und der Ausführbarkeits-Teil war ein echter Bug.** Zutreffend: Step 6 wies an, „Schritt 1 und 10 zu wiederholen", nachdem Step 5.12 alles gelöscht hatte — `Live Repo` existierte nicht mehr, der `node.deleted`-Beweis war unmöglich, und das neu erzeugte Engagement wäre liegengeblieben. Step 6 ist jetzt ausdrücklich **zwischen 5.8 und 5.9** verortet, verwendet einen eigenen, selbst wieder abgeräumten `SSE Probe`-Node und deckt als Tabelle alle vier emittierenden Mutationen ab — `node.created` (beide Pfade: `worktime.go:237` und create-bound `projectbindings.go:54`), `node.moved` (`nodemove.go:37`), `node.updated` (`nodetags.go:51`) und `node.deleted` (`worktime.go:294`) — plus den zweiten Konsumenten, den TUI-Nodetree.
- **C12 — „Mehrere Bestandsnamen ohne `rg`-Verifikationsstep": EINGEARBEITET.** Task 5 prüft jetzt zusätzlich `ValidParentKind` und die Kind-/Status-Konstanten; Task 6 prüft `flow_move_doc`/`flow_move_node` als registrierte Tool-Namen, damit der Löschbericht kein nicht existierendes Tool empfiehlt; Task 10 prüft `degradedSession`, `text` und `domain.ResolveBinding` (letzteres ruft der Chain-Fixture bewusst auf, damit sein `resolve` genauso priorisiert wie der echte Server, statt eine eigene Auflösung zu erfinden).

**agy** (mit eingebettetem Kontext):

- **A1 — „Fehlende Loopback-Testkette (Spec §7)": BEGRÜNDET ABGELEHNT.** Task 10 enthält `TestLoopback_NodeManagementChain`, das die Kette aus Spec §7 in 13 Schritten gegen ein echtes `httptest`-Backend mit In-Memory-Transport durchläuft (Engagement → Vorhaben → Repo mit `bind_path` → resolve → list → unbind → rebind → get_node → set_node_tags → move → Trockenlauf → confirm → Baum-Gegenprobe). Das Finding widerspricht dem Plantext; Ursache ist mit hoher Wahrscheinlichkeit Kontext-Abschneidung des 238-KB-Prompts.
- **A2 — „Fehlendes Kontrakt-Dokument (Spec §8)": BEGRÜNDET ABGELEHNT.** Task 11, Step 7 zieht `claude-code-flow-kontrakt` über `flow_search_docs` → `flow_get_doc` → `flow_patch_doc` nach und benennt den zu ersetzenden Inhalt. Ebenfalls im Plan vorhanden.
- **A3 — „Fehlerpfad bei Pfad-Kollision (Task 4 & 9)": EINGEARBEITET, und das wertvollste Finding der Runde.** Die Vermutung „vermutlich 409 Conflict" war falsch — die Wahrheit ist schlimmer: `BindNode.Execute` endet in `Bindings.Upsert` (`internal/usecase/bind_node.go:54`), und der Store konfliktet auf `(owner_id, remote_slug)` bzw. `(owner_id, machine_id, path)` und ersetzt **nur `node_id`** (`internal/adapter/pgstore/projectbindings.go:49`). Ein erneutes Binden verschiebt das Ziel also **still** auf den neuen Node. Eingearbeitet als: neuer Global-Constraint-Absatz „Binden ist ein Upsert, kein Konflikt"; Warnung in den Descriptions von `flow_bind_project` (Task 3, Step 7) und `flow_node_binding` (Task 9, Step 8) mit Empfehlung `action="resolve"` als Vorabprüfung; neuer Kettenschritt 7b in Task 10, der das Verschieben beweist; und der Chain-Fixture (Task 10) upsertet jetzt auf den Ziel-Schlüssel statt anzuhängen, weil ein anhängender Fake genau dieses Verhalten verdeckt hätte.
- **A4 — „Fehlende `rg`-Verifikationen (Tasks 2, 3, 4, 6, 7, 9)": EINGEARBEITET.** Berechtigt: die Step-1-Verifikationen waren punktuell statt vollständig. Erweitert um — Task 2: Kind- und Status-Konstanten (`node.go:12-14,21-24`); Task 3: `BindRemote`/`BindPath` und `domain.User`; Task 4: `CreateBoundNodeInput`/`CreateBoundNodeResult`/`BindingFields`; Task 6: `GetNode`/`DeleteNode`/`NodeStats`/`ListDocumentsScoped`, `domain.Artifact`, `LogoRef`; Task 7: `GetNode`/`NodeStats` und `domain.ProjectBinding` (inkl. der fehlenden json-Tags); Task 9: `BindRemote`/`BindPath` plus der Upsert-Nachweis.
- **A5 — „Geratene Typen und Felder in den Test-Fixtures": TEILWEISE EINGEARBEITET, im Kern abgelehnt.** Abgelehnt, dass geraten wurde: `domain.NodeActive`/`NodePaused`/`NodeArchived` (`node.go:12-14`), `domain.User{ID, DisplayName, Email}` (`user.go:6-12`, und wörtlich aus `loopback_test.go:22` übernommen), `domain.Artifact{ID, NodeID, Slug, Name, Mime}` (`artifact.go:19-35`) und `apiclient.CreateBoundNodeInput/Result{Node, Binding}` (`projectbindings.go:20-28`) wurden alle vor dem Schreiben gelesen; sie fehlten lediglich im Dossier-Auszug, nicht in der Verifikation. Eingearbeitet wurde die berechtigte Konsequenz: das Dossier deckte Fixture-Typen nicht ab, deshalb nennt der Global Constraint „Bestandsnamen nur aus dem Dossier" jetzt ausdrücklich auch die Fixture-Typen, und die `rg`-Steps aus A4 verifizieren sie.

### Eigene Findings (vor und neben der Berater-Runde)

- **S1 — Degraded-Mode-Querschnitt fehlte.** Die Bestandssuite hat `TestLoopback_ReadTools_DegradedRequireLogin` (`loopback_test.go:634`), für die sechs neuen Tools gab es nichts. **Eingearbeitet:** `TestLoopback_NodeTools_DegradedRequireLogin` in Task 10 über alle sechs neuen plus `flow_bind_project`, mit absichtlich gültigen Argumenten, damit nur die Authentifizierung scheitern kann.
- **S2 — „lang" war für eine Textoberfläche nicht behandelt.** Der Löschbericht jointe alle Kind-Slugs und alle Dokumentpfade unbegrenzt mit Komma — das Text-Äquivalent eines unbrechbaren Strings, das die handlungsrelevante Zeile ersäuft und Modell-Kontext verbrennt. **Eingearbeitet:** `maxDeleteImpactItems = 10` + `joinCapped` in Task 6, die exakte Anzahl bleibt im Satz erhalten, plus `TestFormatDeleteImpact_ManyChildrenAreCappedNotDumped` und `TestJoinCapped`.
- **S3 — Server-409 bei Zyklus war ungetestet.** Der Plan schrieb „Zyklenfreiheit prüft der Server", ohne zu belegen, dass die Meldung lesbar ankommt. **Eingearbeitet:** `TestLoopback_MoveNode_ServerCycleConflictReachesTheModel` in Task 5 mit eigenem Minimal-Fixture (zunächst als Redirect-Proxy geschrieben, dann als eigenständiger `httptest.Server` ersetzt, weil ein 307 auf ein POST mit Body unzuverlässig replayt).
- **S4 — `ErrInvalidBindTarget` war ungetestet.** Ob ein Node ein Binding tragen darf, hängt an Kind **und** Blatt-Eigenschaft (`bind_node.go:58-75`) und ist clientseitig bewusst nicht vorgeprüft. **Eingearbeitet:** `TestLoopback_NodeBinding_ServerRejectsAnUnbindableKind` in Task 9.
- **S5 — Kommentar widersprach dem Code.** Der Doc-Kommentar von `nodeTarget` behauptete, alle Aufrufer läsen den Node mit `GetNode` neu — `setNodeTags` tut das nicht. **Eingearbeitet:** Kommentar in Task 1 präzisiert („Only Node.ID is guaranteed fresh", Print-Aufrufer müssen neu lesen).
- **S6 — Fixture-Defekte im wörtlichen Code.** `"n" + string(rune('0'+st.nextID))` und `"t" + string(rune('1'+i))` brechen ab dem zehnten Element; `createRecorder` war nicht gofmt-konform ausgerichtet. **Eingearbeitet:** `fmt.Sprintf` statt Rune-Arithmetik, Ausrichtung korrigiert.
- **S7 — Artefakt-Route war eine offene Frage im Plan.** Ursprünglich stand „falls abweichend, gewinnt der Bestand". **Eingearbeitet:** selbst verifiziert (`apiclient/artifacts.go:29`, `httpserver/server.go:213`) und im Plan festgeschrieben, damit der Implementer nicht raten muss.
- **S8 — `TestLoopback_CreateNode_UnknownParentSaysParent` war unbrauchbar formuliert** (verschachtelte `HasPrefix(TrimPrefix(...))`-Logik, die nie fehlschlägt). **Eingearbeitet:** durch klare Substring-Assertions plus Recorder-Prüfung ersetzt.
- **S9 — Ein Wächter-Artefakt stand im wörtlichen Code.** Task 10 enthielt `"kind_ignored": nil` mit der Anweisung, es vor dem Commit zu entfernen — ein Platzhalter in Verkleidung. **Eingearbeitet:** entfernt, durch eine echte Assertion und eine erklärende Notiz ersetzt.

### Bewusst nicht behandelt

- **Fehlende SSE-Events für bind/unbind** — Backend-Arbeit, von Spec §2 ausgeschlossen. Dokumentiert als Offene Entscheidung #1, im Live-Gate (Task 11, Step 6) als erwartete Ausnahme benannt und als Backlog-Nachtrag vorgesehen.
- **`icon` bei `flow_create_node`** — braucht ein neues Feld in `apiclient.CreateNodeFields`, von Spec §2 ausgeschlossen. Offene Entscheidung #4.
- **Tilde-Expansion für `flow_upload_artifact`** — Spec §4 und §9 verweisen es explizit in den Nachtrag; `internal/artifactfile/artifactfile.go` wird in diesem Slice nicht angefasst.
- **Obergrenze für `formatNodeTree`** — der Baum kann bei sehr vielen Nodes lang werden. Der Vorgänger `formatProjects` hatte ebenfalls keine Grenze, und `NodeMRU` als Sortier-/Kürzungshilfe steht bereits in Spec §9. Bewusst Bestandsverhalten.
- **Ungenutzte Route `POST /api/v1/nodes` in `fakeBindBackend`** — nach Task 3 toter Fixture-Code, aber harmlos und außerhalb des Leichen-Sweeps, der auf produktive Altlasten zielt.

