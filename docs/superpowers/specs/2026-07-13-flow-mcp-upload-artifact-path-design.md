---
type: agent
project: github.com/serverkraken/flow
---
# flow-mcp `flow_upload_artifact` — `path`-Parameter (Design)

Date: 2026-07-13 · Branch: `fr-artifact-path` (off `rebuild`) · Status: approved for planning

Source FR: `feature-requests/upload-artifact-path-param` — *`flow_upload_artifact` — Datei-Pfad statt Base64-Pflicht*.

## 1. Problem

Das MCP-Tool `flow_upload_artifact` akzeptiert Dateiinhalte nur als `base64`-Parameter.
Für LLM-Agenten ist das strukturell ungeeignet: der komplette Inhalt fließt Token für
Token durch den Model-Output (ein 160-KB-Bild ≈ 55k Output-Tokens), muss vorher in den
Kontext gelesen werden (Claude Codes Read-Tool kappt lange Zeilen bei ~25k Zeichen), und
verbatim kopiertes Base64 riskiert stille Korruption.

Der MCP-Server läuft **lokal** (er löst bereits das cwd zum Projekt-Node auf) und hat
denselben Dateisystem-Zugriff wie die CLI. Die CLI (`flow artifact add <datei>`) nimmt
Pfade direkt entgegen — die Base64-Pflicht im MCP-Tool ist eine unnötige Einschränkung.

## 2. Ziel / Nicht-Ziele

**Ziel:** `flow_upload_artifact` um einen `path`-Parameter erweitern, gegenseitig
ausschließend mit `base64`. Der MCP-Prozess liest die Datei von Disk und lädt die Bytes
über denselben `apiclient`-Pfad hoch, den das Tool heute schon nutzt — genau wie die CLI.

**Nicht-Ziele (YAGNI, ausdrücklich nicht gebaut):**
- Keine `flow-server`-Änderung. Größenlimit / MIME-Allowlist bleiben serverseitig die
  Quelle der Wahrheit; ein Client-seitiger Größen-Precheck wird **nicht** hinzugefügt.
- Kein Umbau anderer Tools auf `path` — es gibt heute kein weiteres Tool, das Binärdaten
  inline erwartet.
- Die *Nebenbefund*-Punkte der FR (SVG-Rejection-Fehlermeldung, `flow node show` zeigt
  keine Description) gehören zu einer anderen FR und bleiben unangetastet.

## 3. Architektur / Komponenten

Der Change ist auf `cmd/flow-mcp` plus eine kleine geteilte Helper-Extraktion begrenzt.
Datenfluss unverändert ab `apiclient`: MCP-Prozess → `os.ReadFile` → `UploadArtifact` /
`UploadFreeArtifact` → `flow-server`.

### 3.1 NEU `internal/artifactfile/artifactfile.go` (geteilter, getesteter Helper)

```go
// Package artifactfile holds file-side helpers shared by the flow CLI and the
// flow-mcp server for turning a filesystem path into an artifact upload.
package artifactfile

// GuessMime returns override when set, else a best-effort guess from path's
// extension (stripping any "; charset=…" parameter), else the catch-all
// application/octet-stream. No content sniffing — the server validates the
// final MIME type against the allowed set.
func GuessMime(path, override string) string
```

Logik **wörtlich** aus `resolveArtifactMime` in `cmd/flow/artifact.go` übernommen. Das
Datei-Lesen selbst (`os.ReadFile`) bleibt ein Einzeiler beim jeweiligen Aufrufer — es
lohnt keine Wrapper-Funktion.

Begründung: `cmd/flow` und `cmd/flow-mcp` sind getrennte `main`-Pakete und können sich
nicht gegenseitig importieren. Die nicht-triviale Mime-Guess-Logik gehört in ein
`internal`-Paket, das beide importieren (DRY, „Keine Monolithen").

### 3.2 `cmd/flow/artifact.go` (reiner Refactor)

`resolveArtifactMime` löschen; Aufrufstelle in `runArtifactAdd` auf
`artifactfile.GuessMime(path, mimeFlag)` umstellen. Verhalten identisch — die
bestehenden CLI-Tests (`cmd/flow/artifact_test.go`) bleiben grün.

### 3.3 `cmd/flow-mcp/tools_artifacts.go` (das Feature)

`uploadArtifactIn` erweitern:

```go
type uploadArtifactIn struct {
	Node   string `json:"node,omitempty" jsonschema:"project slug, name, or id to attach the artifact to; omit to use the current directory's project"`
	Free   bool   `json:"free,omitempty" jsonschema:"upload/list/delete in the owner-global free (node-less) library instead of a node"`
	Path   string `json:"path,omitempty" jsonschema:"absolute or relative filesystem path the MCP process reads directly (relative resolves against the MCP process's working directory); preferred for files on disk. Mutually exclusive with base64."`
	Name   string `json:"name,omitempty" jsonschema:"the artifact's file name; optional with path (defaults to the file's basename), required with base64"`
	Mime   string `json:"mime,omitempty" jsonschema:"the artifact's MIME type, e.g. image/png; optional with path (guessed from the extension), required with base64"`
	Base64 string `json:"base64,omitempty" jsonschema:"the file contents, base64-encoded; use for small generated content. Mutually exclusive with path."`
}
```

Wichtig: alle drei (`Name`, `Mime`, `Base64`) tragen jetzt `omitempty`, damit das
jsonschema sie **nicht als required** markiert — sonst müsste das Modell weiterhin immer
`base64` liefern.

Auflösung von `data` / `name` / `mime` (vor dem bestehenden `h.do(...)`-Block):

| Bedingung | Verhalten |
|---|---|
| `path` und `base64` beide gesetzt | `errorResult("provide either path or base64, not both")` |
| weder `path` noch `base64` | `errorResult("provide either path or base64")` |
| **path-Modus** | `data, err = os.ReadFile(in.Path)`; bei Fehler `errorResult("read <path>: <err>")`; `name = in.Name` sonst `filepath.Base(in.Path)`; `mime = artifactfile.GuessMime(in.Path, in.Mime)` |
| **base64-Modus** | `name` und `mime` erforderlich (leer → `errorResult`); `data = base64.StdEncoding.DecodeString(in.Base64)`, invalid → bestehender `errorResult` |
| `free` + `node` beide | bestehender `errFreeNodeExclusive`-Check unverändert |

Danach läuft der **unveränderte** `h.do(...)`-Block auf den berechneten `data`/`name`/
`mime`; free- und node-Zweig (inkl. `artifactNode`-Resolution) bleiben wie sie sind.

### 3.4 `cmd/flow-mcp/server.go` (Tool-Description)

`Description` von `flow_upload_artifact` erweitern: beide Varianten dokumentieren und
`path` für Dateien auf Disk empfehlen. Etwa:

> Upload an artifact (image or downloadable file) onto a node. Provide the file as
> `path` (a filesystem path the local MCP process reads directly — preferred for files
> on disk, no token overhead) **or** as `base64` (for small generated content); exactly
> one is required. Scoped to the current project by default; pass node to target another.
> Images render inline via `![[slug]]` in Kompendium docs; other MIME types are download
> links.

## 4. Fehlerbehandlung

- Datei nicht lesbar → `errorResult("read <path>: <err>")` (aktionabel, nennt den Pfad).
- `path` + `base64` beide / keins → `errorResult`, benennt die Regel.
- Base64 ungültig → bestehender `errorResult`.
- Serverseitige Ablehnung (nicht erlaubter MIME-Type, Größenlimit) → wie heute über
  `h.resultErr(err)` durchgereicht. Kein Client-seitiger Größen-Precheck.

## 5. Tests (TDD)

**`internal/artifactfile/artifactfile_test.go`** — `GuessMime`:
- override gesetzt → override gewinnt (auch bei unpassender Endung).
- `.png` → `image/png`; `.pdf` → `application/pdf`.
- Endung mit `; charset=…` (z. B. `.txt` → `text/plain; charset=utf-8`) → Charset gestrippt.
- unbekannte/keine Endung → `application/octet-stream`.

**`cmd/flow-mcp/tools_artifacts_test.go`** — Loopback-Tests (Muster der bestehenden Datei):
- path → node: Temp-Datei anlegen, hochladen; `name` = Basename, `mime` geraten.
- path → free (`free:true`): analog in die free-Library.
- path + explizite `name`/`mime`: Overrides greifen.
- path + base64 beide gesetzt → Validierungsfehler (kein Upload).
- weder path noch base64 → Validierungsfehler.
- path auf nicht existierende Datei → Read-Fehler (kein Upload).
- base64-Regression: bestehender base64-Pfad lädt weiterhin (name+mime required bleibt).

**`cmd/flow/artifact_test.go`** — unverändert grün nach dem `GuessMime`-Refactor.

## 6. Datei-Layout (Keine Monolithen)

```
internal/artifactfile/artifactfile.go        # NEU  GuessMime (aus cmd/flow verschoben)
internal/artifactfile/artifactfile_test.go   # NEU  Unit-Tests
cmd/flow/artifact.go                         # resolveArtifactMime → artifactfile.GuessMime
cmd/flow-mcp/tools_artifacts.go              # Path-Feld + Validierung + path/base64-Auflösung
cmd/flow-mcp/tools_artifacts_test.go         # Loopback-Tests (path-Varianten)
cmd/flow-mcp/server.go                       # erweiterte Tool-Description
```

## 7. Done-Gate

1. `make ci` grün (golangci-lint 0, verify-generate clean, Coverage-Gate, build;
   pgstore-Docker-Tests laufen via Podman — Gate ist golangci-lint, nicht gofmt).
2. `go build -o bin/flow-mcp ./cmd/flow-mcp`.
3. **Schema-Smoke** (Äquivalent zur Main-Wiring-Verifikation — kein neues Tool, keine
   neue Route, aber die Schema-Erweiterung muss durchschlagen): throwaway
   `CommandTransport`-Client listet die Tools; `flow_upload_artifact`s inputSchema
   enthält jetzt `path`, und `base64` ist **nicht** mehr required.
4. **Live-Gate vs. PROD** (`.mcp.json` → `flow.thebackend.org`; `flow login`; `/mcp`
   reconnect auf das neue Binary): eine reale Datei per `path` hochladen (node **und**
   `free:true`) → Artefakt landet, `name`/`mime` korrekt; `path`+`base64` beide sowie
   keins von beiden → Fehler. Test-Artefakte danach aufräumen.
5. Merge `fr-artifact-path` → `rebuild`.

## 8. Akzeptanzkriterien (aus der FR)

- [ ] `flow_upload_artifact` akzeptiert `path` (absolut oder relativ) und lädt die Datei
      ohne Base64 im Request hoch.
- [ ] `base64`-Variante funktioniert unverändert (Rückwärtskompatibilität).
- [ ] `path` + `base64` gleichzeitig → Validierungsfehler; keins von beiden →
      Validierungsfehler.
- [ ] MIME-Guessing aus der Dateiendung, `mime`-Override möglich.
- [ ] Tool-Description im MCP-Schema dokumentiert beide Varianten und empfiehlt `path`
      für Dateien auf Disk.
