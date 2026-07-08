# tmux-Worktime-Integration — Status-Segment + Stop-Verb (Design Spec)

**Datum:** 2026-07-08 · **Branch:** `tmux-status` (ab `rebuild`) · **Status:** approved

## Problem

Die tmux-Seite (Plugin `worktime`, `segment-wrap.sh`, Bindings) ist intakt, ruft aber
Verben auf, die es im Rebuild-Binary nicht mehr gibt. Ergebnis: leeres Status-Segment,
totes `prefix+E`.

| tmux-Stelle | ruft auf | Rebuild-Status |
|---|---|---|
| Status-Segment (`status-right`, Tick alle **5 s**) | `flow worktime status` | fehlt |
| `prefix+E` (sofort stoppen) | `flow worktime stop` | fehlt |
| `prefix+W` Deep-Link / Sidekick-View „flow" | `goto.sh` / `flow sidekick` | fehlt — **Non-Goal, Slice 2** |

Der strukturelle Unterschied zur alten Welt: früher las `status` lokale Dateien
(gratis, nie offline), jetzt ist `flow` ein REST-Client gegen `flow-server`
(PROD: flow.thebackend.org). Bei 5-s-Tick und fünf nötigen Lesequellen ist
Naiv-Polling keine Option.

## Entscheidungen (mit Soenne geklärt)

1. **Scope:** `flow worktime status` + `flow worktime stop`. Sidekick-Deep-Links
   (`prefix+W`, `flow sidekick`-View) sind ein eigener Slice.
2. **Offline:** Stale + Dim — letzter Cache-Stand wird komplett in Dim-Farbe
   weitergezeigt, die laufende Session tickt lokal weiter; nach **30 min** ohne
   erfolgreichen Fetch verschwindet das Segment (leere Ausgabe).
3. **Architektur:** Ein neuer aggregierter Server-Endpoint + client-seitiger
   Cache mit **30 s TTL**. Kein SSE-Daemon (Overkill), kein 5–6-Endpoint-Fanout.
4. **Stop-Buchung:** Der Server hat Buchungspflicht beim Stop
   (`ErrProjectRequired`, Routen-Test erwartet 400 bei `{}`) — die bleibt
   unangetastet. `flow worktime stop` wird deshalb ein **Picker-Popup**
   (Entscheidung Soenne, statt MRU-Blindbuchung): nie falsch gebucht, ein
   Tastendruck mehr.

## 1. Server — aggregierter Status-Endpoint

**Route:** `GET /api/v1/worktime/status` (owner-scoped wie alle anderen, read-only,
kein SSE-Emit).

**Usecase:** `usecase.WorktimeStatus` (ein File, „keine Monolithen") — reine
Komposition der bestehenden Reader, analog dem alten `StatusComposer`:
heute + Woche + DayOffs + Streak + Monats-Burndown. **Keine neuen Store-Methoden.**

**DTO (Wire-Form):**

```json
{
  "date": "2026-07-08",
  "loggedMin": 312,
  "targetMin": 480,
  "running": true,
  "activeSessionId": "s-…",
  "activeStart": "2026-07-08T13:05:00+02:00",
  "activeNodeId": null,
  "dayOff": { "kind": "vacation", "label": "Urlaub" },
  "week": [
    { "date": "2026-07-06", "loggedMin": 480, "targetMin": 480,
      "workday": true, "isToday": false, "dayOffKind": null }
  ],
  "streak": 4,
  "burndown": { "saldoMin": 130, "targetMin": 9600 }
}
```

- `loggedMin` = **abgeschlossene** Zeit heute (ohne laufende Session) — der Client
  extrapoliert die laufende Session selbst aus `activeStart`.
- `activeSessionId` / `activeStart` / `activeNodeId` nur bei `running: true`;
  `activeNodeId` gesetzt, wenn die Session schon beim Start gebucht wurde.
- `dayOff` = heutiger Frei-Eintrag oder `null`; `week[].dayOffKind` liefert die
  Kind pro Tag für die Pace-Dot-Farben (holiday/vacation/sick).
- `burndown` = Monatssaldo-Light (`saldoMin`, `targetMin`); `targetMin: 0` =
  kein Ziel konfiguriert → Saldo-Marker entfällt.

**Wiring:** Feld am `httpserver.Server` + Konstruktion in
`cmd/flow-server/main.go` — eigener Verifikations-Task im Plan (curl-Smoke),
wie in [[feedback_plan_main_wiring_task]] gefordert.

## 2. Client — Renderer, Cache, `flow worktime status`

### Renderer `internal/statusline` (pur, client-only)

Port von `domain/status.go` + `status_test.go` aus dem alten Repo (Layout und
Semantik unverändert), Inputs kommen statt aus fünf Readern aus dem einen DTO:

- Banner `⏱ HH:MM` (running, bold: cyan → gelb ab Ziel−2 h → grün ab Ziel →
  rot ab Ziel+4 h) bzw. `‖` idle (grün wenn Ziel erreicht, sonst dim).
- `▶ H:MM` laufende Session (midnight-clamped) + `→HH:MM` Ziel-ETA solange
  unter Ziel; `▶!` gelb/rot ab MaxStreak / 2×MaxStreak.
- `✓` bei erreichtem Ziel, Pace-Dots Mo–Fr (●/○, Frei-Kinds: Holiday=Blue,
  Vacation=Purple, Sick=Orange), `Streak N` ab 3, `▲ +Nh`/`▼ -Nh` Monatssaldo
  ab |1 h|.
- Leere Ausgabe, wenn heute nichts getrackt, keine Wochen-Aktivität und kein
  Frei-Eintrag — `segment-wrap.sh` unterdrückt dann den Delimiter.

### Palette + MaxStreak aus tmux-Optionen

`@tn_green` … `@tn_dim` überschreiben die Tokyonight-Defaults; neu
`@flow_max_streak_min` (0/fehlt = Warnung aus — alte `worktime.conf`-Semantik).
Gelesen mit **einem** `tmux show-options -g`-Aufruf (parsen, nicht 8 Einzelcalls).
Außerhalb von tmux (`$TMUX` leer): Defaults, kein Fehler.

### Cache `~/.cache/flow/worktime-status.json`

- Inhalt: `{ fetchedAt, status: <DTO> }`, Schreiben atomar (tmp + rename);
  konkurrierende Ticks: last-writer-wins, kein Lock nötig.
- Tick-Ablauf: Cache lesen → jünger als **30 s** → nur rendern (laufende
  Session tickt lokal aus `activeStart`). Älter → Fetch mit **~2 s Timeout**;
  Erfolg → Cache erneuern, frisch rendern; Fehler → Stale-Pfad.
- **Stale-Pfad:** Cache-Stand komplett in Dim rendern (alle Farb-Slots → Dim,
  eindeutiges Offline-Signal); `fetchedAt` älter als **30 min** → leere Ausgabe.
- Der Server sieht ~2 Requests/min pro Maschine statt 12 × 5 Endpoints.

### CLI-Verb `status`

- `cmd/flow/worktime.go` bekommt `statusCmd` als Subcommand.
- **Nie** interaktiv: abgelaufenes/fehlendes Token = Offline-Pfad, niemals
  Device-Flow-Prompt aus dem Status-Tick.
- **Immer** Exit 0, kein stderr-Output (Segment-Kontext verschluckt nichts,
  aber Fehlertext im Status-Bar wäre schlimmer als gar nichts).

## 3. Client — `flow worktime stop` (Picker-Popup)

- **tmux-Seite (einzige dotfiles-Änderung):** im worktime-Plugin
  `bind E run-shell -b 'flow worktime stop'` → `bind E display-popup -E 'flow worktime stop'`
  (Picker braucht TTY). Commit im dotfiles-Repo, nicht hier.
- **Ablauf:**
  1. Status **frisch** vom Server holen (Cache umgehen) → `activeSessionId`.
  2. Keine laufende Session → kurze Meldung, Exit 0, Popup schließt.
  3. Session hat schon einen Node (`activeNodeId`) → sofort stoppen und darauf
     buchen, kein Picker.
  4. Unzugeordnet (Normalfall) → fuzzy/MRU-Picker über die bookable Nodes —
     dieselbe Picker-Komponente wie TUI-Booking-Dialog / `flow project bind`
     (`projectbind_picker.go`-Pattern). Enter = buchen + stoppen,
     Esc = abbrechen **ohne** zu stoppen.
  5. Nach Erfolg: gebuchten Posten ausgeben, Cache invalidieren (Datei löschen)
     → Segment aktualisiert beim nächsten 5-s-Tick.
- `--node <ref>` für nicht-interaktive Nutzung; ohne TTY und ohne Flag →
  Fehlermeldung statt hängendem Prompt.
- Stop über den bestehenden `POST /api/v1/sessions/{id}/stop` — Midnight-Split
  und SSE-Events macht der vorhandene `StopSession`-Usecase.

## 4. Tests

- `internal/statusline`: portierte alte Suite (Banner-Schwellen, ▶/ETA,
  Midnight-Clamp, Pace-Dots, Frei-Farben, Saldo-Rundung, Leer-Fälle) +
  Dim-Rendering im Stale-Pfad.
- `usecase.WorktimeStatus`: Fake-Stores (running/idle, Frei heute,
  Wochen-Kinds, Burndown ohne Ziel).
- httpserver: Routen-Test `GET /api/v1/worktime/status` (Auth, DTO-Shape).
- Cache-Logik: Fake-Clock/FS — TTL-Grenzen (29 s/31 s), Stale-Grenze (30 min),
  atomares Schreiben, korrupte Cache-Datei → wie „kein Cache".
- `stop`: Kaskade (keine Session / Node vorhanden / Picker / `--node` / kein TTY).
- Gate: `make ci` grün.

## Non-Goals (Slice 2, falls gewünscht)

- Sidekick-Deep-Links: `prefix+W`, `~/.cache/flow/next-screen`-Protokoll,
  `flow sidekick`-Alias (Sidekick-View „flow" crash-looped bis dahin weiter,
  wenn man sie zeigt — bekannt, unverändert).
- Kein Start-Verb, kein Pause/Resume (TUI/WebUI decken das ab).
- Keine Änderung an Buchungspflicht, DTOs bestehender Endpoints oder WebUI.

## Risiken / Ränder

- **Mitternacht:** laufende Session gestern gestartet → Renderer clamped auf
  heutige Mitternacht (Banner und ▶ zeigen dieselbe Zahl) — Verhalten wie alt.
- **Multi-Maschine:** Cache ist pro Maschine; 30 s Sichtverzug auf Mutationen
  anderer Geräte ist akzeptiert.
- **Uhrzeit-Skew Client/Server:** Extrapolation nutzt lokale Uhr gegen
  Server-`activeStart` (RFC 3339 mit Offset) — Minuten-Genauigkeit reicht fürs
  Segment.
