# flow Rebuild · M1c — DayOff + Feiertage + ICS · Design

**Datum:** 2026-06-14
**Status:** Draft — Brainstorm abgeschlossen, wartet auf User-Review
**Scope:** Erster der drei M1c-Slices. Abwesenheits-Vertical: manuelle DayOffs (Urlaub/Krank) inkl. Halbtag, berechnete deutsche Feiertage, abonnierbarer ICS-Feed — in Server + TUI + WebUI, live-synced. Hält die slice-spezifischen Entscheidungen fest; Architektur-Fundament siehe `2026-06-13-flow-rebuild-design.md`, Worktime-Spine siehe `2026-06-14-flow-rebuild-m1a-worktime-design.md`, Auth siehe `2026-06-14-flow-rebuild-m1b-device-flow-design.md`.
**Branch:** Code auf dem langlebigen Orphan-Branch `rebuild` (kein main-merge pro Milestone); Planungs-Docs auf `main` (M0/M1a-Präzedenz).

## Warum dieser Schnitt (Slicing)

Die M1a-Spec definierte **M1c = „Worktime-Extras — DayOff, Stats, Burndown, dt. Feiertage, ICS-Export, Zeit-Export pro Projekt/Zeitraum"**. Das bündelt erneut sechs Subsysteme — genau der Scope-Sprawl, den der Neuaufbau vermeidet. Entlang der natürlichen Abhängigkeitskette (DayOff/Feiertage → Stats/Burndown → Exporte) wird M1c in **drei eigenständig abnehmbare Schnitte** geteilt:

- **M1c (dieses Dokument):** DayOff + deutsche Feiertage + abonnierbarer ICS-Feed. Liefert die „Arbeitstag"-Grundlage, auf der Stats/Burndown aufsetzen.
- **M1d:** Stats + Burndown + Tagesziel-Config (konsumiert Sessions + DayOffs + Feiertage).
- **M1e:** Zeit-Export pro Projekt/Zeitraum (Session-Aggregation).

Jeder Schnitt: eigener Spec → Plan → Ausführung, `make ci` grün als Tor.

## Done-Gate (Akzeptanztest)

> Urlaubswoche (Von–Bis) in der WebUI eintragen → erscheint binnen ~1 s in der TUI; derselbe ICS-Feed-Link im echten Kalender (Apple/Google) abonniert zeigt die Abwesenheiten.

Der Cross-Surface-Live-Loop (wie M1a) plus der abonnierbare Feed sind der Zweck von M1c.

## Scope / Non-Goals

**In Scope:**
- `DayOff` (manuell: `vacation`/`sick`) — Entity-Carry-over + Store + REST + SSE + TUI-Sicht + WebUI-Page.
- **Range-Eingabe → Tageszeilen:** Von–Bis wird in einzelne Tageseinträge expandiert (Wochenenden überspringbar); Speichermodell bleibt 1 Row/Tag wie v1; einzeln löschbar.
- **Halbtags-Override:** `target_min` pro DayOff (voll eingebbar in M1c).
- **Deutsche Feiertage:** berechnet, nicht persistiert; beim Lesen mit manuellen DayOffs gemerged. Per-User-Bundesland-Setting (Default `NW`).
- **ICS-Feed:** abonnierbare URL pro User mit Geheim-Token (`/ics/{token}.ics`, kein OIDC); regenerierbar/revozierbar.
- TUI: DayOff-Sicht im Worktime-Screen + Settings-Mini (Bundesland, Token).
- WebUI: DayOff-Page (HTMX + HTMX-SSE) + Settings-Snippet mit Feed-URL.

**Non-Goals (vertagt):**
- Stats / Burndown / Tagesziel → **M1d**.
- Zeit-Export pro Projekt/Zeitraum → **M1e**.
- One-Shot-ICS-Download (entschieden gegen — Feed deckt den Use-Case besser ab).
- Mehrere Bundesländer / Auslandsfeiertage; Custom-Feiertags-Pflege.
- Pixelgenaue UI-Politur → `frontend-design` / `tui-usability` (später).
- Offline/Read-Cache (M1c ist online-only wie M1a).

## Kern-Entscheidungen (Brainstorm 2026-06-14)

| Frage | Entscheidung |
|---|---|
| M1c-Slicing | **3 dünne Slices:** M1c (DayOff+Feiertage+ICS) → M1d (Stats+Burndown) → M1e (Export). |
| Oberflächen | **TUI + WebUI, live-synced** (M1a-Symmetrie). |
| Feiertage | **Berechnet, nicht persistiert**, beim Lesen gemerged (kein Drift, kein Re-Seed). |
| Bundesland | **Per-User-Setting, Default `NW`.** |
| ICS-Delivery | **Abonnierbarer Feed + Geheim-Token** (Token-by-URL-Auth), nicht One-Shot. |
| DayOff-Range | **Range-Eingabe → Tageszeilen** (1 Row/Tag, Wochenenden überspringbar). |
| Halbtags | **Voll dabei** (`target_min`-Override, Von–Bis + halber Tag). |
| Settings-/Token-Speicher | **`user_settings` + separate `feed_tokens`-Tabelle** (mehrere Tokens, revozierbar). |
| ICS-Feed-Inhalt | **Nur eigene Abwesenheiten** (`vacation`/`sick`); Feiertage exkludiert (public, würden im Kalender doppeln). |

## Datenmodell-Deltas

Neue Migration `migrations/0003_dayoff_settings_feedtokens.sql` (inkrementell, embedded — goose-Tooling wie M0/M1a).

```sql
CREATE TABLE day_offs (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    day        DATE NOT NULL,              -- ein Eintrag pro Tag
    kind       TEXT NOT NULL,             -- 'vacation' | 'sick' (holiday = berechnet, nie gespeichert)
    label      TEXT NOT NULL DEFAULT '',
    target_min INTEGER NOT NULL DEFAULT 0, -- Halbtags-Override in Minuten; 0 = ganzer Tag frei
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_id, day)
);
CREATE INDEX day_offs_owner_day ON day_offs (owner_id, day);

CREATE TABLE user_settings (
    user_id    TEXT PRIMARY KEY REFERENCES users(id),
    bundesland TEXT NOT NULL DEFAULT 'NW',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE feed_tokens (
    token      TEXT PRIMARY KEY,          -- 32-byte crypto-random, base64url
    user_id    TEXT NOT NULL REFERENCES users(id),
    kind       TEXT NOT NULL DEFAULT 'ics',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ                 -- null = aktiv
);
CREATE INDEX feed_tokens_user ON feed_tokens (user_id) WHERE revoked_at IS NULL;
```

**Invarianten:** `UNIQUE (owner_id, day)` erzwingt max. einen manuellen DayOff pro Tag (Re-Add desselben Tags = Upsert). `holiday` landet nie in `day_offs`. Domain rechnet Halbtag in `time.Duration`, DB speichert Minuten (`target_min`).

## Domain & Usecases (Carry-over, kein Rewrite)

**Carry-over aus `main` (`internal/domain/`):**
- `dayoff.go` — `Kind`, `AllKinds`, `LabelDe`, `ParseKind`, `DayOff{Date, Kind, Label, Target}`. Übernehmen wie ist.
- `holidays_de.go` — `GermanHolidays(year, land, loc)` + `easterSunday`/`busBettag`/`normalizeLand`/`appliesIn`. **1:1** inkl. Tests.
- `ics.go` — `WriteICS(w, dayoffs, now)`. Übernehmen.

**Neu:**
- `domain/dayoff_range.go` — `ExpandRange(from, to time.Time, kind Kind, label string, targetPerDay time.Duration, skipWeekends bool) []DayOff`. Reine Funktion, voll testbar.

**Neue Ports** (`internal/ports/ports.go`, M1a-Stil):
- `DayOffStore` — `Add(ctx, ownerID, DayOff) error` (Upsert pro Tag), `Delete(ctx, ownerID, day) error`, `ListRange(ctx, ownerID, from, to) ([]DayOff, error)`.
- `UserSettingsStore` — `Get(ctx, userID) (Settings, error)` (lazy Default-Row), `SetBundesland(ctx, userID, land) error`.
- `FeedTokenStore` — `Create(ctx, userID, kind) (token string, err error)`, `Resolve(ctx, token) (userID string, err error)` (nur aktive), `ListByUser(ctx, userID) ([]FeedToken, error)`, `Revoke(ctx, token) error`.

**pgstore-Adapter** (je eine Datei, „keine Monolithen"): `dayoffs.go`, `user_settings.go`, `feed_tokens.go`.

**Usecases (dünn):**
- `AddDayOffs(owner, from, to, kind, label, targetPerDay, skipWeekends)` → `ExpandRange` → Upsert pro Tag → **ein** `dayoff.changed`-Event auf `sse.Bus`.
- `DeleteDayOff(owner, day)` → Store + SSE-Event.
- `ListDayOffs(owner, range)` → **Merge** manueller DayOffs ∪ `GermanHolidays(jahre(range), bundesland, loc)`; bei Datums-Kollision gewinnt der manuelle Eintrag. *Die eine Lese-Quelle* für TUI, WebUI und ICS.
- `IcsFeed(token)` → `FeedTokenStore.Resolve` → manuelle DayOffs in `[heute−1J, heute+1J]` (ohne Feiertage) → `WriteICS`.
- `RegenerateIcsToken(owner)` → alte aktive Tokens revoken + neuen `Create`; `SetBundesland(owner, land)`.

Jede Mutation published ein `domain.Event` auf dem bestehenden `sse.Bus` (M1a-Pattern). Berechnete Feiertage sind statisch → kein Event.

## Auth — dritter Pfad: Token-by-URL

Neben M0-Bearer (CLI/TUI) und M1a-Session-Cookie (Browser) bekommt der ICS-Feed einen **eigenen dünnen Auth-Zweig**, isoliert in der Feed-Handler-Datei:

- `GET /ics/{token}.ics` — extrahiert das Pfad-Segment, `FeedTokenStore.Resolve(token)` → `ownerID` (oder 404 bei unbekannt/revoked), liefert `text/calendar`. Kein OIDC, kein Cookie.
- Token: 32-byte crypto-random, base64url, in `feed_tokens`. Regenerate revoked alte + legt neuen an. Revozierter/unbekannter Token → **404** (nicht 401 — keine Existenz-Leaks).

Alle übrigen Routen (`/api/dayoffs`, `/api/settings`, `/api/ics-token/regenerate`) hängen hinter der bestehenden OIDC/Cookie-bzw-Bearer-Middleware.

## HTTP-Routen

| Methode | Pfad | Auth | Zweck |
|---|---|---|---|
| `GET` | `/api/dayoffs?from=&to=` | OIDC/Bearer | gemergte DayOffs (manuell + Feiertage) |
| `POST` | `/api/dayoffs` | OIDC/Bearer | Range anlegen (from,to,kind,label,targetMin,skipWeekends) |
| `DELETE` | `/api/dayoffs/{day}` | OIDC/Bearer | einen Tag löschen |
| `GET` | `/api/settings` | OIDC/Bearer | Bundesland + aktive Feed-URL(s) |
| `POST` | `/api/settings/bundesland` | OIDC/Bearer | Bundesland setzen |
| `POST` | `/api/ics-token/regenerate` | OIDC/Bearer | neuen Feed-Token, alte revoken |
| `GET` | `/ics/{token}.ics` | Token-by-URL | abonnierbarer Kalender-Feed |

## TUI

- DayOff-Sicht im bestehenden Worktime-Screen (Liste: Feiertage + Urlaub + Krank, Glyph/Sem-Farbe je Kind wie v1-Dayoff-Glyph-Unification). Von–Bis-Eingabe inkl. Halbtag + Kind; Löschen einzelner Tage.
- Settings-Mini: Bundesland anzeigen/ändern, aktive Feed-URL anzeigen, Token regenerieren.
- SSE → `tea.Msg` → Re-render (M1a-Pattern). TUI-Auth: M1b-Device-Flow-Token (bereits live).

## WebUI

- `dayoffs.templ` Page: HTMX-Form (Von–Bis / Kind / Halbtag / skipWeekends), Liste mit Delete, HTMX-SSE-Re-render.
- Settings-Snippet: Bundesland-Select, abonnierbare ICS-URL (Copy), Regenerate-Button.
- Rendering bewusst schlicht (Pixel-Politur vertagt, M1a-Präzedenz).

## Error-Handling

- Range mit `to < from` → 400 (Validierung im Usecase).
- Unbekanntes/ungültiges `kind` → 400 (`ParseKind`).
- Löschen eines nicht existenten Tags → idempotent 204.
- Re-Add desselben Tags → Upsert (kein Konflikt-Fehler).
- Feed-Token unbekannt/revoked → 404 `text/plain`.
- Bundesland unbekannt → `normalizeLand` fällt auf den Roh-String zurück; Validierung gegen die 16-Codes-Liste, sonst 400.

## Testing

- **Domain:** `ExpandRange` (Wochenend-Skip, Halbtag-Target, Grenztage, `to==from`), `GermanHolidays` (Carry-over-Tests mitnehmen, NW-Set prüfen), `WriteICS` (Format), Merge-Dedup (manuell schlägt Feiertag).
- **pgstore:** testcontainers wie M1a (`worktime_test.go`-Muster) — DayOff Upsert/Delete/ListRange, Settings lazy-default, FeedToken create/resolve/revoke.
- **Auth:** Feed-Route mit gültigem/revoziertem/unbekanntem Token; Owner-Isolation (Token A sieht nicht B's DayOffs).
- **Usecase:** `AddDayOffs` published genau ein Event; `ListDayOffs` merged korrekt.
- **Done-Gate manuell:** WebUI-Urlaubswoche → TUI ~1 s; Feed-URL im echten Kalender abonniert → Einträge erscheinen.

## Wiring-Verification (Pflicht-Abschlusstask)

Letzter Plan-Task (Lesson „Plans need a main-wiring task"): `cmd/flow/main.go` (bzw. der Server-Composition-Root) verdrahtet **jeden** neuen Store/Usecase/Handler, und ein curl-Smoke trifft **jede** neue Route (inkl. `/ics/{token}.ics` mit echtem Token). `make ci` grün.

## Offene Punkte

- Bundesland-Auswahl-UX in der WebUI (Select aller 16 Codes vs. nur die häufigen) — Detail für die Plan-Phase.
- Glyph/Farb-Mapping der Kinds in der WebUI an die TUI-Sem-Farben angleichen (Carry-over Dayoff-Glyph-Unification) — Plan-Detail.
