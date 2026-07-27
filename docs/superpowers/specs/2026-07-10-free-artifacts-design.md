# Free Artifacts — owner-globale (node-lose) Artefakt-Bibliothek · Design-Spec

> **Stand:** 2026-07-10. Folge-Slice auf **Lesesaal L6** (Node-Artefakte, `docs/superpowers/plans/2026-07-09-lesesaal-l6-artefakte.md`). Additiv — L6 bleibt unverändert. Diese Spec ist die normative Vorlage für den Implementation-Plan (writing-plans).

## 1. Motivation & Problem

L6 hat Artefakte (Bilder, PDFs, Datei-Anhänge) als **node-gebundene** Bausteine eingeführt: ein Artefakt hängt an genau einem `node`, ist über die Ahnenkette referenzierbar und wird per `![[slug]]` als Figur/Chip gerendert. **Freie Notizen** (`Document.Type == "free"`, `NodeID == nil`) haben aber **keinen Node** → keine Ahnenkette → **keine Artefakt-Bibliothek**. Sie können weder Artefakte hochladen noch `![[slug]]` auflösen. Das ist die bewusste Design-Grenze von L6.

Dieser Slice schließt die Lücke mit einer **owner-globalen „freien" Artefakt-Bibliothek**: Artefakte ohne Node, owner-scoped, die freie Notizen (und, als unterste Auflösungs-Ebene, **jedes** Dokument) referenzieren können.

## 2. Bestätigte Entscheidungen (Soenne, 2026-07-10)

| # | Entscheidung | Vorgabe |
|---|---|---|
| E1 | **Reichweite** | Die freie Bibliothek ist **überall** sichtbar. Ein Node-Dokument löst `![[slug]]` gegen seine **Ahnenkette UND** die freie Bibliothek auf — freie Ebene als **root-oberste (niedrigste) Priorität**, node-spezifisch gewinnt bei Namensgleichheit. Freie Notizen sehen **nur** die freie Bibliothek. |
| E2 | **IA-Ort** | Owner-Galerie unter **`/wissen/artefakte`** (im Wissen-Bereich, wo die freien Notizen leben) — Verwalten/Hochladen/Umbenennen/Löschen. |
| E3 | **Datenmodell** | **Nullable `node_id`** auf der bestehenden `artifacts`-Tabelle (Ansatz A). Ein Modell, ein Store, DRY. **Kein** separates `free_artifacts` (B), **kein** synthetischer Root-Node (C). |
| E4 | **Cockpit-Sicht** | Freie Artefakte erscheinen im Node-Cockpit als **geerbt (Quelle „Frei")**, read-only (verwaltet unter `/wissen/artefakte`). |
| E5 | **Scope** | **Volle L6-Parität:** Web-Galerie + Editor-Picker + Resolver-Frei-Ebene + REST + MCP + CLI + SSE. Agenten sind first-class. |

## 3. Datenmodell

### 3.1 Migration (nächste freie Nummer, Bestand gewinnt — L6 belegt `0031`; **Preflight** `ls internal/adapter/pgstore/migrations/ | tail -3`, i. d. R. `0032`)

```sql
-- +goose Up
ALTER TABLE artifacts ALTER COLUMN node_id DROP NOT NULL;
-- Frei-Slug-Eindeutigkeit: unique(owner,node,slug) greift bei NULL-Zeilen NICHT
-- (Postgres behandelt NULL als distinkt) → Partial-Unique-Index für frei.
CREATE UNIQUE INDEX artifacts_owner_free_slug ON artifacts (owner_id, slug) WHERE node_id IS NULL;

-- +goose Down
DROP INDEX artifacts_owner_free_slug;
-- Achtung: DROP NOT NULL ist nur rückrollbar, wenn keine NULL-Zeilen existieren.
-- Down setzt NOT NULL wieder; wenn freie Artefakte existieren, muss der Betreiber
-- sie vorher entfernen. (Down ist ein Entwicklungs-Rollback, kein PROD-Pfad.)
ALTER TABLE artifacts ALTER COLUMN node_id SET NOT NULL;
```

- Der FK `node_id → nodes(id) ON DELETE CASCADE` **bleibt** — NULL wird vom FK nicht geprüft; freie Artefakte haben keinen Node zum Cascaden.
- Bestehender `unique(owner_id, node_id, slug)` **bleibt** (deckt Node-Artefakte).
- Bestehender Index `artifacts_owner_node (owner_id, node_id)` deckt Frei-Listing über das `owner_id`-Präfix.

### 3.2 Domain

- `domain.Artifact.NodeID string` **bleibt** — `""` bedeutet **frei**. Kein Pointer-Umbau.
- pgstore mappt `"" ↔ NULL`: Schreiben via `NULLIF($n, '')`, Lesen NULL → `""` (nullable Scan).
- `Validate()` verlangt **keinen** Node (frei ist gültig). Node-Existenz wird — wie in L6 — im Usecase geprüft, nicht in der Domain.
- Slug-Regeln, MIME-Allowlist, `MaxArtifactBytes`, `IsImage()`, `ref = sha256[:12]` — **unverändert** aus L6.

## 4. Store (`ports.ArtifactStore`)

Alle Reads owner-scoped. NULL-sichere Behandlung von `node=""`:

- **Einzelzeilen-Methoden** (`Get`, `GetMeta`, `Rename`, `Delete`, `ExistingSlugs`): `WHERE owner_id=$1 AND node_id IS NOT DISTINCT FROM $2` — `$2 = NULL` bei `node==""` fällt auf die Frei-Zeilen, sonst exakte Node-Gleichheit. Ein Query-Pfad, NULL-sicher.
- **`Put`** verzweigt am `ON CONFLICT`-Target (weil das Partial-Index-Target nur für NULL-Zeilen greift):
  - Node gesetzt → `ON CONFLICT (owner_id, node_id, slug) DO UPDATE …` (wie L6).
  - Frei (`node==""`) → `ON CONFLICT (owner_id, slug) WHERE node_id IS NULL DO UPDATE …`.
- **Neu: `ListFree(ctx, ownerID string) ([]domain.Artifact, error)`** — `WHERE owner_id=$1 AND node_id IS NULL ORDER BY created_at DESC` (Meta ohne bytes).
- `List(ctx, owner, nodeIDs...)` — **unverändert** (Node-Kette).
- `TotalBytes(ctx, owner)` — **unverändert** (summiert alle Owner-Artefakte inkl. freie → Quota greift automatisch).

**Interface-Ripple:** `ListFree` zwingt jede `ArtifactStore`-Fake zur Methode (Compiler-geführt).

## 5. Usecases

- **`UploadArtifact.Execute(ctx, owner, nodeID, name, mime, data, replaceSlug, actorKind, actorRef)`** — der Ownership-Guard `Nodes.Get(owner, nodeID)` wird **nur bei `nodeID != ""`** ausgeführt (frei hat keinen Node). Rest identisch: Validate/Sniff, Quota (`TotalBytes(owner)+size`), Kollision via `ExistingSlugs(owner, nodeID)` (frei: `nodeID=""`), `ref`, `Put`, **Emit** `artifact.created`/`updated`.
- **`ListArtifacts.Execute(ctx, owner, nodeID)`**:
  - `nodeID == ""` (freies Doc / Frei-Galerie) → `ListFree(owner)` **allein**.
  - `nodeID != ""` → `Ancestors(owner,nodeID)` → `List(owner, chain…)` **++ `ListFree(owner)`** (frei angehängt als root-oberste Ebene).
- **`RenameArtifact` / `DeleteArtifact` / `GetArtifact`** — akzeptieren `nodeID==""` (NULL-sicher über den Store). `RenameArtifact`s `GetMeta`-Guard bestätigt weiterhin, dass das Artefakt an **diesem** Node (bzw. der freien Ebene) hängt.

## 6. Resolver (Rendering)

- **`buildArtifactResolver(chain, arts)`** (in `webui_document.go`): Positions-Map erweitern — `NodeID == ""` (freies Artefakt) bekommt Position `len(chain)` (nach der Wurzel = niedrigste Priorität). Ein `slug`, der sowohl an einem Node der Kette als auch frei existiert, wird vom **node-nächsten** aufgelöst; existiert er nur frei, gewinnt frei. Nearest-wins bleibt die einzige Regel, „frei" ist die letzte Stufe.
- **`ArtifactRef.Href`**: node → `/nodes/{artifact.NodeID}/artifacts/{slug}` (L6), frei (`NodeID==""`) → `/artefakte/{slug}`. Bild-`src = "{Href}?v={Ref}"`, Datei-Chip nacktes `{Href}` — wie L6.
- **Editor-Preview:** `renderEditorPreview` / `handleWebEditorPreview` mit `node==""` bauen den Resolver aus `ListFree(owner)` → `![[slug]]` löst in der Vorschau einer **freien** Notiz auf (heute bleibt es ungelöst — der eigentliche Fix dieses Slice).

## 7. Sicherheit / Sanitizer (KRITISCH — wie L6)

Der Bild-Sanitizer-Gate ist der **`safeImageHTMLRenderer`-Override** auf `ast.KindImage` (die bluemonday-`Matching(re)`-Policy ist wegen OR-Semantik ein No-Op — L6-Befund). Er + die Regexp `artifactSrcRe` müssen **beide** legitimen Serve-Formen durchlassen:

```
^/nodes/[A-Za-z0-9_-]+/artifacts/[a-z0-9-]+(\?v=[0-9a-f]{12})?$      (Node, L6)
^/artefakte/[a-z0-9-]+(\?v=[0-9a-f]{12})?$                            (frei, NEU)
```

Alles andere — externer Host, `data:`-URI, protokoll-relativ `//host/...` — wird weiter gestrippt (leere `src`). Roh-`<img>`-HTML bleibt geblockt (`RenderDocument` setzt kein `html.WithUnsafe()`). **Drei Negativtests bleiben Pflicht** (extern/`data:`/`//host`) **plus** eine Positivkontrolle für die neue `/artefakte/{slug}`-Form.

## 8. Oberflächen (volle Parität, E5)

### 8.1 Serve
- `GET /artefakte/{slug}` (`s.webAuth`) → `GetArtifact(owner, "", slug)`. Identische Header-Logik wie die Node-Serve-Route (ETag `"{ref}"`, Cache-Split nackt=`no-cache`/`?v=`=`immutable`, ETag+Cache-Control **vor** 304, `nosniff`, Bild `inline` / sonst `attachment; filename`).

### 8.2 Web-Galerie `/wissen/artefakte`
- `GET /wissen/artefakte` — Grid (eigene freie Artefakte) + Upload-Form. Nav-Eintrag im Wissen-Bereich (neben Daily/Projekte/Frei/System). Go-1.22-Mux: `/wissen/artefakte` (spezifisch) schlägt `/wissen/{id}` (Wildcard) — kein Konflikt.
- `POST /wissen/artefakte` (multipart-Upload + „Ersetzen" via `slug`-Feld), `POST /wissen/artefakte/{slug}/rename`, `POST /wissen/artefakte/{slug}/delete` (ConfirmDialog). Spiegelt die L6-Cockpit-Galerie-Handler mit `node=""`. **Handler emittieren nicht — die Usecases tun es** (L6-Muster). Fehler inline i18n, kein Popup.
- Neuer SSE-Container auf `/wissen/artefakte`: `hx-trigger="sse:artifact.created, sse:artifact.updated, sse:artifact.deleted"`.

### 8.3 Editor
- Picker-Route `/ui/editor/artefakte?node=` (leer) → freie Bibliothek; mit Node → Kette + frei (via `ListArtifacts`). Der Artefakt-Einfüge-Button ist auch im Editor freier Notizen sichtbar.
- Preview-Node-Kontext: freies Doc → `node=""` → Resolver aus `ListFree` (§6).

### 8.4 REST (owner-scoped, JSON-only)
- `POST /api/v1/artifacts` `{name,mime,dataBase64}` → 201 (frei); `GET /api/v1/artifacts` → Frei-Liste; `DELETE /api/v1/artifacts/{slug}` → 204/404. Body-Limit + Fehler-Codes (400 Bad-Type/zu groß, **413** Quota, 404) wie L6. Node-Routen bleiben unverändert.
- apiclient: **neue** Verben `UploadFreeArtifact(name,mime,data)` / `ListFreeArtifacts()` / `DeleteFreeArtifact(slug)` (die Frei-Routen haben einen anderen Pfad als die Node-Routen `/api/v1/nodes/{id}/artifacts`, daher eigene Methoden statt Node-Verben mit leerem Node). JSON-Konvention + `c.do(...)`-Muster wie L6.

### 8.5 MCP
- `flow_upload_artifact` / `flow_list_artifacts` / `flow_delete_artifact` — Parameter `node` wird **optional**; weggelassen → frei (`node=""`). Der bookable-/Scope-Guard (`h.artifactNode`) wird für den Frei-Fall umgangen (frei hat keinen Node). Actor-Kind `agent` (Server stempelt).

### 8.6 CLI
- Neues Flag **`--free`** auf `flow artifact add/ls/rm` (`flow artifact add x.png --free`, `ls --free`, `rm <slug> --free`) → `node=""`. Eindeutig gegen die cwd-Node-Bindung (ohne `--free` gilt die bisherige Node-/Projektauflösung). `--free` und `--node` schließen sich aus (Fehler bei beidem).

## 9. SSE

Freie Mutationen emittieren dieselben `artifact.created/updated/deleted`-Events **aus den Usecases** (L6-Muster). `EventData` trägt `{"id": slug, "name": name, "node": nodeID}` (bei frei: `node==""`) — konsistent mit dem L6-Final-Review-Fix, damit auch freie Artefakt-Aktionen sauber im account-weiten Puls-Feed erscheinen (Verb-Keys `activity.verb.artifact.*` existieren bereits). Konsument der Galerie-Live-Updates: der `/wissen/artefakte`-SSE-Container (+ Node-Cockpit für Node-Ops, unverändert).

## 10. Fehlerbehandlung

Identisch zu L6: Owner-Quota (`TotalBytes(owner)`) — freie zählen mit → 413 (REST) / inline-i18n (Web); Bad-Type/SVG → 400 / `ErrArtifactBadType`; nicht gefunden → 404 / `ErrArtifactNotFound`; Owner-Scope überall (fremder Owner sieht/lädt/löscht/referenziert nichts).

## 11. Tests (Pflicht)

- **pgstore:** Frei-`Put/Get/GetMeta/Rename/Delete/ExistingSlugs/ListFree/TotalBytes` (node="" ↔ NULL); **NULL-sichere Uniqueness**: freies `"logo"` + node-`"logo"` koexistieren; zweites freies `Put("logo")` (neu) → `-1`-Suffix; Owner-Scope-Negativtest (fremder Owner → nichts/`ErrArtifactNotFound`).
- **Resolver:** freies Doc → nur freie Bibliothek; Node-Doc → Kette **+** frei, node schlägt frei bei gleichem Slug, frei löst wenn nur frei vorhanden.
- **Serve:** `/artefakte/{slug}` Cache/ETag/304/`nosniff`/Disposition + **Cross-Tenant-404**.
- **Sanitizer:** beide Serve-Formen (`/nodes/…`, `/artefakte/…`) überleben; extern/`data:`/`//host` werden gestrippt (3 Negativtests + Positivkontrolle je Form).
- **Web-Galerie:** Grid/Upload/Rename/Delete + `artifact.*`-Emit (echter Bus) + inline-Fehler (Quota/zu groß, kein 500/Popup).
- **Editor:** Picker `?node=` (leer) listet frei (Owner-Scope-Negativ: fremdes freies Artefakt erscheint nicht); Preview einer freien Notiz mit `![[slug]]` → Figur/`<img src="/artefakte/slug?v=…">`.
- **REST/MCP/CLI:** frei-Upload/List/Delete round-trip + **Owner-Scope-Negativtest** (Stub 404 → Fehler, kein stiller Erfolg); CLI `--free`/`--node`-Ausschluss.

## 12. Out of Scope / YAGNI

- **Inline-Upload im Editor** (Upload direkt aus dem Einfüge-Picker) — Upload läuft über `/wissen/artefakte`; der Editor referenziert nur Vorhandenes. Kandidat für später.
- **Verschieben** eines Artefakts zwischen frei ↔ Node oder zwischen Nodes.
- **Ordner/Sammlungen** innerhalb der freien Bibliothek.
- **Live-Reload einzelner Figuren** in offenen Doc-Tabs (wie L6: Re-Upload bumpt `ref`, `?v=hash` lädt beim nächsten Doc-Render neu; bewusst kein Per-Figur-Push).
- Der Owner-Quota-Check-then-Act-Race bleibt der bewusst akzeptierte Soft-Cap aus L6.

## 13. Done-Gate (Live-Dogfood)

Freie Notiz anlegen → in `/wissen/artefakte` Bild + PDF hochladen → in der freien Notiz `![[bild]]` + `![[pdf]]` → Figur/Chip + Abb.-Nummern; Preview löst live auf; ein Node-Doc referenziert dasselbe globale Artefakt (`![[bild]]`) und rendert es; ein node-lokaler Slug schlägt den gleichnamigen freien; Deep-Link/Serve `?v=ref` immutable/nackt no-cache/If-None-Match 304; MCP `flow_upload_artifact` ohne node → landet in `/wissen/artefakte`; CLI `flow artifact add x.png --free`; owner-fremder Zugriff → 404; 960/375px-Sichtprobe.
