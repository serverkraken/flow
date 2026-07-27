# flow Kontext — cap+rank (Always-Tier budgetieren + global-Pins laden)

**Datum:** 2026-06-29 · **Branch:** Code in eigenem Worktree `cap-rank` (von `rebuild` @ `525618f`); diese Spec wird auf `rebuild` committet · **Status:** Entwurf — Brainstorm mit Soenne abgestimmt 2026-06-29 (Ranking-Modell, pinned-bypassed-D7, cap=12000 alle bestätigt); Plan ausstehend.

**Bezug:** realisiert das Token-Budget aus [[2026-06-27-flow-kontext-redesign-design]] (D8) und schließt die in [[2026-06-28-flow-kontext-b3-kontext-store-design]] offengelassene Lücke „repo/vorhaben-Tier ungekappt". Folgt direkt auf B3d (DocType-Migration, gemerged `a14ea88`).

## Warum

Der SessionStart-Bootstrap (`flow context`) liefert **~59 000 Token** gegen einen 6 000er-Cap. Grund: der B3-Kern `Compose` kappt **nur** den engagement/global-Tier. **leaf (dieses Repo) + vorhaben** liegen im ungekappten Always-Tier — und nach der B3d-Migration liegen **74 Repo-Memories** dort.

Live-Messung (`flow context --cap ∞ --json`, vs. PROD, 2026-06-29):

| Tier | Items | Token | gepinnt |
|---|---|---|---|
| instructions (immer) | 2 | 2 628 | — |
| activeContext (immer) | 1 | 298 | — |
| **leaf (flow)** | **74** | **55 724** | 4 |
| engagement (Privat) | 1 | 375 | 0 |
| global | 0 sichtbar | 0 | (8 vorhanden, alle gepinnt — s.u.) |
| **Summe** | | **59 025** | |

→ Die Aufblähung sind **zu 100 % die 74 leaf-Memories** (55,7k). vorhaben ist leer (Kette = `flow → Privat`); global ist 0 sichtbar (D7-Gate, s.u.).

## Ist-Stand (Code)

`internal/usecase/compose_context.go`:

- `Compose(chain, docs, globalAllowed, cap)` ist **rein** (kein I/O, table-testbar). Es klassifiziert nach Tier (`leaf`=Kette[0], `engagement`=letzter Knoten wenn KindEngagement, `vorhaben`=dazwischen, `global`=NodeID nil).
- **Always-Tier, unconditional in `Used`, nie gedroppt:** instructions + activeContext + **alle** leaf-Memories + **alle** vorhaben-Memories (Zeilen 193–204).
- **Nur** engagement + global werden gerankt (`pinned → updated_at desc`) und gegen den Cap gefüllt; Rest gedroppt (`DroppedCount{Engagement, Global}`).
- **D7-Tag-Gate** sitzt in `globalAllowed` (Execute), eine global-Memory kreuzt nur, wenn sie einen Tag mit einem Knoten der Kette teilt. `ListForContext(includeGlobal=true)` liefert verifiziert **alle** global-Docs (kein Tag-Vorfilter im Store — `documents.go:242`); die Gatung passiert allein über `globalAllowed[d.ID]` in `Compose`.

Render: `cmd/flow/context.go:renderContext` — Footer `Used/Cap tokens · +N engagement not shown · +M global not shown`.

## Grounding-Daten (Pins)

Manifest-Aggregat (`…/2026-06-29-b3d-manifest/manifest.tsv`) + Datei-Sizing:

- **12 Pins total: 8 global + 4 leaf.**
- Die **8 global-Pins sind genau die universellen Arbeitsweisen-Feedbacks** (`no_icons`, `no_monoliths`, `dont_descope_hobby_projects`, `generic_features`, `long_lived_integration_branch`, `navigation_discoverability`, `plan_main_wiring_task`, `subagent_git_commits_isolated`), ~3 533 Token. Soenne hat „immer laden überall" also bereits via **pin** kodiert — nur der D7-Gate blockt sie (0 sichtbar).
- Die **4 leaf-Pins** sind die flow-Anker (`tailwind_v4_templ_gotchas`, `flow_dev_env`, `flow_launch_modes`, `pgstore_goose_migrations`), 2 051 Token.

## Beschluss / Design

### 1. Uncapped, zuerst
instructions + activeContext werden **immer** injiziert (in `Used` gezählt, nie gedroppt — auch wenn sie allein den Cap übersteigen). Das sind die Working-Agreement-Regeln und der Handoff; klein und essenziell.

### 2. Ein gerankter Pool, gegen den Cap gefüllt
**Alle** Memories (leaf + vorhaben + engagement + global) wandern aus dem Always-Tier in **einen** gerankten Pool. Sortier-Schlüssel:

```
(pinned desc, tierRank asc, updatedAt desc)
   tierRank: global=0, engagement=1, vorhaben=2, leaf=3
```

Resultierende Füll-Reihenfolge:

```
pinned: global → engagement → vorhaben → leaf        (alle 12 Pins zuerst)
dann unpinned: global → engagement → vorhaben → leaf  (newest-first je Tier)
```

Rationale (Brainstorm): die *kleinen* Tiers (global, engagement, vorhaben) zuerst, damit der große leaf-Batzen sie nicht aushungert — pures „leaf zuerst" hätte global komplett verdrängt (74 leaf füllen den Cap vor jedem global). Ergebnis = „global + aktueller Projekt-Kontext", die leaf-Historie auf den Rest-Cap getrimmt.

Greedy-Füllung mit **skip-not-break**: passt ein Item nicht, wird es gedroppt und gezählt, die Schleife läuft weiter (ein einzelnes Über-Item — z.B. die 6 246-Token-Memory — blockiert kleinere dahinter nicht). Verhalten wie heute (`compose_context.go:213`).

### 3. `pinned` umgeht den D7-Gate
Eine **gepinnte** global-Memory kreuzt **immer** (sie ist Soennes explizites „immer laden"); eine **ungepinnte** global-Memory bleibt D7-gegatet (topisch). Ein-Zeilen-Änderung in `Compose`, global-Branch:

```go
if globalAllowed[d.ID] || d.Pinned { … }   // pinned bypassed den Tag-Gate
```

Plumbing (`globalAllowed`/`Execute`/Store) **unverändert** — `ListForContext` liefert eh alle globals. Effekt: die 8 gepinnten Feedbacks laden; topische globals bleiben gegatet (kein Flooding, wenn global wächst).

### 4. Pins respektieren den Cap, aber ein gedroppter Pin schreit
Pins werden zuerst gerankt, zählen aber **gegen** den Cap (eine harte Decke bleibt — ein Pin-Spree soll das Budget nicht sprengen). Wird ein **gepinntes** Item gedroppt, kommt ein **lauter, distinkter** Footer-Marker (`!! N pinned not shown — raise --cap or unpin`, emoji-frei) — eine Anomalie, vs. das stille `+N leaf not shown` für ungepinnte. Bei cap=12000 und Floor ~8,5k droppt normal kein Pin; der Pfad ist ein Sicherheitssignal.

### 5. Dropped-Accounting pro Tier
`DroppedCount{Engagement, Global}` → `DroppedCount{Leaf, Vorhaben, Engagement, Global, Pinned int}`. Die Tier-Zähler zählen **alle** Drops des Tiers; `Pinned` ist die Teilmenge gedroppter Pins (für den lauten Marker). Additive JSON-Felder → rückwärtskompatibel für Offline-Cache + MCP `flow_get_context`. Footer listet je nicht-null Tier `· +N <tier> not shown`. Gedroppte Memories bleiben in flow **suchbar** (kein Datenverlust).

### 6. Cap-Default 12 000
Der **kuratierte Floor** = instructions (2 628) + activeContext (298) + 12 Pins (2 051 leaf + ~3 533 global) ≈ **~8 510 Token**. cap+rank kann den Bootstrap **nicht** unter diesen Floor drücken — die Pins *sind* der Floor (bewusst gesetzt). Der Cap regelt nur, wie viele *neueste ungepinnte leaf*-Memories obendrauf reiten:

| cap | Ergebnis |
|---|---|
| 10 000 | Pins + ~2 neueste leaf (knapp) |
| **12 000** | Pins + ~5 neueste leaf ← **Default** |
| 14 000 | Pins + ~8 neueste leaf |

**cap=12000**: voller kuratierter Satz + die 1 engagement-Memory + ~5 neueste flow-Memories ≈ 12k, **80 % Schnitt** von 59k. Bleibt Runtime-Knopf: `FLOW_CONTEXT_BUDGET` (env) + `--cap`. Wer kleiner will, zieht am **Inhalt** (un-pinnen, AGENTS.md kürzen) — das ist Querschnitt A, nicht cap+rank.

## Surfaces

- `internal/usecase/compose_context.go` — Sortier-Schlüssel (tierRank), pinned-bypassed-Gate (1 Zeile), Füll-Schleife auf den gemeinsamen Pool, `DroppedCount`-Struktur + Zählung.
- `cmd/flow/context.go` — `renderContext`-Footer (Tier-Zeilen + lauter Pin-Marker).
- Default-Cap **6000 → 12000** an beiden Literalen: `cmd/flow-server/main.go:250` (env `FLOW_CONTEXT_BUDGET`-Fallback) und `internal/adapter/httpserver/context.go:56` (Handler-Fallback wenn `ContextBudget==0`). CLI `--cap` bleibt `0` = „nimm Server-Default".
- Tests: `compose_context_test.go`, `cmd/flow/context_test.go`, ggf. `apiclient/context_test.go` + `httpserver/context_test.go` (falls sie die `Dropped`-Form asserten).

## Tests / Done-Gate (TDD)

Neue Table-Cases in `compose_context_test.go`:
- **pinned global bypassed Gate:** gepinnte global-Memory + leerer `globalAllowed` → kreuzt; ungepinnte global + leerer `globalAllowed` → bleibt draußen (bestehender `TestCompose_GlobalGatedByTag` bleibt grün).
- **tierRank:** unter knappem Cap füllt global/engagement/vorhaben vor leaf; leaf newest-first.
- **pinned schlägt Tier:** ein gepinntes leaf vor einem ungepinnten global? Nein — pinned ist primär, also alle Pins zuerst, dann tierRank. Test fixiert die Reihenfolge.
- **per-Tier Drops + Pinned-Drop:** Zähler stimmen; ein gedroppter Pin setzt `Dropped.Pinned`.
- **Floor > Cap:** instructions+activeContext allein > cap → beide trotzdem drin, `Used > cap`, alle Memories gedroppt.
- Bestehende Cap=100000-Tests bleiben grün (alles passt).

Footer-Assertions in `context_test.go` aktualisieren. **`make ci` grün** (Coverage-Gate halten). **Live-Smoke vs PROD:** `flow context` zeigt instructions + activeContext + 8 global-Pins + 4 leaf-Pins + 1 engagement + ~5 neueste leaf, `Used` ~12k, Footer meldet `+~65 leaf not shown`. **Dogfood:** SessionStart-Block plausibel; Cap-Zahl am Footer kalibrieren (D8).

## Nicht in diesem Slice (bewusst out-of-scope)

- **Querschnitt A (Lifecycle):** dass ~70 „DONE"-Milestone-Memories überhaupt Bootstrap-Kandidaten sind, ist eine Inhalts-/Verrottungs-Frage, kein Ranking-Problem. Eigener Slice (z.B. `veraltet`-Markierung / Recency-Fenster).
- **D7-Knoten-Tags für *ungepinnte* global-Memories:** topische globals laden weiterhin nur via Tag-Match; das Taggen der Kette-Knoten ist Setup, nicht Code.
- **Die eine 6 246-Token-leaf-Memory:** droppt unter jedem sinnvollen Cap (im Footer sichtbar) — korrektes Verhalten; Größe ist ein Inhalts-Thema (Querschnitt A).

## Offene Kalibrierung

Cap-Zahl (D8): 12000 ist fundiert geschätzt, aber am Footer des **ersten Dogfoods** justieren — nicht raten. Knopf ist `FLOW_CONTEXT_BUDGET`.
