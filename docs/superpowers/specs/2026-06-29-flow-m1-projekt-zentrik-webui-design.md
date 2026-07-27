# flow M1 — Projekt-zentrisches flow (WebUI-Reframe · Kristall-Identität · AI-native) — Design Spec

- **Datum:** 2026-06-29
- **Branch:** `rebuild` (Implementierung in eigenem Worktree off `rebuild`)
- **Status:** Draft — zur Review
- **Methode:** Brainstorm (Visual Companion, Mockups unter `.superpowers/brainstorm/`) → diese Spec → `writing-plans` pro Slice.
- **Vorläufer:** WebUI „Studio"-Overhaul (Slices 0–2, `2026-06-23-flow-webui-overhaul-design`) — wird hier abgelöst/umgeordnet.

---

## 1. Kontext & Ziel

flow ist ein **multi-tenant** Wissens- + Worktime-Produkt für **Menschen UND AI-Agents** (über den MCP-Server). Die bestehende WebUI wirkt „steril/zu technisch" und ist als gleichrangige Tab-Sammlung gewachsen (Worktime · Wissen · Projekte · Stats nebeneinander).

M1 ordnet die WebUI **projekt-zentrisch** neu: **Projekte (nodes) sind die Spine**; Wissen/Worktime/Stats werden zu **Facetten** eines Projekts. Ein **Home** zeigt „woran arbeite ich gerade" + Aktivität. Eine neue, unverwechselbare **Identität („Kristall")** ersetzt den klinischen Look. Und eine **Kontext-Vorschau** macht sichtbar, was ein Agent als Kontext erhält — kuratierbar.

M1 wird **teilen-bewusst** entworfen (Hybrid-Sharing kommt in M2/M3), implementiert das Teilen-Backend aber **noch nicht**.

### Erfolgskriterien
1. Der **Projektbaum ist die Navigation**; jeder Knoten hat ein Cockpit mit eigenem Wissen/Worktime/Stats/Kontext.
2. **Home** = Timer (Start sofort, Projekt beim Stop) + **Logstream mit Actor** + allgemeine Stats + neueste Wissensartikel.
3. **Jeder Node buchbar** (Rate über Ahnenkette, Stats über Subtree-Rollup); **Job vs. Privat** über „zählt aufs Soll"-Flag.
4. **Kristall-Identität**: Twilight-Backdrop, runde Glas-Karten, farbige Projekt-Ecke, Hexagon-Logo, **keine Emojis**, light+dark.
5. **Kontext-Vorschau** („AI-Sicht") read-only + Pin/Priorität.
6. **Mermaid** im Markdown.
7. Strukturfehler der bisherigen WebUI bereinigt.

---

## 2. Nicht-Ziele (M1)
- **Sharing-Backend** (Permissions, Teams, Einzel-Freigaben) → **M2** (Fundament) + **M3** (UI/Teams). M1 legt nur Hooks/Affordances an.
- Keine neue Doc-Taxonomie; keine Migration bestehender Doc-IDs/Pfade.
- Kein SPA-Routing; server-rendered **templ + htmx** bleibt.
- **TUI/CLI** beim Start/Stop-Zuordnungs-Zeitpunkt unverändert (siehe §6).

---

## 3. Information-Architektur

Sidebar-Navigation („Modell C": aufklappbarer Projektbaum, Dashboard = Home):
- **Home** (`/`) — Landeseite (§5).
- **Projekte** — Baum `Engagement → Vorhaben → Repo`; jeder Knoten → **Cockpit** (§7).
- **Wissen** (global) — projektloses + projektübergreifendes Wissen (Daily/Frei/System) + Suche (§8).
- Utilities: **Zeit** (Woche/Historie-Detail), **Frei**, **Export**, **Einstellungen**.
- **Stats-Seite entfällt**: allgemeine Stats auf Home; Soll/Tagesziel-Editor → **Einstellungen**; per-Projekt-Stats → **Cockpit**.
- **Persistenter Timer** lebt auf Home (Dashboard = Home, immer 1 Klick); optional ein kleiner Lauf-Indikator in der Sidebar.

**Wissen-Hybrid:** pro Knoten im Cockpit-Tab „Wissen" **und** global unter „Wissen". Nichts ist heimatlos; der Baum bleibt die Spine.

**Mobile:** Bottom-Nav mit Home/Projekte/Wissen + Mehr-Menü für Utilities (behebt heutigen Mobile-Gap, bei dem Frei/Export/Einstellungen unerreichbar sind).

---

## 4. Visuelle Identität — „Kristall"

- **Backdrop:** ruhiger Twilight-Verlauf (Indigo→Magenta→Teal) + **Low-Poly-Facetten** (SVG-Polygone, ~10–20 % Deckkraft) + weiche radiale Glows; fixed. Helles Pendant mit blasseren Facetten.
- **Form:** **runde** Glas-Karten (`backdrop-blur`), bewusst NICHT kantig. Signatur-Detail: **farbige Projekt-Ecke** (dreieckige Hue-Fläche oben links, via `overflow:hidden`+Rundung geclippt).
- **Logo:** **Hexagon-Marke** (geschliffenes Sechseck, Verlauf Lavender→Cyan) + Wortmarke „flow". Hexagon = Knoten/Zelle als Motiv.
- **Avatare:** Mensch = Kreis; **AI-Agent = Hexagon** (Actor-Unterscheidung, §9).
- **Keine Emojis** — ausschließlich SVG/geometrische Marken (Memory `feedback_no_icons`). Farbe ist erwünscht: Projekt-Hues leben (Rails, Ecken, Timeline-Blöcke, Bars).
- **Umsetzung:** Erweiterung `web/tailwind.css` (`@theme`) um Twilight/Glass-Palette + Facetten/Backdrop-Utilities; **light+dark beide** pflegen. Rein Tailwind v4 + templ + CSS/SVG (Gradients, `clip-path`, `backdrop-blur`, SVG-Noise) — kein Canvas/WebGL; **WCAG-AA**-Kontrast. Font-Beibehaltung (Clash Display/Inter/JetBrains Mono) vs. Wechsel = Detail im Identity-Slice.

---

## 5. Home (Dashboard)
- **Timer-Hero:** Start **sofort** (kein Projekt) — Timer läuft „anonym"; **Stop → Projekt-Picker** (Fuzzy + MRU + inline-create). Globales Tastenkürzel; Mobile-Control in der Bottom-Leiste. Backend unverändert (§6).
- **Logstream** (neuer Activity-Feed, §9) mit **Actor** (Mensch = Kreis, Agent = Hexagon): Timer/Buchung/Doc-erstellt/-bearbeitet/Frei … chronologisch.
- **Allgemeine Stats:** Heute/Monat + Woche-**Job-Soll** (über/unter), Pace; **Privat getrennt** ausgewiesen („zählt nicht").
- **Neueste Wissensartikel:** Karten (farbige Projekt-Ecke) mit letztem Actor, direkt anklickbar.

---

## 6. Worktime-Modell
- **Start/Stop-Zeitpunkt unverändert** (im Code verifiziert): `StartSession(nodeID *string = nil)` → Start **ohne** Projekt; `StopSession(nodeID required)` → Projekt **beim Stop** (Fuzzy-Picker). Gilt heute schon **identisch in WebUI + TUI** → **kein TUI-/Backend-Change** am Zuordnungs-Zeitpunkt.
- **Jeder Node buchbar:** `requireEngagement` → `requireBookable` (Engagement/Vorhaben/Repo; Branch später).
  - **Rate-Auflösung:** Ahnenkette hoch — nächster Node mit Rate gewinnt.
  - **Stats-Rollup:** Knoten-Total = eigene Sessions + alle Nachfahren (jede Session genau ein Node → kein Doppelzählen).
  - Bestehende Sessions (auf Engagements) bleiben gültig.
- **Job/Privat-Soll:** Flag pro Engagement `countsTowardTarget bool` („zählt aufs Soll"). Das Soll-Gauge (Home-Woche, Pace, Burndown) summiert **nur** geflaggte Zeit; Privat wird getrackt + bleibt sichtbar, **außerhalb** des Gauges. Annahme: **ein** Soll (Job).

---

## 7. Projekt-Cockpit
- **Aufbau:** Übersicht + **Facetten-Tabs**.
- **Header:** Identität (Name, Art, Ahnen-Pfad, **geerbter Satz**), **Timer auf diesen Node** starten, **Rollup-Kennzahlen** (eigene + Nachfahren).
- **Tabs:** **Übersicht** (Karten-Summary) · **Worktime** · **Wissen** (Projekt-Notizen) · **Stats** · **Struktur** (Unterprojekte/Bindings) · **Kontext** (AI-Sicht, §10).
- Behebt den heutigen „read-only Bindings"-Gap (Bindings hinzufügen/entfernen), soweit ohne Sharing-Backend sinnvoll.

---

## 8. Wissen
- **Global** (`/wissen`): Daily-Timeline, Projekt-gruppiert, Frei, System; Tag-Chips; Volltext + Semantik-Suche (existiert: FTS + pg_trgm + pgvector).
- **Pro Projekt:** Cockpit-Tab.
- **Mermaid:** ` ```mermaid `-Blöcke als gerendertes Diagramm — **vendored `mermaid.js`** (kein CDN) + Renderer-Tweak (`internal/adapter/webui/wikilink.go`: mermaid-Fence → `<pre class="mermaid">` statt chroma) + Init-Aufruf.
- Markdown-Parität bleibt: GFM/Footnotes/Callouts/Wikilinks/Backlinks/chroma.

---

## 9. Activity-Feed + Actor (neue Komponente)
- **Persistierte Aktivitäts-Events** (Quelle: bestehende SSE `session.*`/`document.*`/`node.*`) mit **Actor** = Auth-Subjekt (Mensch-Login **oder** MCP-Agent-Token), Zeit, Ziel.
- **Feed-Query** (chronologisch, paginiert), pro Owner/Scope.
- **UI:** Home-Logstream; optional Cockpit-Tab.
- **Datenmodell-Skizze:** `activity(owner_id, actor_ref, actor_kind {human|agent}, kind, target_ref, at)`. Actor-Identität Mensch vs. Agent unterscheidbar (Hexagon-Avatar für Agent).
- Genauer Event-Katalog = Detail im Slice-Plan.

---

## 10. Kontext-Vorschau / „AI-Sicht" (letzte M1-Slice)
- **Cockpit-Tab „Kontext":** zeigt das `flow_get_context`-Ergebnis für den Node — **komponierter Markdown**, **Token-Budget**-Anzeige, **enthaltene** Docs (gerankt, mit Token-Kosten), **verworfene** (über Budget), **Pins**.
- **Kuratierung:** **Pin** (immer rein, umgeht Cap) · **Priorität** (Hoch/Normal/Niedrig, Klick/Drag) als zusätzliches Rank-Signal · manuell ein-/ausschließen.
- **Backend:** cap+rank-Komposition existiert (Memory `project_flow_rebuild_cap_rank`, Budget ~12k, Pins bypass); **neu** = **Prioritäts-Feld** (global pro Doc **oder** pro `(Doc,Kontext)` — im Slice zu entscheiden) + Kuratier-API + read-only Compose-Exposition für die WebUI.
- **Reihenfolge:** erst read-only Ansicht, dann Pin/Priorität.

---

## 11. Multi-Tenant / Sharing-Awareness (Hooks; Backend in M2/M3)
- M1 wirkt produkt-tauglich: **Account/Avatar** oben rechts, **Workspace-Hinweis** („Privat ▾").
- IA lässt **Workspace-Dimension** + „geteilt mit mir" + **Kollaborator-Anzeige** zu (Platzhalter), ohne sie zu implementieren.
- Tenancy bleibt owner-scoped. **Hybrid-Sharing** (persönlich + Teams + Einzel) = **M2** (Rechte-/Datenmodell, Permissions durch alle Queries) + **M3** (UI/Teams/Einladen/„geteilt mit mir").

---

## 12. Strukturelle Bereinigung (aus der Forensik)
- **Export-Seite** auf AppShell/Tokens heben (raus aus rohem HTML, CDN-htmx, slate-Farben, alter `Nav()`); `nav.templ`-Zombie löschen.
- Tote Komponenten (`CategoryStrip`, `Glyph`) entfernen oder verdrahten; `Badge`-/Breadcrumb-Duplikate vereinen.
- Nav-Aktivierung + Mobile-Erreichbarkeit fixen; toten `/einstellungen`-Link → echte Seite.
- Naming-Leak `projectId`/`FuzzyProjectVM`/`historieProjectPickers` → node-konsistent (pragmatisch, kein Big-Bang).

---

## 13. Slices (Reihenfolge; jede Slice = eigener Plan + Review + `make ci` + Live-Done-Gate)
1. **Worktime-Modell** (Backend-Kern): `requireBookable`, Rate-Ahnenkette, Subtree-Rollup, Job/Privat-Soll-Flag + Soll-Scoping. *(Unblockt Home/Cockpit-Zahlen.)*
2. **Identität „Kristall"** (Design-System): Twilight/Glass-Tokens, Backdrop, Hexagon-Logo, farbige Ecke, keine Emojis, light+dark; `/ui`-Styleguide.
3. **Shell & IA-Reframe:** Sidebar mit Projektbaum-Spine + globalem Wissen, **Home-Route**, Stats-Seite raus, Export auf Shell, Strukturfehler.
4. **Home:** Timer-Hero (Start/Stop wie gehabt) + allgemeine Stats + neueste Wissensartikel.
5. **Activity-Feed + Actor:** Datenmodell/Store/Feed-API + Logstream auf Home + Actor (Mensch/Agent).
6. **Cockpit:** Übersicht + Tabs (Worktime/Wissen/Stats/Struktur), per-Node-Timer, Rollup, Bindings-Mgmt.
7. **Wissen + Mermaid:** globale Wissen-Fläche + Cockpit-Tab + Mermaid-Rendering.
8. **Kontext-Vorschau:** read-only AI-Sicht → Pin/Priorität.

Jede Slice **subagent-driven**, mit **Wiring-Verifikation** (Composition-Root/`main.go` + curl-Smoke jeder neuen Route) als Abschluss (Memory `feedback_plan_main_wiring_task`).

---

## 14. Testing & Done-Gate
- **TDD**; `make ci` grün pro Slice (Coverage-Gate beachten; `make web` für app.css nicht vergessen — CI baut es nicht).
- **Live-Done-Gate** vs. Dev-Stack (Postgres + Dex) je Slice end-to-end (curl-smoke + Browser-Dogfood): u. a. Multi-Day-Split, **Rollup-Korrektheit**, **Soll-Scoping** (Privat zählt nicht), **Actor**-Anzeige, **Kontext-Compose** + Pin/Priorität.
- **Holistic-Review (Opus)** am Ende jeder Slice; finaler Branch-Review vor Merge nach `rebuild`.

---

## 15. Offene Detail-Entscheidungen (in den Slice-Plänen zu fixieren)
- Prioritäts-Feld **global pro Doc** vs. **pro (Doc, Kontext)**.
- Font-Beibehaltung vs. -Wechsel in „Kristall".
- Genauer Event-Katalog des Activity-Feeds + ob Cockpit einen Activity-Tab bekommt.
- Umfang Bindings-Management ohne Sharing-Backend.
- Persistenter Sidebar-Lauf-Indikator: ja/nein.
