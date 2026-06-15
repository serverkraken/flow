# flow Rebuild · M1d — Stats + Burndown + Tagesziel · Design

**Datum:** 2026-06-15
**Status:** Draft — Brainstorm abgeschlossen, wartet auf User-Review
**Scope:** Zweiter der drei M1c-Folgeslices. Auswertungs-Vertical: Tagesziel-Config (Default + per-Weekday-Overrides), Monats-Burndown, Today-Saldo, Wochen-Sicht und Range-Stats (KW/Monat) — server-berechnet, in TUI + WebUI, live-synced. Hält die slice-spezifischen Entscheidungen fest; Architektur-Fundament siehe `2026-06-13-flow-rebuild-design.md`, Worktime-Spine siehe `2026-06-14-flow-rebuild-m1a-worktime-design.md`, Abwesenheits-Grundlage siehe `2026-06-14-flow-rebuild-m1c-dayoff-ics-design.md`.
**Branch:** Code auf dem langlebigen Orphan-Branch `rebuild` (kein main-merge pro Milestone); Planungs-Docs auf `main` (M0/M1a/M1c-Präzedenz).

## Warum dieser Schnitt (Slicing)

Die M1c-Spec teilte das ursprüngliche M1c-Bündel in drei eigenständig abnehmbare Schnitte: **M1c** (DayOff + Feiertage + ICS, erledigt), **M1d** (dieses Dokument: Stats + Burndown + Tagesziel), **M1e** (Zeit-Export pro Projekt/Zeitraum). M1d konsumiert, was M1c liefert: Sessions (M1a) + gemergte DayOffs/Feiertage (M1c) sind die Eingaben, aus denen sich „Arbeitstag", „Tagesziel" und „Saldo" ableiten.

M1d ist überwiegend **Carry-over** der reichen, getesteten Auswertungs-Domain aus `main` — keine Neuerfindung. Der **eine echte Neubau** ist die Tagesziel-Config: in `main` kam sie aus env+file (`ConfigReader`/`TargetResolver`), im server-authoritativen Rebuild wandert sie nach `user_settings`.

## Done-Gate (Akzeptanztest)

> Tagesziel in Settings setzen (Default + z.B. Freitag 6h) → Wochen-Sicht & Monats-Burndown passen das geplante Target sofort an. Timer in der TUI laufen lassen → Today-Saldo und Monats-Gauge aktualisieren live (~1 s) in TUI **und** WebUI. Einen Urlaubstag (M1c) in der WebUI eintragen → das geplante Monats-Target sinkt binnen ~1 s in beiden Oberflächen.

Der Cross-Surface-Live-Loop (M1a/M1c-Symmetrie) auf abgeleiteten Auswertungen ist der Zweck von M1d.

## Scope / Non-Goals

**In Scope:**
- **Tagesziel-Config:** Default-Tagesziel + optionale per-Weekday-Overrides (Presence-Semantik), persistiert in `user_settings`. REST + Settings-UI (TUI + WebUI).
- **Server-seitiger `StatsComputer`-Usecase:** baut `DayRecord`s aus Sessions, mergt DayOffs+Feiertage, löst Targets → liefert `Stats` / `MonthBurndownReport` / `Day` / `[]WeekDay` als JSON-DTO.
- **Monats-Burndown-Gauge:** „Monat 78h / 160h · vorne 2h"-Saldo-Blick (`MonthBurndownCompute`).
- **Today: Ziel + Saldo:** Today-Header zeigt geloggt vs. Tagesziel + Tagessaldo, am bestehenden Worktime-Screen.
- **Wochen-Sicht (7 Tage):** je Tag geloggt/Ziel (`WeekDay`), horizontale Balken; neue TUI- und WebUI-Sicht.
- **Range-Stats (KW/Monat):** Aggregat — Total, ⌀, Max/Min, Workdays, Hits, Streak/BestStreak, Saldo (`Stats`/`AggregateRange`).
- **Live-Sync:** Re-Fetch der Stats-DTOs auf bestehende `session.*`/`dayoff.changed`-Events (kein neues Event-Topic).

**Non-Goals (vertagt):**
- **Zeit-Export pro Projekt/Zeitraum → M1e.**
- **by-tag-UI:** `Stats.ByTag`/`TopTags` werden als Domain mit-übernommen, aber **nicht** in der M1d-UI gezeigt — der Session-Flow setzt noch keine Tags, die Auswertung wäre leer. Taucht ohne Code-Änderung auf, sobald Tag-Erfassung existiert (späterer Slice).
- **Markdown-Brief/Report** (`reporter.go`, ICS-Brief, `ReportRange`) — gehört zum Kompendium-/Export-Vertical, nicht hier.
- **Streak im Status-Segment / Pause-Konzept** — kein eigener Status-Bar-Slice in M1d; Streak ist Teil des Range-Stats-DTO, aber kein dauerhaftes Header-Eyecatcher-Widget.
- **Pixelgenaue UI-Politur → `frontend-design` / `tui-usability` (später); Rendering bewusst schlicht (M1a/M1c-Präzedenz).**
- **Offline/Read-Cache** (M1d ist online-only wie M1a/M1c).

## Kern-Entscheidungen (Brainstorm 2026-06-15)

| Frage | Entscheidung |
|---|---|
| Berechnungsort | **Server rechnet, Client rendert** (Thin-Client-Doktrin, Design-Doc Z.87). Aggregation als JSON-DTO, keine Client-Logik. |
| Tagesziel-Modell | **Default + per-Weekday-Overrides** mit Presence-Semantik (NULL = kein Override → Default; expliziter Wert inkl. `0` = Override). |
| Tagesziel-Speicher | **Nullable Spalten auf `user_settings`** (`default_target_min` + `target_mon_min … target_sun_min`), nicht separate Tabelle. |
| Target-Priorität | **DayOff-Target (M1c) > Weekday-Override > Default** (v1-`TargetResolver`-Ordnung). |
| Oberflächen | **Alle vier:** Burndown-Gauge, Today-Saldo, Wochen-Sicht, Range-Stats — TUI + WebUI, live-synced. |
| Live-Sync | **Kein neues Event** — Stats sind abgeleitet; Re-Fetch auf bestehende `session.*`/`dayoff.changed`. |
| by-tag | **Domain tragen, UI nicht zeigen** (kein Tag-Flow → leer). YAGNI für die UI. |
| Carry-over vs. Rewrite | **Aggregations-Funktionen unverändert** übernehmen (inkl. Tests); neu sind nur die Datenquelle (Store statt File) und der `WorkSession`→Session-Input-Adapter. |

## Datenmodell-Deltas

Neue Migration `migrations/0004_target_config.sql` (inkrementell, embedded — goose wie M0/M1a/M1c). Erweitert die bestehende `user_settings`-Tabelle aus M1c:

```sql
ALTER TABLE user_settings
    ADD COLUMN default_target_min INT NOT NULL DEFAULT 480,  -- 8h Default-Tagesziel
    ADD COLUMN target_mon_min INT,   -- NULL = kein Override → Default
    ADD COLUMN target_tue_min INT,
    ADD COLUMN target_wed_min INT,
    ADD COLUMN target_thu_min INT,
    ADD COLUMN target_fri_min INT,
    ADD COLUMN target_sat_min INT,
    ADD COLUMN target_sun_min INT;
```

**Invarianten:** `default_target_min >= 0`. Per-Weekday-Spalte `NULL` bedeutet „kein Override" (Presence-Semantik exakt wie v1 `Config.LookupWeekday`); ein explizit gesetztes `0` bedeutet „dieser Wochentag ist Soll-frei" und wird **nicht** vom Default überschrieben. Domain rechnet in `time.Duration`, DB speichert Minuten. Bestehende `user_settings`-Zeilen erhalten beim Migrieren `default_target_min = 480` und alle Overrides `NULL` (= reines 8h-Mo–So-Verhalten, gedämpft durch `IsWorkday`, das Wochenenden ohnehin ausschließt).

## Domain & Usecases (Carry-over, kein Rewrite)

**Carry-over aus `main` (`internal/domain/`) — übernehmen wie ist, inkl. Tests:**
- `aggregate.go` — `Aggregate`, `AggregateRange`, `PlannedTarget`, `MonthBurndownCompute`, `FilterRecords`, Hilfsfunktionen (`isHit`, `bestStreak`, `currentStreak`, `walkWorkdaysForSaldo`, `truncDay`). **Ohne** den Brief-Teil (`ReportRange`/`BriefBounds` bleiben in `main`/M1e — siehe Non-Goals; falls untrennbar verdrahtet, mitnehmen aber nicht verdrahten).
- `stats.go` — `Stats`, `TagDur`, `TopTags`. JSON-serialisierbar (Maps für `ByTag`/`CountByTag`).
- `burndown.go` — `MonthBurndownReport`.
- `calendar.go` — `IsWorkday`, `isoMonday` und verwandte Datums-Helfer (soweit Stats sie brauchen).
- `day.go` — `DayRecord`, `WeekDay`, `Day` (Today-Summary). Im Rebuild existiert `worksession.go` bereits; `Day`/`WeekDay`/`DayRecord` ergänzen, ohne `WorkSession` zu duplizieren.

**Wichtige Integrations-Reibung (kein reiner Copy):** `main`s Auswertungs-Domain hängt am alten `domain.Session`-Typ (Felder `Date`/`Start`/`Stop time.Time`, **`Elapsed time.Duration` als gespeichertes Feld**, `Tag`/`Note`). Der Rebuild hat `domain.WorkSession` (`Stop *time.Time`, `Elapsed(now)` als **Methode**). Die Aggregation (`st.ByTag[s.Tag] += s.Elapsed`) liest `Elapsed`/`Tag` als Felder. Beim Port muss der Reader jede `WorkSession` in eine aggregations-taugliche Session-Form übersetzen (Tag + zur Build-Zeit berechnetes `Elapsed`) — entweder `domain.Session` als reine Aggregations-/Presentations-Shape mitübernehmen, oder `DayRecord.Sessions` auf eine schlanke `{Tag string; Elapsed time.Duration}`-Form umstellen. **Entscheidung im Plan**; die Aggregations-Funktionen selbst (`Aggregate`/`AggregateRange`/`MonthBurndownCompute`) bleiben unverändert, nur ihr Input-Adapter ist neu.

**Neu / angepasst:**
- `usecase/target_resolver.go` — `TargetResolver` aus `main` portiert, aber Quelle ist `UserSettingsStore` statt `ConfigReader`. `For(date) time.Duration` mit der Priorität DayOff > Weekday-Override > Default; `IsWorkday(date)` / `IsDayOff(date)` über die M1c-DayOff-Quelle. Lädt Settings ctx-scoped pro User.
- `usecase/stats_computer.go` — server-seitiger `StatsComputer`: `Reader` (Sessions→`DayRecord`s), `Targets` (`TargetResolver`), `DayOffs` (M1c-Merge-Quelle), `Sessions` (`SessionStore`). Methoden: `Today(ctx, owner, now)`, `Week(ctx, owner, ref)`, `RangeStats(ctx, owner, range)`, `Burndown(ctx, owner, now)`.
- `usecase/worktime_reader`-Äquivalent — baut `[]DayRecord` aus `SessionStore.List` (Sessions nach Kalendertag gruppiert, `Total` + `Target` je Tag gesetzt). Inkl. der laufenden Session (Live-Tail) für Today/Burndown.

**Neue/erweiterte Ports** (`internal/ports/ports.go`):
- `UserSettingsStore` (M1c) wird erweitert: `GetTargetConfig(ctx, userID) (domain.TargetConfig, error)` und `SetTargetConfig(ctx, userID, domain.TargetConfig) error` — oder die bestehende `Settings`-Struct/`Get` um die Target-Felder erweitern (Plan-Detail: eine kohärente `Settings`-DTO bevorzugen).
- `SessionStore.List` (M0/M1a) liefert die Sessions bereits; ggf. eine `ListRange(ctx, owner, from, to)`-Variante ergänzen, damit Range-Stats nicht „alles" laden müssen (Plan-Detail).

**pgstore-Adapter** (je eine Datei, „keine Monolithen"): `user_settings.go` (M1c) um die Target-Spalten erweitern; neue Reads für Range falls ergänzt.

Jede Mutation, die Stats beeinflusst, ist bereits abgedeckt: Sessions (M1a) und DayOffs (M1c) publishen ihre Events. Die Target-Config-Mutation (`SetTargetConfig`) published **ein** `settings.changed`-Event (neues, kleines Topic — oder Wiederverwendung von `dayoff.changed`-artigem Re-Fetch-Trigger; Plan-Detail), damit andere Surfaces des **selben** Users ihr Stats-Fragment neu ziehen.

## HTTP-Routen

| Methode | Pfad | Auth | Zweck |
|---|---|---|---|
| `GET` | `/api/today` | OIDC/Bearer | Today-Summary: geloggt, Ziel, Saldo, running |
| `GET` | `/api/week?ref=` | OIDC/Bearer | 7-Tage-Sicht (`[]WeekDay`) der ISO-Woche um `ref` (Default heute) |
| `GET` | `/api/stats?range=week\|month` | OIDC/Bearer | Aggregat-`Stats` für KW/Monat |
| `GET` | `/api/burndown` | OIDC/Bearer | Monats-`MonthBurndownReport` |
| `GET` | `/api/settings` | OIDC/Bearer | erweitert: Bundesland (M1c) + Target-Config |
| `POST` | `/api/settings/target` | OIDC/Bearer | Default + Weekday-Overrides setzen |

Alle hinter der bestehenden OIDC/Cookie-bzw-Bearer-Middleware (M0/M1a). Kein neuer Auth-Zweig (anders als M1c-ICS).

## TUI

- **Today-Header** (bestehender Worktime-Screen): unter dem Timer eine Zeile „heute Hh Mm / Ziel · Saldo ±Hh" (Sem-Farbe grün/rot je Saldo). Konsumiert `/api/today`.
- **Monats-Burndown** kompakt im Header oder als eigene kurze Zeile: „Monat 78h / 160h · vorne 2h". Konsumiert `/api/burndown`.
- **Wochen-Sicht** als neue Sub-Sicht (Taste analog `d` für DayOffs, z.B. `w`): 7 Zeilen Mo–So mit geloggt/Ziel + horizontalem Balken, heute markiert.
- **Range-Stats** als Sub-Sicht (z.B. `t` für „Stats"): KW/Monat umschaltbar, zeigt Total/⌀/Max/Min/Workdays/Hits/Streak/Saldo. by-tag-Block ausgelassen (kein Tag-Flow).
- **Settings-Mini** (M1c-Settings erweitern): Default-Tagesziel + Weekday-Overrides anzeigen/ändern.
- SSE → `tea.Msg` → Re-render: im `eventMsg`-Zweig zusätzlich `m.reloadStats()` (Today/Week/Burndown/Range) neben `reload`/`reloadDayOffs`. TUI-Auth: M1b-Device-Flow-Token (live).

## WebUI

- `stats.templ` Page (oder Erweiterung der Worktime-Page): Today-Saldo-Snippet, Monats-Burndown-Balken (horizontal, Design-Doc-Sprache), Wochen-Sicht (7 horizontale Balken), Range-Stats-Block mit KW/Monat-Toggle (HTMX).
- Settings-Snippet (M1c-Settings erweitern): Default-Tagesziel-Input + Weekday-Override-Inputs.
- HTMX-SSE: Fragmente lauschen auf `sse:session.stopped` / `sse:dayoff.changed` / `sse:settings.changed` und re-fetchen ihr Stats-Fragment — Server rendert den neuen Stand.
- Rendering bewusst schlicht (Pixel-Politur vertagt, M1a/M1c-Präzedenz).

## Error-Handling

- `range` ≠ `week`|`month` → 400.
- `default_target_min` < 0 oder Weekday-Override < 0 → 400 (Validierung im Usecase).
- Weekday-Override leer/weggelassen → bleibt `NULL` (kein Override), kein Fehler.
- `ref` (Week) unparsebar → 400; fehlend → Default heute.
- Settings nie gesetzt → `UserSettingsStore.Get` liefert lazy Default-Row (Bundesland `NW`, `default_target_min` 480, alle Overrides `NULL`) — kein 404.
- Keine Sessions im Range → leere `Stats{}` (Nullwerte), kein Fehler; Burndown rechnet Saldo gegen erwartetes Target (negativ, wenn nichts geloggt).

## Testing

- **Domain (Carry-over-Tests mitnehmen):** `Aggregate`/`AggregateRange` (Saldo mit unworked workdays, Streak-Edges, `isHit`-Target-0-Regel), `MonthBurndownCompute` (today zählt nicht zu „expected", Live-Tail-Clamping), `PlannedTarget` (DayOff reduziert), `WeekDay.Total`/`Day.Total` (Live-Tail, Midnight-Clamp). Sicherstellen, dass die portierten Tests grün bleiben.
- **TargetResolver:** Priorität DayOff > Weekday > Default; expliziter Weekday-`0` wird respektiert (nicht vom Default überschrieben); Config-Default als Fallback; lazy-Default-Settings.
- **pgstore:** testcontainers wie M1a/M1c — `user_settings` Target-Spalten round-trip, Presence-Semantik (`NULL` vs. explizites `0`), lazy-Default beim ersten Get.
- **Usecase:** `StatsComputer.Today/Week/RangeStats/Burndown` gegen synthetische Sessions+DayOffs; Owner-Isolation (User A sieht nicht B's Sessions/Targets); `SetTargetConfig` published genau ein Event.
- **httpserver:** alle neuen Routen — 200 happy path, 400-Validierung, Owner-Scoping.
- **Done-Gate manuell:** WebUI Tagesziel ändern → TUI ~1 s; Timer in TUI → WebUI Today-Saldo & Burndown ~1 s; Urlaubstag → geplantes Monats-Target sinkt cross-surface.

## Wiring-Verification (Pflicht-Abschlusstask)

Letzter Plan-Task (Lesson „Plans need a main-wiring task"): der Server-Composition-Root (`cmd/flow-server/main.go`) verdrahtet **jeden** neuen Usecase (`TargetResolver`, `StatsComputer`, Reader), Store-Erweiterung und Handler, und ein curl-Smoke trifft **jede** neue Route (`/api/today`, `/api/week`, `/api/stats`, `/api/burndown`, `/api/settings/target`). `make ci` grün als Tor.

## Offene Punkte (für die Plan-Phase)

- **Settings-DTO-Form:** Target-Felder in die bestehende `domain.Settings`-Struct integrieren vs. eigene `TargetConfig`-Struct — eine kohärente Settings-Antwort bevorzugen, exakte Form im Plan.
- **`settings.changed`-Event** als neues Topic vs. Wiederverwendung eines bestehenden Re-Fetch-Triggers — kleinster Eingriff, der Cross-Surface-Settings-Sync auslöst.
- **`SessionStore.ListRange`** ergänzen vs. „alles laden + clientseitig filtern" im Reader — Plan entscheidet nach Datenvolumen-Erwartung (für Soenne-Einzeluser unkritisch).
- **TUI-Tastenbelegung** für Wochen-/Stats-Sicht (`w`/`t`?) im Einklang mit `tui-usability`-Keybind-Grammatik — Plan-Detail.
- **Burndown-Platzierung** in der TUI (Header-Zeile vs. eigene Sicht) — Plan-Detail.
