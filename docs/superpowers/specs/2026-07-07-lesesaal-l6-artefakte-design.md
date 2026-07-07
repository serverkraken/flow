# flow — Lesesaal L6: Artefakte + Deep-Links — Design Spec

> Datum: 2026-07-07 · Status: **APPROVED** (Soenne: „Ja, das passt") · Branch: `lesesaal-l6` (off `rebuild` `9c37d6e`).
> Übergeordnete Spec: [[specs/2026-07-04-lesesaal-webui-redesign-design]] (§Lese-Ebene „Artefakte als Figuren", §Plan-Input „Artefakt-Storage (neu)", §Slicing „L6 Artefakte"). Vorgänger: L5 + L5.5 (Kontext-Kuratierung + Kontext-Modus), gemergt `db8b0a5`.
> Normative Optik-Referenz: `docs/superpowers/specs/assets/2026-07-03-lesesaal/lesesaal.html` (v2.4) + vorhandene Lesesaal-Primitives.

---

## 1. Anlass & Auftrag

Die Master-Spec pinnt L6 bewusst dünn: **Artefakt-Storage (neu)** + **„Artefakte als Figuren (Datei-Chip/Preview)"** in der Lese-Ebene. Dieser Slice entwirft das aus. Soenne hat im Brainstorming die mächtigere Ausprägung gewählt und zwei angrenzende „Verlinkung"-Anliegen eingebracht, die in dieselbe Fläche (Dokument-Leseseite + goldmark-Pipeline + Editor) fallen und mitkommen.

**Umfang L6:** Artefakt-Storage + Cockpit-Galerie + Inline-Figuren in Dokumenten + Überschriften-Deep-Links + Editor-Einfügehelfer (Artefakt & Wikilink).

## 2. Entscheidungen (Brainstorming 2026-07-07)

- **E1 — Artefakt-Modell:** **Node-Asset-Bibliothek**. Ein Artefakt hängt an einem `node` (nicht an einem Dokument), über mehrere Dokumente wiederverwendbar.
- **E2 — Erscheinung:** **Galerie + inline referenzierbar**. Cockpit-Galerie zum Verwalten *und* aus Dokumenten heraus per `![[slug]]` inline als nummerierte Figur.
- **E3 — Autorschaft:** **Mensch + Agent**. Mensch via WebUI-Upload; Agent via `flow-mcp` + CLI. „Generische Features in alle Hosts."
- **E4 — Storage:** **Postgres-Blob** (`bytea`), Präzedenz `node_logos`. Kein externer Objektspeicher (YAGNI, Homelab-Single-Postgres).
- **E5 — Deep-Links mit rein:** Überschriften-Slug-Anker als Begleit-Baustein in L6 (nicht eigener Slice).
- **E6 — Editor-Einfügehelfer mit rein:** Picker für Artefakt-Referenz *und* Seiten-Wikilink im Editor.

Eingebettete Detail-Empfehlungen (im Brainstorming bestätigt): MIME-Allowlist mit SVG-Verbot · Limit ~8 MB · **Reichweite = Ahnenkette (Node + Vorfahren)** — Auflösung nach oben, ein Artefakt am Engagement ist in allen Repos darunter referenzierbar · `![[…]]`-Syntax · Hover-Anker „¶" · Upload nur in der Galerie (Editor referenziert nur Vorhandenes).

## 3. Nicht-Ziele (bewusst außen vor)

- **Externer Objektspeicher / große Dumps** (>Limit). Später, falls je nötig.
- **Inline-Autocomplete beim Tippen von `[[`** (Obsidian-Gefühl) — mehr fragiles JS; Kandidat für L7-Politur, nicht L6.
- **Artefakt-Versionierung / Historie.** Ein Blob pro `(node, slug)`; Re-Upload überschreibt (ETag bumpt).
- **Doc-scoped Artefakte** (Anhang direkt am Dokument). E1 verwirft das zugunsten der Node-Bibliothek.
- **SVG-Inline-Rendering** (XSS) — SVG-Upload verboten, wie beim Logo.
- **Transklusion von Dokumenten** via `![[doc]]` — `![[…]]` löst ausschließlich Artefakte auf; Doc-Einbettung ist kein Ziel.

## 4. Datenmodell

### 4.1 Migration 0031 (`-- +goose Up/Down` Pflicht)

Neue Tabelle `artifacts` (Muster: `node_logos`, aber N pro Node statt 1):

```
artifacts(
  id            text primary key,
  owner_id      text not null,
  node_id       text not null references nodes(id) on delete cascade,
  slug          text not null,          -- referenzierbarer Name, node-eindeutig
  name          text not null,          -- Anzeigename (Originaldateiname)
  mime          text not null,
  size_bytes    bigint not null,
  ref           text not null,          -- 12-hex Content-Hash (ETag + ?v= Cache-Bust), Muster node_logos
  bytes         bytea not null,
  width         int,                    -- nullable; nur Bilder
  height        int,                    -- nullable; nur Bilder
  created_by_kind text not null default '',  -- 'human' | 'agent' | '' (unbekannt)
  created_by_ref  text not null default '',
  created_at    timestamptz not null,
  updated_at    timestamptz not null,
  unique (owner_id, node_id, slug)
)
```

Index `artifacts_owner_node (owner_id, node_id)` für Listen/Serve. FK `ON DELETE CASCADE` räumt beim Node-Löschen auf (wie `node_logos`).

### 4.2 Domain

`domain.Artifact` (Struct wie oben). Reine Validierung `Validate()`:
- `SlugOK(slug)` (vorhandener Slug-Regex).
- `mime ∈ ArtifactMimeAllowlist` — **Bilder** `image/png,image/jpeg,image/webp,image/gif` (inline-preview-fähig) · **Downloads** `application/pdf,text/csv,text/plain,application/json,application/zip,application/octet-stream` (+ erweiterbar). **SVG (`image/svg+xml`) verboten.**
- `size_bytes ≤ MaxArtifactBytes` (`8 << 20`).
- `IsImage()` (mime-Präfix `image/`) entscheidet Preview vs. Chip; `width/height` nur dann gesetzt (via `image.DecodeConfig`, Muster `ValidateNodeLogo`).

`ArtifactSlug(name)` — Slugifizierung des Dateinamens (Muster Doc-`slugify`), Basis für Kollisions-Suffix.

## 5. Storage + Serving

- **`ports.ArtifactStore`** (+ `ErrArtifactNotFound`): `Put(ctx, Artifact) error` (Upsert auf `(owner,node,slug)`), `Get(ctx, owner, node, slug) (Artifact, error)` (inkl. bytes), `GetMeta(...)` (ohne bytes, für Listen), `List(ctx, owner, nodeIDs...) ([]Artifact, error)` (Meta; Caller übergibt die Ahnenkette — Node + Vorfahren — für Galerie/Picker), `Delete(ctx, owner, node, slug) error`, `ExistingSlugs(ctx, owner, node) ([]string, error)` (Kollisions-Check beim Anlegen).
- **`pgstore.ArtifactStore`** — analog `nodelogos.go`.
- **Serve-Route** `GET /nodes/{id}/artifacts/{slug}` (webAuth, owner-scoped): `Content-Type: mime`, `ETag: "{contentHash}"`, `Cache-Control: private, max-age=31536000, immutable`, 304 bei `If-None-Match`. `Content-Disposition: inline` für Bilder, `attachment; filename="name"` sonst.

## 6. Referenz + Rendering (Lese-Ebene)

### 6.1 Syntax + Auflösung
- `wikiLinkParser` erkennt zusätzlich die **Embed-Form** `![[slug]]` (führendes `!`). `[[…]]` bleibt unverändert Doc-Wikilink.
- Auflösung: gegen die **Artefakt-Bibliothek der Ahnenkette des Dokuments** (`NodeStore.Ancestors(doc.NodeID)` leaf→root, wie Kontext-Compose) per `slug`. Erster Treffer gewinnt (nächster Vorfahr zuerst).
- **Nicht gefunden / Doc ungebunden** → sichtbarer „ungelöst"-Chip (Muster unaufgelöster Wikilink), bricht das Rendern nicht.

### 6.2 Figur
- **Bild** → `<figure>` mit `<img loading="lazy">` (Serve-Route als `src`, `?v={hash}` gegen Stale) + `<figcaption>` „Abb. N — {name}". **N teilt sich denselben `FigLabel`-Zähler mit Mermaid** (durchgehende Figurennummerierung im Dokument).
- **Nicht-Bild** → `<figure>` mit **Datei-Chip** (Typ-Icon, `name`, formatierte Größe, Download-Link auf die Serve-Route mit `attachment`).
- **Eindämmung (L3-Regel):** jede Figur scrollt im eigenen Rahmen, mobil Bild-Naturgröße, Seite pannt nie horizontal.
- Der HTML-Renderer läuft **vor** dem bluemonday-Sanitizer; die Policy muss `<figure>/<figcaption>/<img src|alt|loading|width|height>/<a download>` auf den Artefakt-Knoten erlauben (siehe §8).

## 7. Deep-Links (Verlinkung)

- goldmark `parser.WithHeadingAttribute()` **+** AST-Transformer, der jede Überschrift **GitHub-Stil sluggt** (identisch zum Slug in geteilten URLs) und bei Duplikaten im selben Dokument `-1/-2/…` anhängt. Ergebnis: stabile `id` an `h1–h6` server-seitig.
- **Sanitizer:** `id` auf `h1..h6` erlauben.
- **`toc.js`:** nutzt die vorhandenen Server-`id`s; das positionelle `heading.id = 'h-'+index` bleibt nur Fallback für ID-lose Überschriften.
- **Hover-Anker:** dezentes „¶" an Überschriften, das `#slug` verlinkt (kopierbar). Rein CSS/Markup + winziges optionales JS für „Link kopiert"-Feedback (kein `alert`, `verify-no-popups`).

## 8. Sicherheit / Sanitizer

- **SVG-Upload verboten** (kein Inline-Fremd-Markup).
- `getDocPolicy()` (bluemonday) erweitern: `id` auf Überschriften; `figure/figcaption`; `img` mit `src`(nur relative Serve-Route-Pfade oder same-origin)/`alt`/`loading`/`width`/`height`; `a` mit `download`. **Kein** beliebiges `img src` auf externe Hosts (CSP + Policy) — Artefakt-`src` zeigt immer auf die eigene Serve-Route.
- Datei-Content wird nie als HTML interpretiert; Downloads gehen als `attachment` mit korrektem `Content-Type` (kein Sniffing-XSS: `X-Content-Type-Options: nosniff` auf der Serve-Route).

## 9. Editor-Einfügehelfer

Zwei Buttons in der Editor-Werkzeugleiste über der `<textarea name="body">` öffnen je einen **`FuzzyPicker`**-Dialog (htmx-geladene Liste, fuzzy-Filter):
- **„Artefakt einfügen ⋯"** → `GET /ui/editor/artefakte?node={id}` listet die Artefakte des Editor-Kontext-Nodes + seiner Vorfahren (Ahnenkette) → Auswahl fügt `![[slug]]` an `selectionStart` ein.
- **„Seite verlinken ⋯"** → `GET /ui/editor/seiten` listet Dokumente (fuzzy/MRU, Muster ⌘K-Palette) → Auswahl fügt `[[pfad]]` ein.
- Einfügen via kleines JS (`static/js/editor-insert.js`): am `selectionStart` einsetzen, Cursor dahinter, Live-Vorschau (`/wissen/preview`) triggern.
- **Upload bleibt in der Galerie** (§10); der Editor referenziert nur Vorhandenes.

## 10. Cockpit-Galerie

Artefakt-Block auf `/nodes/{id}` (Fragment `#cockpit-...`, SSE-Container):
- Grid aus **Bild-Thumbnails** / **Datei-Chips** (eigene Artefakte + die von Vorfahren geerbten; Herkunft markiert, wenn von einem Vorfahren). Nachfahren-Artefakte gehören in deren eigene Cockpits, nicht hierher.
- **Upload** (multipart `enctype`, Icon-/Datei-Input; Fehler inline i18n, kein Popup) → `POST /api/v1/nodes/{id}/artifacts` bzw. Web-Handler.
- **Umbenennen** (ändert Anzeigename; Slug bleibt stabil, sonst brechen Referenzen) und **Löschen** (mit Bestätigungs-Dialog `data-dialog-open`, kein `confirm()`).
- Kollision beim Upload: Slug existiert → `-1/-2`-Suffix (Serverseite über `ExistingSlugs`).

## 11. REST · MCP · CLI

- **REST** (owner-scoped, `requireBookable`/Node-Ownership geprüft):
  - `POST /api/v1/nodes/{id}/artifacts` — multipart *oder* `{name,mime,dataBase64}` → 201 mit Artefakt-Meta (+ finalem Slug).
  - `GET /api/v1/nodes/{id}/artifacts` — Liste (Subtree, Meta).
  - `DELETE /api/v1/nodes/{id}/artifacts/{slug}` — 204.
  - `GET /nodes/{id}/artifacts/{slug}` — Serve (§5).
- **flow-mcp:** `flow_upload_artifact(node, name, mime, base64)` · `flow_list_artifacts(node)` · `flow_delete_artifact(node, slug)`. Node-Auflösung wie bestehende Tools (Slug/Name/Binding).
- **CLI:** `flow artifact add <datei> [--node] · ls [--node] · rm <slug> [--node]` (dünner apiclient).

## 12. SSE / Live-Sync

- Neue Events `artifact.created` · `artifact.updated` (Re-Upload/Umbenennen) · `artifact.deleted`, emittiert von den Mutations-Usecases über den `Emitter`.
- Konsument: der Cockpit-Galerie-Container (`hx-trigger="sse:artifact.created, sse:artifact.updated, sse:artifact.deleted"`).
- Dokument-Embeds: die Doc-Seite bleibt an `document.updated`; ein Re-Upload bumpt den Content-Hash → `<img ?v=hash>` lädt bei nächstem Doc-Render neu. (Kein Live-Reload einzelner Figuren in offenen Doc-Seiten — bewusst YAGNI.)

## 13. Querschnitt / Done-Gate

- **Multi-Tenant:** jede Query owner-scoped; Node-Ownership vor jedem Zugriff; **Negativtests** (fremder Owner sieht/lädt/löscht nichts).
- **goose-Annotationen** in 0031 (nur pgstore-Docker-Test fängt fehlende).
- **i18n de+en** für alle neuen Strings (test-enforced).
- `verify-no-popups` (kein `alert/confirm/prompt`); `verify-css`/`verify-generate`.
- **main.go-Wiring-Task** (Composition-Root): `ArtifactStore` + Usecases + Routen + MCP-Tools verdrahtet, mit curl-Smoke (sonst „ship a usecase nothing calls").
- **TDD**, `make ci` grün je Task (Coverage-Gate 75 %, `*_templ.go` exkl.); `make generate` + `make web` nach jeder templ/CSS-Änderung.
- **Live-Dogfood** (Dev-Stack): Upload (Bild + PDF) → in Doc `![[slug]]` einbetten → Figur/Chip + Abb.-Nummer korrekt → Deep-Link `#slug` scrollt zur Sektion → Editor-Picker fügt beide Token-Arten ein → MCP-Upload eines Agenten landet in der Galerie → owner-fremder Zugriff scheitert.

## 14. Datei-Änderungs-Karte (Orientierung für writing-plans)

- **domain:** `artifact.go` (+ `_test.go`), `errors.go` (`ErrArtifactNotFound`), MIME-Allowlist + `ArtifactSlug`.
- **ports:** `ArtifactStore`-Interface + Sentinel.
- **usecase:** `upload_artifact.go`, `list_artifacts.go`, `delete_artifact.go`, `get_artifact.go` (je Datei, „keine Monolithen"); Fakes in `testutil`.
- **adapter/pgstore:** `artifacts.go`, `migrations/0031_artifacts.sql`.
- **adapter/httpserver:** REST-Handler + Serve-Route + Web-Handler (Galerie, Editor-Picker-Fragmente); `server.go`-Felder.
- **adapter/webui:** `wikilink.go` (Embed-Parser + Artefakt-Renderer + Heading-Slugger), Sanitizer-Policy, `document.templ`/Figur-Komponente, `cockpit_*`/Galerie, `editor.templ`/Werkzeugleiste, `static/toc.js`, `static/js/editor-insert.js`, i18n-Katalog.
- **adapter/apiclient:** Artefakt-Verben.
- **cmd/flow:** `artifact.go` (Cobra).
- **cmd/flow-mcp:** drei Tools.
- **cmd/flow-server/main.go:** Wiring.

## 15. Reihenfolge (Slicing-Hinweis)

Grob abhängigkeitsgeleitet (Blätter zuerst), finaler Task = Wiring+Gate:
1. Domain + Migration + pgstore + Ports/Fakes (Storage-Fundament).
2. Usecases + REST + apiclient + Serve-Route + SSE.
3. Inline-Referenz (`![[…]]`-Parser + Figur/Chip-Renderer + Sanitizer) in der Lese-Ebene.
4. Deep-Links (Heading-Slugger + Sanitizer + toc.js + Hover-Anker).
5. Cockpit-Galerie (Upload/Umbenennen/Löschen + SSE).
6. Editor-Einfügehelfer (zwei Picker + Insert-JS).
7. flow-mcp + CLI-Verben.
8. Wiring (main.go) + `make ci` + Live-Dogfood.

Je Task TDD + `make ci` grün + unabhängig testbar; Reviews wie in L2–L5 (Task-Reviewer je Task, Slice-Ende Final-Reviewer + Mockup-Auditor).
