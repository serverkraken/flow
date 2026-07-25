# flow-mcp Node-Management — Design

Date: 2026-07-25 · Worktree `flow-node-mgmt`, Branch `node-mgmt` (off `rebuild` @ `2049a1d`) · Status: approved for planning

## 1. Problem

Die Node-Hierarchie ist über MCP fast nur lesbar. `flow_bind_project` bindet
ausschließlich das Working Directory des MCP-Prozesses (`cmd/flow-mcp/tools_project.go:85`),
und sein `create_name`-Zweig kann nur ein `repo` anlegen, das zwingend sofort
gebunden wird (`cmd/flow-mcp/bind.go:67-91`). Für ein Engagement, ein Vorhaben,
einen Node ohne Binding oder ein fremdes Verzeichnis gibt es keinen MCP-Weg.

Konkret aufgelaufen ist das bei der Kontext-Migration am 2026-07-16: für vier
fremde Repos musste auf die CLI ausgewichen werden, weil der MCP-Prozess in einem
anderen Verzeichnis lief.

Ein Mengenvergleich beziffert die Lücke: von 79 exportierten `apiclient`-Methoden
ruft flow-mcp 29 auf. In der Node-Domäne fehlen `CreateNode`, `MoveNode`,
`DeleteNode`, `GetNode`, `Ancestors`, `NodeTags`, `SetNodeTags`, `NodeStats`,
`ListBindings`, `UnbindPath`, `UnbindRemote`, `ResolveNode` und
`ResolveEngagement`.

## 2. Ziel und Abgrenzung

Die Domäne **Nodes und Bindings** wird über MCP vollständig bedienbar, Schreiben
wie Lesen. Danach muss für Node-Verwaltung niemand mehr in die CLI oder WebUI
wechseln.

Ausdrücklich **nicht** in diesem Slice, jeweils mit eigener Spec später:

- **Worktime** — `StartSession`, `StopSession`, `AddSession`, `EditSession`,
  `DeleteSession`, `BulkDeleteSessions`, `ReassignSessions`, die `ListSessions*`-Familie,
  `TagTimes`, `GetWorktimeStatus`, `GetToday`, `GetWeek`, `GetStats`, `GetBurndown`,
  `Export`. Zeit per Agent buchen hat eigene Fragen (laufende Session, Zeitzone,
  Kollisionen), die diese Spec verwässern würden.
- **Day-offs und Settings** — `AddDayOffs`, `DeleteDayOff`, `ListDayOffs`,
  `GetSettings`, `SetBundesland`, `SetTargetConfig`, `RegenIcsToken`.
- **`ImportDocument`** — das Doc-Analogon zum `path`-Parameter bei Artefakten.
- **Logo-Upload** — existiert nur als WebUI-Multipart, es gibt keine
  `apiclient`-Methode. Das wäre Backend-Arbeit, nicht nur Tool-Oberfläche.
  `icon`, `glyph` und `color` laufen bereits über `flow_update_node`.

**Keine Backend-Änderung.** Alle acht benötigten Server-Endpunkte und
`apiclient`-Methoden existieren; dieser Slice baut ausschließlich MCP-Oberfläche.
Keine Migration, keine neue `apiclient`-Methode.

## 3. Tool-Fläche: 25 → 31

```
NEU
  flow_create_node(name, kind, parent?, description?, color?, glyph?,
                   upstream?, counts_toward_target?, bind_path?)
  flow_move_node(node, parent | to_root)
  flow_delete_node(node, confirm?)
  flow_get_node(node?)
  flow_set_node_tags(node?, tags)
  flow_node_binding(action = bind | unbind | list | resolve,
                    node?, path?, remote?, kind?)

GEÄNDERT
  flow_bind_project    + path, + remote   − create_name, − create_parent
  flow_list_projects   Baum mit kind und parent statt flacher Namensliste
```

`resolve` ist eine Aktion der Bindings-Familie und kein eigenes Tool: es ist
dieselbe Frage wie `list`, nur von der anderen Seite her gestellt — welcher Node
hängt an diesem Pfad oder Remote, ohne etwas zu binden.

### flow_create_node

Der einzige Weg, einen Node anzulegen. Ruft `CreateNode`, bei gesetztem
`bind_path` stattdessen `CreateBoundNode`, damit Node und Binding wie heute ein
atomarer REST-Command bleiben (das war Finding 56 des Reviews vom 2026-07-15).

- `kind` ∈ `engagement` | `vorhaben` | `repo`. `branch` wird abgelehnt, solange es
  im Domain reserviert ohne Verhalten ist (`internal/domain/node.go:24`).
- `kind=engagement` lehnt ein gesetztes `parent` ab — `ValidParentKind` lässt ein
  Engagement ausschließlich als Root zu (`internal/domain/node.go:107`).
- `kind=vorhaben` und `kind=repo` verlangen ein `parent`, das zu einem
  `engagement` oder `vorhaben` auflöst (`internal/domain/node.go:103-104`).
  Die Client-Prüfung ist eine Vorprüfung für eine gute Fehlermeldung; der Server
  bleibt die Autorität.
- `upstream` ist nur für `repo` sinnvoll und wird bei anderen Kinds abgelehnt.
- `bind_path` ist optional und ersetzt das Zwangs-Binding von heute. Gesetzt
  bindet es das angegebene Verzeichnis über dieselbe Ziel-Auflösung wie
  `flow_node_binding`; weggelassen entsteht ein reiner Node ohne Binding. Ein
  relativer Pfad löst gegen das Working Directory des MCP-Prozesses auf, `"."`
  ist damit ohne Sonderfall das cwd.
- `counts_toward_target` ist ein `*bool` und damit dreiwertig: weggelassen bleibt
  der Server-Default, `true` und `false` setzen die Work-/Privat-Zuordnung
  explizit.
- Kein `slug`-Parameter: der Server leitet den Slug aus dem Namen ab, wie bei
  `flow node create`. Umbenennen kann `flow_update_node`.
- Nach dem Anlegen wird der Node-Cache invalidiert (`h.nodeList(ctx, true)`),
  damit der neue Node für den nächsten Aufruf sichtbar ist. Bei gesetztem
  `bind_path` zusätzlich `h.refreshResolved`, wie `bindProject` es heute tut.

### flow_move_node

Ruft `MoveNode`. `parent` setzt einen neuen Eltern-Node, `to_root=true` macht den
Node zur Wurzel. Genau einer von beiden ist Pflicht, beide gleichzeitig ist ein
Fehler — JSON unterscheidet nicht zwischen „weggelassen" und „leerer String",
weshalb das CLI-Muster `--parent ""` mit `fl.Changed` über MCP nicht abbildbar
ist. Zyklenfreiheit prüft der Server.

### flow_delete_node

Ohne `confirm` löscht das Tool nichts, sondern berichtet die Folgen. Mit
`confirm=true` ruft es `DeleteNode`.

Der Trockenlauf liest: den Node selbst (`GetNode`, liefert auch `LogoRef`), seine
Kinder aus der gecachten Node-Liste über `ParentID`, seine eigenen Artefakte
(`ListArtifacts`), die Projekt-Dokumente in seinem Scope
(`ListDocumentsScoped(&nodeID)`, gefiltert auf `type=project`) und `NodeStats`.

Zwei Fallstricke, die der Bericht korrekt behandeln muss:

- `ListArtifacts` liefert die Artefakte **inklusive der Ahnenkette**
  (`internal/usecase/list_artifacts.go:25`). Der Bericht filtert auf
  `NodeID == node.ID`, sonst zählt er fremde Artefakte mit, die gar nicht
  gelöscht werden.
- `NodeStats` liefert nur Minuten (`TotalMin`, `WeekMin`, `MonthMin`), keinen
  Session-Count, und rollt den **Teilbaum** auf. Weil ein Node mit Kindern
  ohnehin nicht löschbar ist, ist die Zahl genau im löschbaren Fall exakt. Der
  Bericht nennt deshalb Minuten, nicht Sessions.

Kinder und Projekt-Dokumente melden „nicht löschbar", weil die DB das ohnehin
blockt: `nodes.parent_id` ist `ON DELETE RESTRICT` (Migration `0016`), und
Projekt-Dokumente prüft der Store explizit (`internal/adapter/pgstore/nodes.go:144-151`).
Sessions und übrige Dokumente werden still auf `NULL` gesetzt (Migration `0012`),
Bindings, Artefakte und Logos per `CASCADE` gelöscht — genau diese stille Wirkung
macht der Bericht sichtbar.

```
flow_delete_node(node="jukebox")
→ Would delete repo "Jukebox" (jukebox).
  12h 30m of booked worktime would lose its node.
  3 artifacts and 1 logo would be deleted.
  No children, no project documents — delete is possible.
  Pass confirm=true to proceed.

flow_delete_node(node="jukebox", confirm=true)
→ Deleted repo "Jukebox" (jukebox).
```

### flow_get_node

Das MCP-Gegenstück zu `flow node show`. Node-Detail inklusive Description,
Ahnenkette (`Ancestors`), Node-Tags (`NodeTags`), Bindings dieses Nodes und
Worktime-Rollup (`NodeStats`). `node` weggelassen nimmt den für dieses
Verzeichnis aufgelösten Node; ist keiner gebunden, ist das ein Fehler mit
Hinweis auf `flow_node_binding`.

### flow_set_node_tags

Ruft `SetNodeTags` und **ersetzt die Tag-Menge komplett** — dieselbe Semantik wie
`flow_create_doc` für Dokument-Tags. Die Tool-Description sagt das ausdrücklich,
damit ein Modell nicht versehentlich Tags wegwirft. Die Antwort nennt die
resultierende Menge.

### flow_node_binding

Vier Aktionen auf derselben Ziel-Adressierung:

- `bind` — bindet das Ziel an `node` (Pflicht). Ruft `BindRemote(nodeID, remoteSlug)`
  oder `BindPath(nodeID, machineID, machineLabel, path)`.
- `unbind` — löst das Binding des Ziels. Ruft `UnbindRemote(remoteSlug)` oder
  `UnbindPath(machineID, path)`. Beide Signaturen kennen **keinen** Node, das
  Binding wird allein über sein Ziel adressiert; ein mitgegebenes `node` wäre
  irreführend und ist deshalb ein Fehler.
- `list` — listet die Bindings. `ListBindings` nimmt keine Filterparameter, das
  Filtern auf `node` passiert deshalb clientseitig über `NodeID`.
- `resolve` — beantwortet, welcher Node an diesem Ziel hängt, ohne zu binden.
  Ruft `ResolveNode(remoteSlug, machineID, path)`, dazu
  `ResolveEngagement(...)` für die Angabe des Engagements. `node` ist hier
  ebenfalls unzulässig.

`ListBindings` ist owner-scoped und liefert die Bindings **aller Geräte** des
Nutzers. Die Ausgabe von `list` weist deshalb Maschinen-Label und Maschinen-ID
aus, sonst verwechselt ein Agent Notebook A mit Notebook B.

### flow_bind_project

Bleibt als Kurzform für den Alltagsfall und bekommt `path` und `remote`, verliert
aber `create_name` und `create_parent` — angelegt wird ab jetzt nur noch mit
`flow_create_node`. Ohne `path` und ohne `remote` gilt weiter das cwd des
MCP-Prozesses, der bisherige Aufruf bleibt also unverändert gültig.

### flow_list_projects

Rendert die Hierarchie statt einer flachen, alphabetischen Liste. Heute fehlen in
der Ausgabe `kind` und `parent` (`cmd/flow-mcp/format.go:168-184`) — genau die
Information, die `flow_create_node` braucht, um einen gültigen Parent zu wählen.
Ohne sie rät das Modell. Ausgabe: Baum mit Einrückung, je Zeile Kind-Glyph, Name,
Slug, Status und ID; Wurzeln alphabetisch, Geschwister alphabetisch.

## 4. Ziel-Auflösung (`bindtarget.go`)

Das Herzstück der Wiederverwendung: `flow_bind_project`, alle vier Aktionen von
`flow_node_binding` und das optionale `bind_path` von `flow_create_node` gehen
durch dieselbe Funktion. Sie nimmt `path`, `remote` und `kind` und liefert
Origin-Slug, Binding-Kind, Maschine und Pfad.

- `path` und `remote` schließen sich aus.
- `path` verlangt ein **existierendes Verzeichnis** (`os.Stat`, muss ein
  Directory sein). Grund: `gitremote.OriginSlug` führt `git -C <dir>` aus und
  behandelt jeden Nicht-Null-Exit als „kein Origin" ohne Fehler
  (`internal/gitremote/gitremote.go:22-33`). Ein Tippfehler im Pfad würde sonst
  still ein Pfad-Binding erzeugen, das nie auflöst.
- Relative Pfade lösen gegen das Working Directory des MCP-Prozesses auf, wie bei
  `flow_upload_artifact` (Spec vom 2026-07-13).
- `~` **wird expandiert** — und das ist eine bewusste Abweichung von
  `flow_upload_artifact`, das keine Tilde-Expansion kennt
  (`internal/artifactfile/artifactfile.go:32-57` reicht den Pfad direkt an
  `os.Open`). Begründung: bei einem CLI-Aufruf expandiert die Shell die Tilde,
  bevor das Programm sie sieht; ein MCP-Tool-Call übergibt dagegen einen
  JSON-String ohne Shell, und ein Modell schreibt erfahrungsgemäß
  `~/SourceCode/foo`. Ohne Expansion scheitert das — dank der Existenzprüfung
  immerhin laut und nicht still. Die Expansion gehört in `bindtarget.go`;
  `flow_upload_artifact` auf dieselbe Semantik zu ziehen ist ein Nachtrag für
  Abschnitt 9, kein Teil dieses Slices.
- Liegt am Pfad ein git-Origin, entsteht ein Remote-Binding, sonst ein
  Pfad-Binding auf dieser Maschine. `kind` überschreibt das; `kind=remote` ohne
  Origin bleibt ein Fehler, wie `decideBindKind` es heute hält
  (`cmd/flow-mcp/bind.go:32-49`).
- `remote` normalisiert den Origin-Slug über `domain.NormalizeRemoteSlug`, ohne
  ein lokales Verzeichnis zu verlangen — damit ist ein Repo bindbar, das auf
  dieser Maschine nicht liegt.
- Ohne `path` und ohne `remote` gilt das cwd des MCP-Prozesses.
- Ein Pfad-Binding braucht eine Maschinen-ID; fehlt sie, ist das ein Fehler, wie
  heute (`cmd/flow-mcp/bind.go:63-65`).

## 5. Fehlerbehandlung

Validierungsfehler laufen über `errGuard` (`cmd/flow-mcp/tools_write.go:16`) und
erreichen das Modell als `invalid_request` mit wörtlicher Meldung; Server- und
Auth-Fehler gehen durch `h.resultErr` (`cmd/flow-mcp/server.go:173`). Jede
Meldung nennt die gültigen Werte statt leer zurückzukommen — dieselbe Konvention
wie die bestehende „unknown project"-Meldung, die die bekannten Slugs auflistet
(`cmd/flow-mcp/scope.go:87`). Node-Referenzen werden über `h.lookupNode` gelöst,
das bei einem Miss den Cache einmal erneuert, damit ein gerade angelegter Node
sofort adressierbar ist.

Zur Mandantentrennung: alle Aufrufe gehen über `h.do` an den authentifizierten
`apiclient`, es entstehen keine neuen Store-Queries und damit keine neue
Tenant-Fläche. Der Löschbericht nutzt ausschließlich owner-scoped Endpunkte.
Pfad-Bindings tragen die Maschinen-ID und bleiben damit gerätebezogen.

## 6. Dateien

Eine Verantwortung pro Datei, je mit eigener Testdatei:

```
NEU  cmd/flow-mcp/tools_node_create.go      flow_create_node + Kind/Parent-Validierung
NEU  cmd/flow-mcp/tools_node_lifecycle.go   flow_move_node, flow_delete_node, Löschfolgen-Bericht
NEU  cmd/flow-mcp/tools_node_inspect.go     flow_get_node
NEU  cmd/flow-mcp/tools_node_tags.go        flow_set_node_tags
NEU  cmd/flow-mcp/tools_bindings.go         flow_node_binding + flow_bind_project (Umzug)
NEU  cmd/flow-mcp/bindtarget.go             path | remote | cwd → Origin, Kind, Machine, Pfad
NEU  cmd/flow-mcp/format_nodes.go           Baum, Node-Detail, Bindings, Löschfolgen
MOD  cmd/flow-mcp/bind.go                   bindNodeCore nutzt bindtarget, verliert den Create-Zweig
MOD  cmd/flow-mcp/tools_project.go          behält project_context, list_projects, update_node
MOD  cmd/flow-mcp/server.go                 sechs Registrierungen, zwei neue Descriptions
MOD  cmd/flow-mcp/format.go                 formatProjects → Baum-Renderer aus format_nodes.go
```

## 7. Tests (TDD)

Unit-Tests je neuer Datei, failing-first:

- **Kind- und Parent-Validierung:** `engagement` mit `parent` → Fehler;
  `vorhaben`/`repo` ohne `parent` → Fehler; `repo` unter `repo` → Fehler;
  `branch` → Fehler; `upstream` bei `engagement` → Fehler; gültige Kombinationen
  → durchgelassen.
- **`flow_move_node`:** `parent` und `to_root` gleichzeitig → Fehler; keines von
  beiden → Fehler; `to_root` allein und `parent` allein → je ein Aufruf.
- **Ziel-Auflösung:** fehlendes Verzeichnis → Fehler; Datei statt Verzeichnis →
  Fehler; `path` und `remote` gleichzeitig → Fehler; `kind=remote` ohne Origin →
  Fehler; Origin vorhanden → Remote-Kind; kein Origin → Pfad-Kind; relativer
  Pfad → gegen MCP-cwd aufgelöst; `~/x` → gegen `os.UserHomeDir` expandiert;
  `remote` ohne lokales Verzeichnis → Remote-Kind; Pfad-Kind ohne Maschinen-ID →
  Fehler.
- **Bindings-Adressierung:** `node` bei `unbind` und bei `resolve` → Fehler;
  `list` mit `node` filtert clientseitig auf `NodeID`.
- **Renderer:** Baum mit Einrückung und Kind je Zeile; Node-Detail mit
  Ahnenkette; Bindings-Liste mit Maschinen-Label; Löschbericht mit
  Artefakt-Filterung auf den eigenen Node (Ahnen-Artefakt darf nicht mitgezählt
  werden) und mit Minuten statt Sessions.

Loopback-Tests gegen das echte httptest-Backend mit In-Memory-Transport, im
Muster der bestehenden `loopback_test.go`:

- Tool-Zahl-Assertion von 25 auf 31 (`cmd/flow-mcp/loopback_test.go:353`).
- Kette: Engagement anlegen, Vorhaben darunter, Repo darunter mit `bind_path`,
  `resolve` findet es, `list` zeigt es mit Maschine, `unbind`, `bind` erneut auf
  ein fremdes Verzeichnis, `flow_get_node` zeigt Ahnenkette und Tags,
  `flow_set_node_tags` ersetzt die Menge, `flow_move_node` hängt um,
  `flow_delete_node` ohne `confirm` berichtet und löscht nicht, mit `confirm`
  löscht es.
- Regression: `create_name` steht nicht mehr im `inputSchema` von
  `flow_bind_project`, und `path` sowie `remote` stehen darin.
- Regression: `flow_list_projects` gibt Hierarchie mit `kind` aus.
- Fehlerfall-Regression: Löschen eines Nodes mit Kind → `IsError` mit
  verständlicher Meldung.

## 8. Done-Gate

- `make ci` grün (lint, verify-generate, verify-css, verify-no-popups, cover, build).
- `go build ./cmd/flow-mcp`.
- Schema-Smoke auf die sechs neuen `inputSchema`s.
- Live-Gate gegen den Dev-Stack (`make dev-up`, `make dev-run`): Engagement,
  Vorhaben und Repo anlegen, ein echtes fremdes Verzeichnis binden, auflösen,
  listen, entbinden, umhängen, Trockenlauf und Löschen mit `confirm`.
- Kontrakt-Doc `claude-code-flow-kontrakt` nachziehen: dort steht heute
  `flow_bind_project` als Weg zum Anlegen.
- Merge `node-mgmt` → `rebuild`.

## 9. Offene Enden nach diesem Slice

Bewusst nicht Teil des Slices, als Backlog-Nachtrag zu `notes/backlog-offene-slices`:
Worktime per MCP, Day-offs und Settings per MCP, `ImportDocument` mit
`path`-Parameter, Logo-Upload (braucht erst eine `apiclient`-Methode),
`NodeMRU` als mögliche Sortierhilfe für die Baum-Ausgabe, und die
Tilde-Expansion für `flow_upload_artifact`, damit beide `path`-Parameter
dieselbe Semantik haben (siehe Abschnitt 4).
