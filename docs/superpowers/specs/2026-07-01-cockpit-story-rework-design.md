# flow — Cockpit-Story-Rework (lebendiges Projekt-Zuhause) — Design Spec

> Datum: 2026-07-01 · Status: **DRAFT** (brainstormed, user-approved design + Direction-B mockup) · Branch-Ziel: `rebuild`.
> Umbrella-Spec für **5 Implementierungs-Slices** (jeder = eigener Plan + subagent-driven Umsetzung).
> Vorgänger: [[specs/2026-06-30-flow-m1-slice6-cockpit-design]] (Slice 6, gemerged `ac582d1`). Dies ist die **Post-Dogfood-Evolution** daraus.
> Approved-Mockup: `docs/superpowers/specs/assets/2026-07-01-cockpit-story/direction-b-APPROVED.html` (Alternative A: `direction-a-alt.html`).

---

## 1. Kontext & Ziel

Slice 6 lieferte ein **funktional vollständiges** Cockpit (`/nodes/{id}`: persistenter Head + 4 htmx-Tabs). Der Live-Browser-Dogfood ergab: es ist **praktisch, aber nicht schön und erzählt keine Geschichte** — es zeigt nicht gut, *was dieses Projekt ist* und *was gerade passiert*. Dazu kamen 8 weitere konkrete Reibungspunkte. Diese Spec adressiert alle als **einen zusammenhängenden Rework**.

**Rückgrat (User-Entscheidung):** ein **lebendiges Projekt-Zuhause** — beim Öffnen sieht man Identität (*was ist das*) + Puls (*was passiert gerade*), Rollup/Tabs als Tiefe. Nicht der Rollup ist der Held, sondern **Puls + Identität**.

**Die 9 Dogfood-Punkte und wo sie landen:**
| # | Punkt | Slice |
|---|---|---|
| 1 | Home-Timer nur auf Engagement stoppbar (Picker `Kind==Engagement`) | 4 (Fix) |
| 2 | Sidebar-Stil uneinheitlich + hässlicher `…`-Overflow | 5 |
| 3 | Aktivität ohne „welcher Timer" (Session-Events tragen kein Ziel) | 3 |
| 4 | Zeit-Rollup nach oben (existiert) + **Work/Privat-Split fehlt** | 1 (+ Anzeige in 4) |
| 5 | Wissen sollte auch nach oben rollen (Subtree-Docs) | 5 |
| 6 | Sessions im Cockpit nicht editierbar (nur Liste+Nachbuchen) | 4 |
| 7 | Cockpit „nicht schön / erzählt keine Geschichte" (**Kernthema**) | 4 |
| 8 | Nodes sollen ein Logo bekommen können | 2 |
| 9 | Kein UI-Toggle für Work vs Privat (`countsTowardTarget`) | 1 |

## 2. Nicht-Ziele
- Kein neues Tab-Konzept jenseits von „Übersicht + die 4 bestehenden" (Kontext-Tab = M1 Slice 8, separat).
- Kein Redesign von TUI/CLI/MCP — dieser Rework ist **WebUI + das nötige Backend**.
- Keine Sharing/Multi-Tenant-Features (M2/M3).
- Mermaid-Rendering (M1 Slice 7) bleibt separat; hier nur der Wissen-Rollup-Aspekt (#5).

---

## 3. Architektur — „persistente Schiene + Tab-Spalte"

Das Cockpit (`/nodes/{id}`) wird von „persistenter Head oben + Tab-Panel darunter" (Slice 6) zu **Zwei-Spalten** (Direction B):

- **Linke Schiene (persistent, über ALLE Tabs sichtbar):** Identitäts-Karte (Logo, Name, Kind-Badge, Status, Beschreibung, geerbte Rate, Beiträger) + **Per-Node-Timer-Karte** (die 5 Zustände aus Slice 6: idle/here/otherBound→Wechseln/unbound/notBookable) + Quick-Actions (Nachbuchen · Neues Wissen · Struktur). Die Schiene ist der Kontext, den man nie verliert.
- **Rechte Spalte (tab-umschaltbar):** eine Tab-Leiste **Übersicht · Worktime · Wissen · Struktur · Bindings**; **Übersicht** ist die Default-Landing.

Das ist die direkte Evolution von Slice 6: der Timer wandert aus dem Head in die Schiene, seine `NodeTimer`-Zustandslogik + Start/Stop/Switch-Handler **bleiben** wiederverwendet. Die kanonische htmx-Regel bleibt: die Schiene (`#cockpit-rail`) reloadet auf session/node-SSE; die Tab-Spalte (`#cockpit-main`) swappt bei Tab-Klick/Panel-SSE — **alles, was die Tab-Spalte neu rendert, targetet `#cockpit-main`** (kein Nesting, s. Slice-6-Lehre).

**Responsive:** unter `lg` stapelt die Schiene **über** die Spalte (Identität + Timer zuerst, dann Tabs), nutzt die bestehende AppShell-Mobile-Chrome.

## 4. Die Übersicht-Landing (die Geschichte)

Rechte Spalte, Default-Tab. Komposition (aus dem approved Mockup):
- **Rollup-Kacheln** (oben): Subtree Σ (eigen + alle Unterknoten) · Woche · Monat · **Verdienst** (Work × geerbte Rate).
- **Work vs Privat** (Karte): Split-Balken (Work grün→cyan / Privat purple), Zahlen, Hinweis „Work zählt aufs Tages-Soll · Privat wird nur getrackt". Kollabiert elegant, wenn eine Seite 0 ist.
- **„Fließt nach oben"-Kette** (Karte): dieser Knoten → Vorhaben → Engagement → Gesamt, je mit form-codiertem Icon (● Engagement / ◆ Vorhaben / ⬡ Repo / ▪ Gesamt) + Anteils-Balken. Macht die Akkumulation *fühlbar*.
- **Puls / Aktivität** (Karte, LIVE): Timeline der letzten Aktivität — **jede Zeile mit Ziel-Pill** („startete Timer **auf Repo flow**"), Mensch=Kreis-Avatar / KI-Agent=Hexagon-Avatar (+ `AI-AGENT`-Tag). (Konsumiert Slice 3.)
- **Zuletzt geändertes Wissen** (Karte): neueste Docs aus dem **Subtree** (Slice 5), Klick → `/wissen/{id}`.

## 5. Datenmodell-Änderungen

### 5.1 Work/Privat — vererbt mit Override (Slice 1)
Heute: `countsTowardTarget` ist ein *pro-Knoten unabhängiger* `bool` (Default true), nur REST/CLI setzen ihn; kein UI; Rollup mischt Work+Privat.
Neu:
- **Effektiver Flag** wird über die Ahnenkette aufgelöst (nächster explizit gesetzter Vorfahr gewinnt — wie `ResolveRate`). Ein Knoten hat drei Zustände: *erbt* (Default), *explizit Work*, *explizit Privat*. Root ohne expliziten Flag = Work.
- `domain.NodeRollup` wird **gesplittet**: `{Total, Week, Month}` → zusätzlich `Work{…}` / `Privat{…}` (oder ein `WorkTotal`/`PrivatTotal`-Paar je Zeitfenster). `NodeStats` bucketet jede Subtree-Session nach dem **effektiven Flag ihres Node**.
- **Node-Formular** (WebUI, fehlt komplett): ein Tri-State-Toggle „Zählt zum Soll: erbt / Work / Privat" in `NodeFormValues` + `handleWebNodeCreate/Update`.

> Modell-Detail (im Slice-1-Plan zu fixieren): `countsTowardTarget` von `bool` auf einen erb-fähigen Typ heben — entweder `*bool` mit „nil = erbt" oder ein `NodeTargetMode`-Enum. Migration nötig (bestehende `true` → „explizit Work" ODER „erbt", je nach gewählter Semantik). `StatsComputer` baut die effektive Map künftig via Ancestor-Resolution statt direktem Feld.

### 5.2 Logos (Slice 2)
Heute: nur `Color` + `Glyph` (1 Zeichen). Neu: **Icon aus kuratiertem Set** (Standard) **+ optionaler Bild-Upload**.
- Neues Feld/Felder auf `domain.Node`: `Icon string` (Key aus dem eingebauten Set, gerendert im Knotenfarbton) und `LogoRef string` (Referenz auf ein hochgeladenes Bild; leer = Icon nutzen).
- **Icon-Set:** eine kuratierte, gewhitelistete Menge (analog zum bestehenden Glyph-Whitelist) — SVG-Icons, keine Emojis. Render-Helper wählt Upload > Icon > Glyph-Fallback.
- **Upload:** Endpoint + Storage (Migration + Bytes-Speicherung; Format/Größen-Limit; hexagonaler Crop beim Rendern). Kleinste vertretbare Lösung im Slice-2-Plan fixieren (z.B. DB-Blob oder Objektpfad).
- Gerendert in: Cockpit-Identitäts-Karte + Sidebar-Baum (Slice 5).

### 5.3 Aktivität-Ziel (Slice 3)
Heute: Session-`Emit`-Aufrufe übergeben **kein** `Data` → `activityFor` findet kein `title/name/node` → die Zeile rendert „msoent startete einen Timer" ohne Ziel; die Row-VM (`BuildActivityRows`) hat gar kein Node-Feld.
Neu:
- Session-Emit-Sites (`webui_cockpit.go` start/stop/switch, `webui.go`, `webui_home.go`, REST `worktime.go`) tragen das **Ziel** in `ev.Data` (`node`-Id + `name` + `kind`).
- `domain.ActivityEntry.NodeRef` (existiert, wird persistiert) wird **gelesen**; `BuildActivityRows` bekommt ein Ziel-Feld (Name + Kind); die Aktivitäts-templ rendert die **Ziel-Pill** (form-codiert). Mensch/Agent-Avatar existiert bereits.

## 6. Eingearbeitete Fixes (in ihren Slices)
- **#1 Stop-Picker (Slice 4):** `homeDataFor`/`heuteDataFor` filtern den Buchungs-`<select>` künftig auf `domain.IsBookable(p.Kind)` statt `== KindEngagement`. Ein auf Repo/Vorhaben gebuchter Timer ist überall stopp-/umbuchbar; die laufende Node wird vorselektiert. (Der veraltete Kommentar „only KindEngagement is bookable" verschwindet.)
- **#6 Sessions editierbar (Slice 4):** der Worktime-Tab bekommt **inline Edit/Delete** je Session (nutzt die bestehenden `EditSession`/`DeleteSession`-Usecases + `/zeit`-Muster), nicht mehr nur Liste+Nachbuchen.
- **#2 Sidebar (Slice 5):** form-codierter Baum (● ◆ ⬡) + Logo/Icon je Knoten + **weicher `mask-image`-Fade** statt hartem `…` (aktive Node zeigt vollen Namen; `title`-Tooltip für A11y).
- **#5 Wissen-Rollup (Slice 5):** Übersicht-„zuletzt geändertes Wissen" zieht aus dem **Subtree**; der Wissen-Tab bekommt einen „inkl. Unterknoten"-Umschalter (Subtree-Docs vs. nur direkte).

## 7. Slicing (Reihenfolge; jeder Slice = eigener Plan + subagent-driven + `make ci` + Done-Gate)
Backend/Daten zuerst, damit Slice 4 (Cockpit) die Daten hat:

1. **Work/Privat-Modell** — effektiver Flag (vererbt+Override) via Ancestor-Resolution, `NodeRollup`-Split, Migration, Node-Formular-Toggle (#4/#9). *(Backend + kleines Formular-UI.)*
2. **Node-Logos** — Icon-Set-Whitelist + optionaler Bild-Upload (Storage/Migration), Render-Helper (Upload>Icon>Glyph), Node-Formular (#8).
3. **Aktivität-Ziel** — Session-Events tragen Ziel; Row-VM + templ rendern die Ziel-Pill (#3). *(Klein, gut isoliert.)*
4. **Cockpit-Rework (Direction B)** — persistente Schiene + Übersicht-Landing (Puls + Rollup mit Work/Privat + Fließt-nach-oben-Kette) + Tab-Spalte; foldet **#1** (Stop-Picker) + **#6** (Sessions editierbar). Der große WebUI-Brocken; konsumiert 1–3. *(Timer/NodeTimer/Tab-Mechanik aus Slice 6 wiederverwendet.)*
5. **Sidebar-Rework + Wissen-Rollup** — #2 (form-codierter Baum, Logos, Fade) + #5 (Subtree-Docs + Umschalter).

**Abhängigkeiten:** 4 hängt an 1 (Split-Anzeige), 2 (Logo-Render), 3 (Puls-Aktivität). 5 hängt an 2 (Baum-Logos). Reihenfolge 1→2→3→4→5; 5 kann teils parallel zu 4.

## 8. Design-Sprache (Kristall)
Approved-Mockup ist die Referenz (`assets/2026-07-01-cockpit-story/direction-b-APPROVED.html`). Prinzipien: Twilight-Gradient + Low-Poly-Facetten, Glas-Karten (`backdrop-filter: blur(16px)`), **Form-Codierung der Hierarchie** (● Engagement / ◆ Vorhaben / ⬡ Repo) durchgängig in Baum, Ziel-Pills und Rollup-Kette; **Mensch=Kreis / KI=Hexagon**-Avatare; `tabular-nums` für Uhren/Dauern; **keine Emoji-Pictogramme** (nur SVG + Monospace-Glyphen ▶ ■ ● ◆ ⬡ › Σ); Motion in `@media (prefers-reduced-motion)` kapseln; Fade-Truncation braucht echten `title`. Dark = primär, Light = derived (am `/ui` gegenchecken).

## 9. Testing & Done-Gate (pro Slice)
- **TDD**; `make ci` grün (Gate 75%, **`*_templ.go` ausgeschlossen** — echte output-asserting Tests, kein Padding; `make web` für app.css). Kanonische htmx-Regel (`#cockpit-main`) durch Tests gepinnt.
- **Live-Done-Gate** vs. Dev-Stack je Slice; Cockpit-Rework (4) inkl. Browser-Dogfood (Story-Gefühl, Timer über Tabs, Work/Privat-Split, Aktivität mit Ziel, Sidebar-Fade, Mobile-Reflow).
- **Opus-Holistic-Review** je Slice; **`make fmt` in Dispatches verbieten** (Toolchain-Skew). Wiring-Verifikation je Slice.

## 10. Offene Detail-Entscheidungen (je Slice-Plan zu fixieren)
- Slice 1: `*bool` (nil=erbt) vs. `NodeTargetMode`-Enum; Migrations-Semantik für bestehende `true`.
- Slice 2: Upload-Storage (DB-Blob vs. Objektpfad); Icon-Set-Umfang.
- Slice 4: „Übersicht" ist der Default-Tab in der Leiste (entschieden). Zu fixieren: die persistente Schiene als eigener SSE-Container `#cockpit-rail` (reload auf session/node-Events), getrennt von `#cockpit-main`; ob die Quick-Actions in der Schiene oder in der Übersicht sitzen.
- Slice 5: Wissen-Rollup Default (direkt vs. Subtree) + Umschalter-Persistenz.
