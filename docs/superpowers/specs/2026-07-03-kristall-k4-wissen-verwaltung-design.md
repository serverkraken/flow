# Kristall K4 — Sweep Wissen & Verwaltung + Wissen-Rollup + Login — Design Spec

> Datum: 2026-07-03 · Status: **APPROVED** (brainstormed, Soenne „Go") · Branch: `cockpit-story` · Base: `a154aaa`
> Slice K4 des Kristall-Redesign-Programms. Umbrella: [[specs/2026-07-02-kristall-redesign-design]] §8 (Sweep), §4 (Containment/Rollup), §13 (offene Detail-Entscheidungen: Login).
> Konsumiert: K1 (Tokens/Glas-Komponenten), K2 (Subtree-Query + Containment), K3 (Sweep-Muster für glass-Karten).

---

## 0. Grundsätze (erben aus Umbrella §0 — binden jede Entscheidung)

- **flow ist multi-tenant.** Jeder Datenzugriff owner-scoped; „ist nur ein User"-Begründungen sind unzulässig, auch in Reviews/Trade-offs. Kein globaler Cache ohne Tenant-Schlüssel.
- **Menschen UND AI-Agents** sind gleichberechtigte Akteure.
- **Design muss einfach änderbar bleiben** (K1-Dogfood-Prinzip, `feedback_design_must_stay_easily_changeable`): K4 verwendet **nur** Tokens, Primitives und benannte Klassen. Neue arbitrary one-off-Utilities (z. B. `h-[26px]`, ad-hoc-Farbwerte) sind ein **Review-Finding**, kein akzeptiertes Muster.
- Kristall-Sprache: Twilight-Gradient + Facets, Glas-Karten (`backdrop-filter: blur`), Form-Codierung ● Engagement / ◆ Vorhaben / ⬡ Repo, `tabular-nums` für Zahlen, keine Emoji-Piktogramme, Motion hinter `prefers-reduced-motion`, i18n **de+en parity**, `verify-css`/`verify-no-popups` bleiben scharf, kein `make fmt`.

## 1. Kontext & Ziel

K1–K3 haben Shell, Cockpit und den kompletten Worktime-Bereich auf die Kristall-Sprache gehoben. K4 zieht die **Wissen**- und **Verwaltung**-Flächen nach, komplettiert die Containment-Regel (§4 der Umbrella) im **Wissen-Tab** des Cockpits (Wissen-Rollup) und liefert den ersten flow-eigenen **Kristall-Moment im Auth-Flow**. Danach steht die komplette App bis auf die K5-Politur (Light-Theme, Mobile, A11y, Gate) auf Kristall.

**Ausgangslage (verifiziert 2026-07-03):** Alle fünf Sweep-Flächen sind noch Studio-Styling — `document.templ`, `editor.templ`, `nodes.templ`, `einstellungen.templ`, `wissen.templ` nutzen `bg-surface`/`border-line`/`shadow-soft`, null `glass`; `wissen.templ` und `nodes.templ` haben rohe `bg-ink`-Buttons statt `components.Button`. Der Cockpit-**Wissen-Tab** ist heute **own-only** (`webui_cockpit.go:101` → `ListDocuments.Execute(u.ID, &n.ID, nil)`), während die Übersicht-„Zuletzt-Wissen"-Karte bereits Subtree-gefiltert ist (`TopDocs(docs, subtreeIDs)`). `/auth/login` ist ein reiner 302-Redirect zum OIDC-Provider (Dex/Authentik) — flow rendert **keine** eigene Login-Seite; die einzigen flow-eigenen Auth-Momente sind heute plaintext-`http.Error` (`forbidden` 403, `auth failed` 401, `bad state` 400).

## 2. Entschiedene Fragen (Soenne, 2026-07-03)

| # | Frage | Entscheidung |
|---|---|---|
| 1 | Slicing | **Ein Plan** (K3-Rhythmus), Tasks enden mit Main-Wiring+Gate-Task |
| 2 | Login-Scope (§13) | **Fehlerseiten + Logout-Landing.** Happy-Path `/auth/login` bleibt Auto-302 zu Dex (kein Extra-Klick). Kein Landing-Gate vor dem normalen Login. |
| 3 | Wissen-Rollup-Reichweite | Nur der **Cockpit-Wissen-Tab** bekommt den Toggle. Übersicht-Karte bleibt Subtree ohne Toggle; globale `/wissen`-Seite bleibt global. |

## 3. Scope A — Glass-Sweep (mechanisch, reuse-only)

Jede Fläche wandert auf die bestehenden Kristall-Primitives (K1). **Keine** neue Komponente, **keine** arbitrary one-offs. Funktion unverändert; nur Optik + Button-/Chip-Vereinheitlichung.

| Fläche | Datei(en) | Was wird getauscht |
|---|---|---|
| **Wissen-Übersicht + Kategorie** | `wissen.templ` | Kategorie-Karten `bg-surface`→Glas (`components.Card`-Optik); Doc-Row-Listen→Glas; Suchleiste→Glas-Input; „Neu"-Button (`bg-ink px-5…`, 3×)→`components.Button` CTA; Tag-Chips→Pill-`Chip`; Count-Badges→Pill; `EmptyState` bereits Komponente (bleibt). Kategorie-Nav-Pills auf Pill-Leiste. |
| **Dokument** | `document.templ` | Prose-Container auf Glas (`markdownprose` bleibt der Renderer); TOC (`components.toc`) auf Glas; Backlinks-Karte Glas; Meta/Header Kristall. |
| **Editor** | `editor.templ` | Formfelder auf Kristall-Formsprache (Glas-Inputs, benannte Klassen); Speichern/Abbrechen→`components.Button` CTA/secondary; bestehende `Card`-Nutzung auf Glas. |
| **Projektliste** | `nodes.templ` | Baum-Rows in Glas-Karten + Logos (Logo>Icon>Glyph-Priorität wie im Cockpit); roher `bg-ink`-Button→CTA; Kind-Badges Pill. |
| **Node-Formular** | `nodes.templ` (Form-Templates) + ggf. `nodeform.js` unberührt | Kristall-Formsprache (Glas-Inputs, CTA-Buttons); Icon-Radio-Grid/Logo-Upload-Optik (aus Slice 2) auf Glas; keine Logik-Änderung. |
| **Einstellungen** | `einstellungen.templ` | `Card`→Glas; ThemeToggle bleibt; Buttons→`components.Button`. |

**Grounding:** K3 hat exakt dieses Muster über Woche/Historie/Frei/Export gefahren (positive + preservation + negative-Assertion-Tests pro Swap, `verify-css` nach jedem). K4 folgt demselben Rezept. Wo eine Fläche schon `components.Card`/`Button` nutzt (Editor/Einstellungen teilweise), reicht der Glas-Flip der Komponente — evtl. schon durch K1 erledigt; pro Task verifizieren, nicht doppelt stylen.

## 4. Scope B — Wissen-Rollup (Containment §4 im Wissen-Tab)

**Regel (Umbrella §4):** Ein Cockpit zeigt Inhalte seines **Subtrees**. Für Wissen:

| Kind | Default | Toggle |
|---|---|---|
| Engagement / Vorhaben | **Subtree-Docs** (alle Docs mit NodeRef ∈ Subtree) | Pill „nur dieser Knoten" → own-only |
| Repo | **own-only** | kein Toggle (Subtree == self) |

**Mechanik (kompositorisch, kein neuer Port):**
- Toggle via Query-Param `?scope=self` am Cockpit-Tab-Request (`/nodes/{id}?tab=wissen&scope=self` bzw. das bestehende htmx-Tab-Fragment). Default (Param fehlt) = Subtree für Eng/Vorhaben. **Kein Server-State.**
- Datenpfad: `Stats.Nodes.Subtree(ctx, u.ID, node.ID)` → `subtreeIDs`-Map (dieselbe Quelle, die die Übersicht-Karte T5 nutzt), dann `ListDocuments.Execute(ctx, u.ID, nil, nil)` in-memory auf `subtreeIDs` filtern. Für `scope=self` **oder** Repo: der bestehende own-only-Pfad `ListDocuments.Execute(ctx, u.ID, &node.ID, nil)`.
- **Owner-scoped durchgängig** (`Subtree` + `List` nehmen `u.ID`); ein Subtree-Fehler degradiert auf leere Liste (kein 500), analog Übersicht.
- Render: Toggle als Zwei-Zustands-Pill im Wissen-Tab-Header (nur bei Eng/Vorhaben sichtbar); aktiver Zustand markiert. Doc-Rows in Glas (teilt das Sweep-Rezept aus §3).

**Nicht betroffen:** Übersicht-„Zuletzt-Wissen"-Karte (bleibt Subtree, kein Toggle) und globale `/wissen`-Seite (bleibt global über alle Owner-Docs).

**Trade-off (dokumentiert, nicht vorgebaut):** Der Subtree-Zweig lädt alle Owner-Docs und filtert in-memory — identisch zum bestehenden Übersicht-Pattern (K2 T5 accepted N+1/full-load-Trade-off). Ein Subtree-scoped Docs-Query (neuer Port) ist der Folgeschritt, falls ein Tenant stark wächst; im Plan als Follow-up notiert, in K4 nicht gebaut.

## 5. Scope C — Login Kristall-Moment

Umbrella §13 offene Entscheidung → gefixt (Soenne 2026-07-03, Option „Fehlerseiten + Logout-Landing").

- **`/auth/login` bleibt Auto-302 zu Dex** — kein Landing-Gate, kein Extra-Klick im normalen Login-Flow (`handleLogin` unverändert).
- **Logout landet auf einer Kristall-Seite** statt zurück nach `/` zu loopen (heute: `handleLogout` → `Redirect /` → `webAuth` → `/auth/login` → 302 Dex, also sofortiger Re-Login-Redirect). Neu: `handleLogout` rendert eine **„Abgemeldet"-Glas-Landing** (Facets-Canvas + Wordmark + „Anmelden →"-CTA → `/auth/login`). Sauberer Logged-out-Zustand, kein Loop.
- **Fehlerseiten Kristall** statt plaintext: `forbidden` (403 — was ein nicht-allowlisteter User sieht, der eigentliche Gewinn), `auth failed` (401), `bad state` (400) werden volle Kristall-Error-Pages (Canvas + Titel + Erklärung + ggf. „Erneut anmelden"-CTA). Diese rendern **ohne `webAuth`** (unauthenticated Kontext) — eigene schlanke Render-Templates, keine AppShell/Sidebar (kein User-Kontext, kein Nav-Tree).

**Kein** neuer Auth-Code, **keine** neue Dependency. Nur Render-Templates + die drei `http.Error`-Aufrufe in `webauth.go` durch Kristall-Renders ersetzen und `handleLogout` auf Landing-Render umstellen. i18n de+en für alle neuen Strings.

## 6. Architektur & Isolation

- **WebUI-only.** Kein Domain-/Port-/Usecase-Zuwachs (Rollup ist kompositorisch; Login sind Render-Templates). TUI/CLI/MCP unberührt.
- **Neue templ-Templates:** ggf. ein `auth.templ` (Logout-Landing + Error-Pages) im `webui`-Paket; Sweep bearbeitet bestehende `.templ` in-place. Kein neues Paket.
- **htmx-Regeln bleiben:** Cockpit-Tab-Fläche targetet `#cockpit-main`; `scope`-Toggle ist ein Tab-Fragment-Reload (kein Full-Page). SSE-Reloads (`document.*`) unverändert.
- **Owner-Scope** wie §4: `Subtree`/`List` mit `u.ID`.

## 7. Testing

Pro Sweep-Task (K3-Muster): Render-Test mit **positive** (Glas-Klasse vorhanden) + **preservation** (Funktion/Link/Attribut erhalten) + **negative** (`bg-surface` weg) Assertions; `verify-css` grün. Rollup: Handler-Tests für (a) Eng/Vorhaben default = Subtree-Docs, (b) `scope=self` = own-only, (c) Repo = own-only kein-Toggle, (d) foreign-owner-Doc leakt nicht, (e) Subtree-Fehler → leere Liste kein 500. Login: Render-Tests für Logout-Landing + je Error-Page (Statuscode + Kristall-Marker + „Anmelden"-CTA-Link), plus Verifikation dass `webAuth` unverändert redirectet. `make ci` ≥ 75 % (`*_templ.go` ausgeschlossen).

## 8. Prozess

Ein Just-in-time-Plan `docs/superpowers/plans/2026-07-03-kristall-k4-wissen-verwaltung.md` (Main-Wiring+Gate-Task am Ende) → subagent-driven-development → per-Task-Review → `make ci` → Live-Gate vs. Dev-Stack (scripted Dex-Login: alle Flächen auf Glas, Wissen-Tab Subtree↔self-Toggle, Repo own-only, Logout-Landing, forbidden-Page) → Opus-Whole-Branch-Review (BASE = Parent des ersten K4-Commits) → Soenne-Dogfood. **Model-Policy (Umbrella):** Implementer sonnet (haiku für reine Transkription), Task-Reviewer haiku/sonnet nach Risiko, Whole-Branch-Review opus — **immer explizit setzen, nie Fable/Opus erben**; stirbt ein Subagent mid-task, finisht + verifiziert der Controller inline.

## 9. Nicht-Ziele

- Kein Landing-**Gate** vor dem normalen Login (Happy-Path bleibt Auto-Redirect).
- Kein Subtree-scoped Docs-Query-Port (in-memory-Filter reicht; Follow-up wenn Skalierung).
- Keine globale Suche, kein neues Fachfeature (Umbrella §11).
- Keine Änderung an `/wissen` (global) oder der Übersicht-Wissen-Karte (schon Subtree).
- TUI/CLI/MCP unverändert.
- Light-Theme-Gegencheck, Mobile-Reflow, A11y-Pass = **K5** (nicht K4) — K4 baut aber nichts, was diese verletzt.

## 10. Risiken

1. Manche Flächen nutzen schon `components.Card`/`Button` (Editor/Einstellungen); wenn K1 die Komponente bereits verglast hat, ist der Task ein No-op-Verify — pro Task prüfen statt doppelt stylen (sonst tote Diff-Zeilen).
2. Wissen-Rollup-Toggle: Query-Param-Reload darf htmx-Tab-Contract (`#cockpit-main`) nicht brechen — Render-Test pinnt Target.
3. Error-Pages ohne `webAuth`/AppShell: müssen ohne User-Kontext/Nav-Tree rendern (kein nil-Deref auf Sidebar-Daten) — eigene schlanke Base-Variante, im Test abgedeckt.
4. Sweep-Umfang groß (5 Flächen): Design-Änderbarkeits-Regel diszipliniert halten — jede neue arbitrary Klasse = Finding.

## 11. Offene Detail-Entscheidungen (im Plan zu fixieren)

- Exakter Query-Param-Name/-Wert für den Wissen-Toggle (`scope=self` vorgeschlagen) + Pill-Beschriftung.
- Error-Page-Struktur: eine parametrisierte `authError(title, msg, showLoginCTA)`-Templ vs. drei Templates (vorgeschlagen: eine parametrisierte).
- Ob die Logout-Landing dieselbe Base wie die Error-Pages teilt (vorgeschlagen: ja, gemeinsame schlanke `authShell`).
