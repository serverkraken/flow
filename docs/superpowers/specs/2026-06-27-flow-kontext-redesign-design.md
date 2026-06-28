# flow Kontext-Redesign — Design-Übersicht

**Datum:** 2026-06-27 · **Status:** Architektur + alle Detail-Entscheidungen bestätigt (siehe Ende); bereit für Baustein-Specs · **Branch:** rebuild

## Warum

flow soll der **Single Source of Truth für Claude-Kontext** werden — CLAUDE.md (Instructions), Memory, aktiver Arbeitsstand und Pläne — **geräteübergreifend**. Lokal soll nichts Loses mehr liegen (bis auf einen minimalen Seed), und der Kontext **verschiedener Projekte/Mandate darf sich nicht vermischen**.

Heute funktioniert nur der Lese-/Abruf-Pfad (78 Spec/Plan-Docs liegen in flow). Es fehlt: Memory kommt nie an (0 Memory-Docs), der Write hängt an Disziplin; es gibt kein Start-Kontext-Protokoll; das Projekt-Modell ist flach und bildet die reale Struktur falsch ab.

## Zwei Grundachsen

```
Hierarchie (vertikal)   → Containment / ISOLATION   global → engagement → vorhaben → repo
Tags        (horizontal) → Thema / QUER-SCHNITT      django, terraform, postgres … über ALLES
```

- **Hierarchie** entscheidet *Sichtbarkeit/Isolation*: wer gehört wohin, was darf in eine Session.
- **Tags** sind eine orthogonale, *neutrale* Klassifikation über alle Entitäten. Ob Tags Isolations-Grenzen überspringen, entscheidet der **Consumer**, nicht das Tag.

## Mechanik-Fundament (recherchiert, verifiziert)

- **SessionStart-Hook + `additionalContext`** ist der Lade-Mechanismus: injiziert Text *vor der ersten Antwort*, „like CLAUDE.md", blockierend. CLAUDE.md darf lokal fehlen.
- `additionalContext` ist laut Doku *nicht garantiert* OVERRIDE-stark → **HARD RULES bleiben ein echtes, winziges lokales `~/.claude/CLAUDE.md`** (der „Seed").
- Auto-Memory via MCP existiert **nicht**; MCP-Resources laden nicht automatisch. Der Hook ist der einzige dokumentierte Weg.
- Das **25-KB-Limit** der nativen `MEMORY.md` gilt nur für die Datei, **nicht für Hook-Output** → via Hook umgangen.
- **Drei Schichten:** Seed (lokal, statisch) · flow-Client-Cache (lokal, unsichtbar, Offline-Fallback) · flow-Server (SSOT).

---

## Baustein 1 — Hierarchie + Bindings (Struktur)

**Modell (D1 ✓: rekursive Gruppen über Repo-Blättern):**
- `workspace`-Entität, **rekursiv** (`parent_id`). Engagement, Vorhaben, Sub-Vorhaben sind dieselbe Sorte, verschieden tief.
- `repo` = **Blatt** (git-origin, Claude-Session, activeContext leben hier).
- **Engagement** = der oberste Workspace eines Astes; trägt `rate`/Worktime-Semantik (heute fälschlich am Projekt).
- Jedes Repo hat Vorfahren bis zur Wurzel — Default-Engagement **„Privat"**, nichts hängt „frei" *(D2 ✓)*.

**Doc-Identität & path (verbessert):**
- `id` (ULID) bleibt der stabile **Primary Key** — path ist *nicht* PK (`0006_documents.sql:3`).
- path-Eindeutigkeit wandert von `(owner, project_id, path)` (`0006:17-18`) auf **`(owner, node_id, path)`** — dieselbe `instruction` (CLAUDE.md) existiert damit unabhängig auf *jeder* Ebene (global/engagement/vorhaben/repo); der Bootstrap merged sie entlang der Ahnenkette.
- path wird zum schlanken **Namen im Knoten** (`claude`, `active-context`) statt Identität + Pseudo-Hierarchie (`specs/…`) in einem. Drei Rollen getrennt: `id`=PK · `node`=Scope · `path`=Name.

**Repo-Zuordnung pro PC — Konzept existiert (`project_bindings`, Migr. 0011):**
- **git-origin-Binding** = geräteübergreifend (gleicher Origin → gleiches Repo, egal Pfad/PC). Trägt Multi-Device.
- **path-Binding** = per-PC (für Repos ohne brauchbaren Origin), via `machine-id`.
- Binding hängt am **Repo-Blatt**.

**Ist-Stand / Lücken:** `projects` ist flach (`0006_documents.sql` referenziert `projects(id)`); `project_id` am Doc ist ein Single-Pointer. Bindings + git-origin-Resolution existieren bereits.

**Aufgaben:** workspace-Entität + Migration · Repos als Blätter + Binding einhängen · Resolution: git-origin → Repo → **Ahnenkette** · `rate`/Worktime auf Engagement-Ebene umhängen *(D3 ✓: Engagement)* · Bestandsdaten einsortieren („RTL Extern" → Engagement, Repos drunter).

## Baustein 2 — Tag-System (generisch, Quer-Achse)

- **Generisches, polymorphes Tag-System**: Tag-Entität + Zuordnung zu beliebigen Taggables (Doc, Workspace, Repo, WorktimeSession, Asset).
- **Frontmatter wird abgeschafft.** Tags (und Metadaten) sind **rein strukturiert** — als Parameter/Relation gesetzt, nicht im Body. **Body = reiner Inhalt.** Einheitlich für alle Taggables.
- Tags sind **neutral** — keine Isolations-Logik im Tag selbst (die lebt im Consumer).
- Von Claude **mitgenutzt**: ich *setze* Tags beim Schreiben (als Parameter) und *nutze* sie beim Ziehen.

**Ist-Stand / Lücken:** Tags existieren **nur auf Docs**, aus **YAML-Frontmatter** geparst, GIN-indiziert (`0008_documents_tags_gin.sql`); `flow_create_doc` nimmt Tags *nur* via Frontmatter, keinen `tags`-Parameter. Nicht polymorph, nicht an Sessions/Workspaces.

**Aufgaben:** Tag-Entität + polymorphe Zuordnung · **`tags` als expliziter API-Parameter** (MCP + REST) statt Frontmatter-Parsing · **Migration: Bestand konvertieren** (Frontmatter parsen → strukturierte Tags → aus Body strippen) · Vault-**Import** liest fremdes Frontmatter nur noch als Konvertierungsschritt beim Einlesen · Tagging in WebUI/TUI überall · Worktime-Session-Tags + Tag-Auswertung.

## Baustein 3 — Kontext-Store (nutzt 1 + 2)

- **Compose-Endpoint** `GET /context?repo=X` (+ `flow_get_context` MCP-Tool): walkt die **Ahnenkette** *und* zieht **Tag-Matches**, gruppiert nach Typ, **ein** Round-Trip, **token-budgetiert**. Kapselt die `global`≠`none`-Falle intern.
- **path-Upsert** (`ON CONFLICT (owner,node_id,path) DO UPDATE`) — activeContext idempotent fortschreiben.
- **Symmetrie:** SessionStart-Hook **lädt**, Stop-Hook **speichert** activeContext (stößt Flush an: „wo war ich, was offen, nächster Schritt").
- **`flow context` CLI** + Client-Cache (Offline-Fallback).
- **Einschrittiger Memory-Write**: ich schreibe nur noch nach flow (natives Auto-Memory aus). Kein vergessener zweiter Spiegel-Schritt.
- **`instruction` vs. `memory` sauber getrennt**: Regeln (befolgen, bindend) vs. Fakten (kennen, Referenz) — verschiedene Autorität/Lebensdauer.
- **activeContext** = `type=memory`, feste path-Konvention `active-context`, repo-scoped *(D4 ✓)*.
- **Bootstrap-Scope** = instructions + memories + activeContext; Specs/Plans bleiben on-demand *(D5 ✓)*.
- **Tag-Reichweite** *(D7 ✓)*: Tags ziehen nur **global-getaggtes** Allgemeinwissen quer; Engagement-Wissen bleibt strikt hierarchisch (volle Tag×Hierarchie erst bei realem Bedarf).
- **Token-Budget** *(D8 ✓)*: immer-laden = instructions + activeContext + Repo/Vorhaben-Memories; engagement/global nach Relevanz mit Cap; Specs/Assets abrufbar. Exakte Zahl am Kontext-Inspektor kalibrieren.

**Ist-Stand / Lücken:** Kein compose/bootstrap-Endpoint, kein path-Upsert, **kein query-freies Ranking**, **kein Pinning/Priorität**, kein Cross-Project-OR-Filter, keine Optimistic Concurrency. Such-Arme (FTS+trgm+vector+RRF) + SSE live-sync stehen.

**Migration des Ist-Stands:** lokale Memory (~40 Files) + globale CLAUDE.md + `CLAUDE-*.md` → flow, **klassifiziert nach Scope** (global / engagement / vorhaben / repo) und Typ (instruction vs. memory).

---

## Querschnitt A — Memory-Lifecycle (gegen Verrottung)

Wir lösen Schreiben + Lesen, aber Memory *altert*. Symptome heute real: `MEMORY.md` 32 KB / 24 KB (über Limit); Claude warnt selbst „verify before trusting". Drei Mechanismen:

- **Provenance** — jedes Memory trägt *wann + woher* (Repo, evtl. Commit). Macht „noch wahr?" beantwortbar.
- **Verfall** — `dauerhaft` vs. `flüchtig`; Flüchtiges bekommt Review-/Ablauf-Trigger.
- **Verdichtung** *(D9 ✓: halbautomatisch)* — Schwellwert-Warnung + Aufräum-Ansicht als Trigger, Verdichtungs-Subagent manuell anstoßbar; Voll-Automatik (Cron) später.
- **Aufräumen = markieren, nicht löschen** *(D11 ✓)* — Veraltetes bekommt **Status `veraltet`** (raus aus Bootstrap + Haupt-Ansichten, aber auffindbar + reversibel), optional „ersetzt durch [[nachfolger]]". **Hard-Delete** ist die seltene, bewusst-**menschliche** Ausnahme (Papierkorb/Tombstone) — wichtig, weil Verdichtung teils ein *Agent* macht: Markieren ist sicher und umkehrbar, Löschen nicht. Lifecycle: `aktiv → veraltet → (selten) gelöscht`.

## Querschnitt B — UI-Transparenz (Zustand sichtbar machen)

WebUI **und** TUI müssen den Zustand des Kontexts **menschenlesbar** zeigen — „was ist was, was ist aktiv, was ist veraltet". Nutzt das vorhandene `ui/badge` · `ui/chip` · `kindcolor`/`kindglyph` (Docs-Kompendium-Look).

- **Pro Doc:** Typ-Badge (instruction/memory/activeContext/spec/plan) · Scope-/Ebenen-Anzeige (global … repo) · Tag-Chips · **Lifecycle-Status** (frisch / review-fällig / **veraltet**) · Provenance (wann/woher).
- **Pro Repo/Workspace:** die **Hierarchie als Baum**; welche Docs hängen auf welcher Ebene; welches Doc ist der activeContext.
- **Bestehende Listen umstellen:** Heute flache Doc-Listen (z.B. die `agent`/Spec-Ansicht in der WebUI ist eine flache Liste) zeigen künftig die **Hierarchie** (Doc unter seinem Workspace/Repo) + **Typ** + **Lifecycle-Status** statt einer flachen chronologischen Reihe. „Was was ist" wird auf einen Blick lesbar.
- **Sicht-Achsen (orthogonal, kombinierbar):** **Gruppieren** nach Hierarchie/Typ/Tag · **Sortieren** nach Neuheit (`created` vs. `updated` getrennt — *was ist neu*) · **Filtern** nach Status/Scope/Tag/Typ. Sortierung (Zeit) und Gruppierung (Struktur) sind orthogonal; die UI kombiniert sie (z.B. Baum nach Workspace, innerhalb nach Neuheit, Veraltetes ausgegraut).
- **Kontext-Inspektor** *(Schlüssel-Ansicht):* „**Was würde der Bootstrap für dieses Repo laden?**" — die komponierte Ahnenkette + Tag-Matches, gruppiert nach Ebene/Typ, mit **Token-Budget-Anzeige** und veralteten Einträgen markiert. Macht den ganzen Start-Kontext transparent und gibt dem Menschen den Hebel, mit mir gemeinsam zu pflegen.
- **Globale Suche/Filter über alles:** quer über Scopes, Typen, Tags hinweg suchen — „finde X, egal wo es hängt", nicht aufs aktuelle Projekt beschränkt.
- **Aufräum-Ansicht:** eine eigene Sicht, die alles **Veraltete/Review-Fällige** sammelt — der gemeinsame Pflege-Stapel. Verbindet Querschnitt A: der Mensch *sieht* die Verrottung, wir räumen zusammen auf.

---

## Baustein 4 — Assets / Artefakte (nicht-textueller Kontext) · **Phase 2** *(D10 ✓)*

Kontext ist nicht nur Text: Diagramme, Screenshots, Whiteboard-Fotos, Mockups liefern Kontext, den Markdown schlecht erfasst. Modell ist vorgedacht, Umsetzung **nach B1–B3**.

- **Asset = Kontext-Objekt wie ein Doc**, nur Binär-Payload statt Markdown-Body. Lebt in **derselben Hierarchie** (Scope) und trägt **Tags + Provenance + Lifecycle-Status** — dieselben Achsen.
- **Asset-Store**: Blob + Metadaten-Zeile. Von Docs **referenziert** (Markdown-Link/Wikilink auf `asset://id`).
- **Bootstrap**: Assets sind **nicht** im Payload (Größe + Vision-Kosten). Nur **Referenz/Metadaten** sichtbar; der Binär-Inhalt wird **on-demand** geladen (geschichtete Ladung, Querschnitt B / Token-Budget).
- **Multimodal nutzbar**: Claude *sieht* Bilder — ein Abruf-Weg (`flow_get_asset` → Bytes/URL → Vision) macht Bild-Artefakte zu echtem, nutzbarem Kontext, nicht nur Anhang.
- **UI**: Thumbnails/Vorschau mit denselben Status/Scope/Tag-Badges (Querschnitt B).
- **Bestandsfall**: `docs/superpowers/specs/assets/` existiert lokal schon → mit-migrieren.

*(Storage-Backend — Postgres-bytea vs. Objektspeicher — wird beim eigenen Phase-2-Spec entschieden.)*

---

## Worktime als Consumer

Nutzt Baustein 1 (Buchungsebene = **Engagement** — *D3 ✓: keine Repo-Granularität*) + Baustein 2 (Session-Tags + Tag-Auswertung, z.B. „gesamte django-Zeit über alle Engagements").

## Reihenfolge / Abhängigkeiten

```
B1 Hierarchie+Bindings   ──▶  B3 Kontext-Store   (braucht Ahnenkette)
B2 Tag-System            ──▶  B3 (Tag-Match)      (quasi-unabhängig, vor/parallel zu B3)
Querschnitt A (Lifecycle) + B (UI-Transparenz)    queren B1–B3
Migration des Ist-Stands  ──▶  nach B1+B2+B3-Kern
B4 Assets                ──▶  PHASE 2 (nach B1–B3)
```

Grobschnitt: **B1 → B2 → B3-Kern (Compose+Hook+Write) → Lifecycle/Budget/UI-Verfeinerung → Migration.** Dann **Phase 2: B4 Assets.** Jeder Baustein bekommt sein eigenes Spec → Plan → Umsetzung.

## Entscheidungen (alle getroffen 2026-06-27)

- **D1** ✓ Hierarchie-Bauform = rekursive Workspaces über Repo-Blättern
- **D2** ✓ Engagement-Pflicht + Default „Privat"
- **D3** ✓ Worktime bleibt auf **Engagement**-Ebene (keine Repo-Granularität)
- **D4** ✓ activeContext = `memory` + path `active-context`
- **D5** ✓ Bootstrap-Scope = instructions + memories + activeContext; Specs on-demand
- **D6** ✓ Seed = nur HARD RULES als echtes lokales CLAUDE.md
- **D7** ✓ Tag-Reichweite = **global-quer** (Engagement-Wissen strikt hierarchisch; volle Tag×Hierarchie erst bei realem Bedarf)
- **D8** ✓ Token-Budget = Konzept + Heuristik; exakte Zahl am Kontext-Inspektor kalibrieren
- **D9** ✓ Verdichtung = halbautomatisch (Warnung + Aufräum-Trigger + manueller Subagent); Voll-Automatik später
- **D10** ✓ Assets = **Phase 2** (Modell vorgedacht, Umsetzung nach B1–B3)
- **D11** ✓ Aufräumen = Status `veraltet` (soft, reversibel, raus aus Bootstrap); Hard-Delete nur als seltener menschlicher Akt
