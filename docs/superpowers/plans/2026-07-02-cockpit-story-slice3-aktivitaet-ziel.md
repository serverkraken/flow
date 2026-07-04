# Cockpit-Story Slice 3 — Aktivität-Ziel (Session-Events tragen Ziel + Ziel-Pill) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Session-Events (`session.started/stopped/updated`) tragen ihr **Ziel** (Node-Id + Name + Kind) im Event-`Data`, der Activity-Log liest den persistierten `NodeRef`, und die Aktivitäts-Zeilen auf Home rendern eine **form-codierte Ziel-Pill** („startete einen Timer **auf ● flow**").

**Architecture:** Ein zentraler `(*Server).sessionEventData`-Helper baut das `Data`-Payload (ein `GetNode`-Read pro Mutation, degradiert auf id-only bei unbekanntem/fehlendem Node). `activityFor` (sse) mappt heute schon `Data["node"]→NodeRef` und `Data["name"]→Label` — dort ist **nichts** zu ändern. `BuildActivityRows` bekommt Name/Kind-Lookups (aus dem bestehenden `s.nodeMaps`) und füllt neue `Target*`-VM-Felder: live-Node gewinnt, gelöschter Node fällt auf den Label-Snapshot zurück (Pill ohne Link). Die Pill selbst rendert im Home-Logstream mit `NodeKindStyle`-Glyph+Tone (● ◆ ▲, wie die Kind-Badges).

**Tech Stack:** Go, templ + Tailwind v4 (WebUI), SSE-Emitter mit Activity-Recorder, `make generate`/`make web`/`make ci`.

## Global Constraints
- Branch: `cockpit-story` im Worktree `/Users/msoent/SourceCode/serverkraken/flow-rebuild` (Slice 2 endete bei `f997804`). Spec: `docs/superpowers/specs/2026-07-01-cockpit-story-rework-design.md` §5.3 + §4 (Puls-Karte).
- **Kein zusätzlicher Store-Roundtrip** über den einen `GetNode` im Helper hinaus; Helper-Fehler degradieren auf id-only-Data, brechen NIE die Mutation.
- Events weiterhin via `s.Emitter.Emit` (nicht `Bus.Publish`). `activityFor` bleibt unverändert (mappt `node`/`name` bereits).
- **Bulk-Endpoints** (`handleReassignSessions` worktime.go:428, `handleBulkDeleteSessions` worktime.go:452) und **Delete-Sites** (nur Session-Id bekannt, Session weg) bekommen KEINE Pill — Data bleibt wie heute (dokumentierte Nicht-Ziele).
- `make ci` grün (Gate **75 %**, `*_templ.go` ausgeschlossen — echte output-asserting Tests). Nach `.templ`: `make generate` + `*_templ.go` committen; neue Tailwind-Klassen: `make web` + `app.css` committen (verify-css ist der Schiedsrichter; Tailwind v4 scannt auch Doc-Kommentare in `.templ`/`.go`). **Kein `make fmt`.** Keine Emojis (Monospace-Glyphen ● ◆ ▲ · sind fein). `CLAUDE*.md`/`.mcp.json` nicht anfassen/committen.
- i18n de+en Parität für neue Keys.
- Falls pgstore-Docker-Tests den Daemon nicht erreichen: `DOCKER_HOST` auf den podman-Socket exportieren, einmal retry.

---

## File Structure
**Create:**
- `internal/adapter/httpserver/session_event.go` — der `sessionEventData`-Helper (eine Verantwortung, eigene Datei).
- `internal/adapter/httpserver/session_event_test.go` — Helper-Unit-Tests.

**Modify:**
- `internal/adapter/httpserver/worktime.go` — REST-Emit-Sites :61/:82/:109/:382 (Ziel), :397 unverändert-dokumentiert.
- `internal/adapter/httpserver/webui.go` — :18/:44 (Start/Stop Heute-Fragment).
- `internal/adapter/httpserver/webui_home.go` — :53/:81 (Start/Stop Home) + `BuildActivityRows`-Caller (2×) + `nodeMaps` in `handleHomeLogstream`.
- `internal/adapter/httpserver/webui_worktime.go` — :83 (Add), :122 (Edit); :95 (Delete) unverändert-dokumentiert.
- `internal/adapter/httpserver/webui_cockpit.go` — :174 (Nachbuchen), :206/:247 (Start), :218/:238 (Stop).
- `internal/adapter/webui/activity_row.go` — `ActivityRowVM.Target*` + `BuildActivityRows`-Signatur/-Logik.
- `internal/adapter/webui/home.templ` — Ziel-Pill in der Logstream-Zeile.
- `internal/i18n/catalog_de.go` / `catalog_en.go` — `activity.on`.
- Tests: `internal/adapter/webui/activity_row_test.go` (finden/anlegen), Home-Render-Test (Datei des bestehenden Home-templ-Tests spiegeln), Handler-Test in `internal/adapter/httpserver/` (Emitter-Capture-Muster der bestehenden Tests spiegeln — finden via `rg -n "Emitter" internal/adapter/httpserver/*_test.go | head`).

---

## Task 1: `sessionEventData`-Helper + alle Session-Emit-Sites tragen das Ziel

**Files:**
- Create: `internal/adapter/httpserver/session_event.go`, `internal/adapter/httpserver/session_event_test.go`.
- Modify: `worktime.go`, `webui.go`, `webui_home.go`, `webui_worktime.go`, `webui_cockpit.go` (Zeilennummern unten sind Stand `f997804` — bei Drift den umgebenden Code-Auszügen vertrauen).

**Interfaces:**
- Produces: `(s *Server) sessionEventData(ctx context.Context, ownerID, sessionID string, nodeID *string) map[string]any` — immer `{"id": sessionID}`; bei buchbarem Node zusätzlich `"node"` (Id), `"name"`, `"kind"` (string). Von Task 2/3 NICHT direkt konsumiert (die lesen den persistierten `ActivityEntry`), aber SSE-Clients (Slice 4 Puls) sehen das volle Payload.

- [ ] **Step 1: Failing Helper-Test** — Create `internal/adapter/httpserver/session_event_test.go` (Paket wie die Nachbar-Tests; minimaler Server mit `GetNode`-Usecase über `testutil.NewFakeNodeStore()` — Konstruktions-Muster eines bestehenden Handler-Tests spiegeln):

```go
func TestSessionEventData(t *testing.T) {
	ctx := context.Background()
	ns := testutil.NewFakeNodeStore()
	n, _ := domain.NewNode("n1", "u1", "flow", "flow", time.Now())
	n.Kind = domain.KindRepo
	_, _ = ns.Create(ctx, n)
	s := &Server{GetNode: usecase.GetNode{Nodes: ns}}

	nid := "n1"
	got := s.sessionEventData(ctx, "u1", "sess-1", &nid)
	if got["id"] != "sess-1" || got["node"] != "n1" || got["name"] != "flow" || got["kind"] != "repo" {
		t.Errorf("booked session data = %#v", got)
	}

	if got := s.sessionEventData(ctx, "u1", "sess-2", nil); len(got) != 1 || got["id"] != "sess-2" {
		t.Errorf("unbooked session must carry id only, got %#v", got)
	}

	ghost := "ghost"
	if got := s.sessionEventData(ctx, "u1", "sess-3", &ghost); len(got) != 1 || got["id"] != "sess-3" {
		t.Errorf("unknown node must degrade to id only, got %#v", got)
	}
}
```
> Falls `usecase.GetNode` anders heißt: `rg -n "GetNode" internal/adapter/httpserver/server.go` zeigt Feldname+Typ; exakt den verwenden.

- [ ] **Step 2: Run — fails** (`sessionEventData` undefined)

Run: `go test ./internal/adapter/httpserver/ -run TestSessionEventData`
Expected: FAIL (undefined).

- [ ] **Step 3: Helper** — Create `internal/adapter/httpserver/session_event.go`:

```go
package httpserver

import "context"

// sessionEventData builds the Data payload for session.* events: always the
// session id, plus — when the session is booked to a node — the target's
// identity (node id, name, kind) so the activity log persists NodeRef+Label
// and live SSE consumers can render the target without a lookup. A missing or
// unreadable node degrades to id-only; emitting never fails the mutation.
func (s *Server) sessionEventData(ctx context.Context, ownerID, sessionID string, nodeID *string) map[string]any {
	data := map[string]any{"id": sessionID}
	if nodeID == nil {
		return data
	}
	n, err := s.GetNode.Execute(ctx, ownerID, *nodeID)
	if err != nil {
		return data
	}
	data["node"] = n.ID
	data["name"] = n.Name
	data["kind"] = string(n.Kind)
	return data
}
```

- [ ] **Step 4: Run — passes.** `go test ./internal/adapter/httpserver/ -run TestSessionEventData -v` → PASS.

- [ ] **Step 5: REST-Sites** — `internal/adapter/httpserver/worktime.go` (alle vier haben `sess` bereits im Scope):

:61 und :82 (`handleStartSession`, add-past + live) sowie :109 (`handleStopSession`) und :382 (`handleEditSessionByID`/der Edit-Handler dort) — die vier `Data: map[string]any{"id": sess.ID}` ersetzen durch:

```go
	Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)
```

:397 (`handleDeleteSession`) bleibt `Data: map[string]any{"id": id}` — Session ist gelöscht, kein Ziel mehr auflösbar; Kommentar ergänzen:

```go
	// deleted: the session is gone — id only, no target (documented non-goal).
```

:428/:452 (Bulk) bleiben unverändert (multi-target, kein einzelnes Ziel).

- [ ] **Step 6: Heute-Fragment-Sites** — `internal/adapter/httpserver/webui.go`:

:18 (`handleWebStart`) — Session einfangen und Ziel mitgeben (Start ohne Node → id-only via Helper):

```go
	if sess, err := s.StartSession.Execute(r.Context(), u.ID, nil, nil, ""); err == nil {
		s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStarted, UserID: u.ID,
			Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
	}
```

:44 (`handleWebStop`) — Ergebnis von `StopSession` einfangen:

```go
	sess, err := s.StopSession.Execute(r.Context(), u.ID, sessionID, &nodeID)
	if err != nil {
		// (bestehender Fehlerzweig unverändert)
		...
	}
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionStopped, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
```
(Der bestehende `if _, err := …`-Einzeiler wird dafür zu Deklaration + Fehlerzweig auseinandergezogen; Fehlerbehandlung Wort für Wort behalten.)

- [ ] **Step 7: Home-Sites** — `internal/adapter/httpserver/webui_home.go` :53 (`handleHomeStart`) + :81 (`handleHomeStop`): exakt dieselben zwei Umbauten wie Step 6 (die Handler sind erklärte Spiegel von webui.go — Kommentar sagt es).

- [ ] **Step 8: Cockpit-Sites** — `internal/adapter/httpserver/webui_cockpit.go`:

:174 (Nachbuchen, `AddSession`): `if _, err := …` → `if sess, err := …` und

```go
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionUpdated, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
```

:206 + :247 (`handleWebNodeStart` / Switch-Start): `if _, err := s.StartSession.Execute(r.Context(), u.ID, &id, nil, "")` bzw. `(…, &nid, …)` → `if sess, err := …` und Data via Helper (`sess.NodeID`).

:218 (`handleWebNodeStop`) + :238 (Switch-Stop): `if _, err := s.StopSession.Execute(…)` → `if sess, err := …` (bzw. bei :238 `sess, err :=` mit bestehendem Fehlerzweig) und Data via Helper (`sess.NodeID` — die gestoppte Session trägt den gebuchten Node).

- [ ] **Step 9: Worktime-WebUI-Sites** — `internal/adapter/httpserver/webui_worktime.go`:

:83 (`handleWebAdd`, `AddSession`) + :122 (`handleWebEdit`, `EditSession`): Ergebnis einfangen (`if sess, err := …`) und

```go
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionUpdated, UserID: u.ID,
		Data: s.sessionEventData(r.Context(), u.ID, sess.ID, sess.NodeID)})
```

:95 (`handleWebDelete`): Ziel nicht mehr auflösbar → nur die Session-Id mitgeben (besser als heute gar nichts):

```go
	s.Emitter.Emit(r.Context(), domain.Event{Type: domain.EventSessionDeleted, UserID: u.ID,
		Data: map[string]any{"id": r.FormValue("sessionId")}})
```

- [ ] **Step 10: Handler-Test (Emit-Payload end-to-end)** — Emitter-Capture-Muster der bestehenden httpserver-Tests finden (`rg -n "Emitter" internal/adapter/httpserver/*_test.go | head` — irgendein Test injiziert einen Capture-/Fake-Emitter; dessen Harness spiegeln). Ein Test: Cockpit-Start auf einen geseedeten Repo-Node `POST /nodes/{id}/start` → das gecapturte `session.started`-Event trägt `Data["node"]=="n1"`, `Data["name"]=="flow"`, `Data["kind"]=="repo"`, `Data["id"]!=""`. Zweiter Fall im selben Test: `POST /ui/home/start` (ohne Node) → Data hat NUR `id`.

```go
func TestSessionEvents_CarryTarget(t *testing.T) {
	// harness: mirror the existing handler-test server construction, inject the
	// capturing emitter the other tests use, seed node n1 ("flow", KindRepo).
	// 1) authed POST /nodes/n1/start  → captured session.started event:
	//    Data["node"]=="n1" && Data["name"]=="flow" && Data["kind"]=="repo" && Data["id"] != ""
	// 2) authed POST /ui/home/start (no node) → captured session.started event:
	//    len(Data)==1 && Data["id"] != ""
	// Assertions verbindlich; Helfer an die reale Harness anpassen.
}
```

- [ ] **Step 11: CI + Commit**

Run: `make ci` → grün.

```bash
git add internal/adapter/httpserver/
git commit -m "feat(activity): session events carry their target node (id+name+kind) via sessionEventData"
```

---

## Task 2: `ActivityRowVM`-Ziel-Felder + `BuildActivityRows`-Lookup + Home-Caller

**Files:**
- Modify: `internal/adapter/webui/activity_row.go`, `internal/adapter/httpserver/webui_home.go` (:218 `homeDataFor`, :267 `handleHomeLogstream`).
- Test: `internal/adapter/webui/activity_row_test.go` (existiert? `rg -l "BuildActivityRows" internal/adapter/webui | rg test` — sonst anlegen; Paket `webui` intern oder `webui_test` je nach Bestand).

**Interfaces:**
- Consumes: `domain.ActivityEntry.NodeRef/Label/Kind` (persistiert seit Migration 0024; Task 1 füllt NodeRef+Label für session.*-Events), `(*Server).nodeMaps(ctx, ownerID) (names map[string]string, colors map[string]string, kinds map[string]domain.NodeKind, error)` (webui_wissen.go:231).
- Produces: `ActivityRowVM.TargetName string`, `TargetKind domain.NodeKind` (`""` = unbekannt/gelöscht), `TargetHref string` (`"/nodes/{id}"` nur bei live existierendem Node); neue Signatur `BuildActivityRows(entries []domain.ActivityEntry, names map[string]string, kinds map[string]domain.NodeKind, now time.Time) []ActivityRowVM` (Task 3 konsumiert die Felder im templ).

- [ ] **Step 1: Failing Unit-Test** — `internal/adapter/webui/activity_row_test.go` (anhängen/anlegen; Paket an Bestand anpassen, Beispiel extern):

```go
func TestBuildActivityRows_TargetPill(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.Local)
	ref := func(s string) *string { return &s }
	entries := []domain.ActivityEntry{
		// 1: session booked to a live node → live name+kind win, label cleared, href set
		{ActorKind: "human", ActorRef: "msoent", Kind: "session.started",
			NodeRef: ref("n1"), Label: ref("old-snapshot"), At: now.Add(-5 * time.Minute)},
		// 2: session booked to a since-deleted node → label snapshot, no kind, no href
		{ActorKind: "human", ActorRef: "msoent", Kind: "session.stopped",
			NodeRef: ref("gone"), Label: ref("Altprojekt"), At: now.Add(-10 * time.Minute)},
		// 3: unbooked session → no pill
		{ActorKind: "human", ActorRef: "msoent", Kind: "session.started", At: now.Add(-1 * time.Minute)},
		// 4: document event with NodeRef must NOT grow a pill (documents keep their label link)
		{ActorKind: "agent", ActorRef: "claude", Kind: "document.updated",
			TargetRef: ref("d1"), Label: ref("Spec"), NodeRef: ref("n1"), At: now.Add(-2 * time.Minute)},
	}
	names := map[string]string{"n1": "flow"}
	kinds := map[string]domain.NodeKind{"n1": domain.KindRepo}

	rows := webui.BuildActivityRows(entries, names, kinds, now)

	if r := rows[0]; r.TargetName != "flow" || r.TargetKind != domain.KindRepo || r.TargetHref != "/nodes/n1" || r.Label != "" {
		t.Errorf("live target row = %+v", r)
	}
	if r := rows[1]; r.TargetName != "Altprojekt" || r.TargetKind != domain.NodeKind("") || r.TargetHref != "" || r.Label != "" {
		t.Errorf("deleted-node row = %+v", r)
	}
	if r := rows[2]; r.TargetName != "" {
		t.Errorf("unbooked row must have no pill, got %+v", r)
	}
	if r := rows[3]; r.TargetName != "" || r.Label != "Spec" || r.Href != "/wissen/d1" {
		t.Errorf("document row must keep label link and grow no pill, got %+v", r)
	}
}
```

- [ ] **Step 2: Run — fails to compile** (Signatur/Felder). `go test ./internal/adapter/webui/ -run TestBuildActivityRows_TargetPill` → FAIL.

- [ ] **Step 3: VM + Logik** — `internal/adapter/webui/activity_row.go`:

Felder an `ActivityRowVM` anhängen:

```go
	// Ziel-Pill (nur session.* mit NodeRef): live node name+kind, or the Label
	// snapshot when the node no longer exists (then kind=="" and no href).
	TargetName string
	TargetKind domain.NodeKind
	TargetHref string
```

`BuildActivityRows` ersetzen:

```go
// BuildActivityRows converts domain.ActivityEntry slices to ActivityRowVM slices.
// names/kinds are the owner's current node lookups (s.nodeMaps): a session row
// whose NodeRef still resolves renders the live target pill (linked); a deleted
// node falls back to the persisted Label snapshot (unlinked pill, no kind).
// `now` is used for RelTime formatting only.
func BuildActivityRows(entries []domain.ActivityEntry, names map[string]string, kinds map[string]domain.NodeKind, now time.Time) []ActivityRowVM {
	rows := make([]ActivityRowVM, 0, len(entries))
	for _, e := range entries {
		var label string
		if e.Label != nil {
			label = *e.Label
		}
		var href string
		if strings.HasPrefix(e.Kind, "document.") && e.TargetRef != nil {
			href = "/wissen/" + *e.TargetRef
		}
		row := ActivityRowVM{
			ActorKind: e.ActorKind,
			ActorRef:  e.ActorRef,
			VerbKey:   "activity.verb." + e.Kind,
			Label:     label,
			Href:      href,
			RelTime:   fmtRelTime(e.At, now),
		}
		if strings.HasPrefix(e.Kind, "session.") && e.NodeRef != nil {
			if name, ok := names[*e.NodeRef]; ok {
				row.TargetName = name
				row.TargetKind = kinds[*e.NodeRef]
				row.TargetHref = "/nodes/" + *e.NodeRef
			} else {
				row.TargetName = label // snapshot of the deleted node's name
			}
			// the label for session rows IS the node name — the pill replaces it.
			row.Label = ""
		}
		rows = append(rows, row)
	}
	return rows
}
```

- [ ] **Step 4: Run — passes.** `go test ./internal/adapter/webui/ -run TestBuildActivityRows -v` → PASS.

- [ ] **Step 5: Caller anpassen** — `internal/adapter/httpserver/webui_home.go`:

`homeDataFor` (~:218): der Activity-Block wird zu

```go
	if s.ListActivity.Activities != nil {
		entries, _, _ := s.ListActivity.Execute(ctx, u.ID, nil, nil, 15, 0)
		names, _, kinds, nerr := s.nodeMaps(ctx, u.ID)
		if nerr != nil {
			slog.WarnContext(ctx, "home: nodeMaps for activity failed", "err", nerr)
		}
		vm.LogEntries = webui.BuildActivityRows(entries, names, kinds, now)
		actors, _ := s.ListActivity.Actors(ctx, u.ID)
		vm.LogActors = actors
	}
```

`handleHomeLogstream` (~:267): vor dem VM-Bau `names, _, kinds, _ := s.nodeMaps(r.Context(), u.ID)` laden (Fehler wie oben nur warnen) und `webui.BuildActivityRows(entries, names, kinds, now)` aufrufen.

> `homeDataFor` ruft `s.nodeMaps` ggf. schon für die Docs-Sektion auf (`_, colors, _, err := s.nodeMaps(…)`, ~:207) — dann DIESEN einen Aufruf auf `names, colors, kinds, err := …` erweitern und für beide Sektionen wiederverwenden statt doppelt zu laden.

- [ ] **Step 6: Build + bestehende Tests** — `go build ./... && go test ./internal/adapter/httpserver/ ./internal/adapter/webui/` → grün (Compile-Fehler an weiteren `BuildActivityRows`-Callern? `rg -n "BuildActivityRows" internal | rg -v _test` — es gibt genau die zwei in webui_home.go).

- [ ] **Step 7: CI + Commit**

```bash
make ci
git add -A ':!/.mcp.json'
git commit -m "feat(activity): activity rows resolve the session target (live name+kind, snapshot fallback)"
```

---

## Task 3: Ziel-Pill im Home-Logstream (templ) + i18n

**Files:**
- Modify: `internal/adapter/webui/home.templ` (Logstream-`<li>`-Zeile, ~:217-229), `internal/i18n/catalog_de.go` + `catalog_en.go`.
- Test: Render-Test bei den bestehenden Home-templ-Tests (finden: `rg -ln "LogEntries|homeLogstream" internal/adapter/webui/*_test.go` — dort anhängen; die Pill-Komponente ist paketintern, also internes Testfile wie in Slice 2 Task 6).

**Interfaces:**
- Consumes: `ActivityRowVM.TargetName/TargetKind/TargetHref` (Task 2), `NodeKindStyle(kind)` + `kindToneClass(tone)` (nodekind.go), i18n `activity.on`.

- [ ] **Step 1: i18n-Keys** (beide Kataloge, beim `activity.*`-Block):

```
"activity.on": "auf"   (de)  /  "on"   (en)
```

- [ ] **Step 2: Failing Render-Test** (an das bestehende Home-Render-Testfile anhängen; `renderToBuf`-Muster wie in `cockpit_render_test.go`, ggf. lokal spiegeln — das Logstream-templ heißt so wie der `LogEntries`-Renderer in home.templ, beim Implementieren dort ablesen und im Test exakt verwenden):

```go
func TestHomeLogstream_TargetPill(t *testing.T) {
	ctx := context.Background()
	vm := HomeVM{LogEntries: []ActivityRowVM{
		{ActorKind: "human", ActorRef: "msoent", VerbKey: "activity.verb.session.started",
			TargetName: "flow", TargetKind: domain.KindRepo, TargetHref: "/nodes/n1", RelTime: "vor 5 Min"},
		{ActorKind: "human", ActorRef: "msoent", VerbKey: "activity.verb.session.stopped",
			TargetName: "Altprojekt", RelTime: "vor 10 Min"}, // deleted node: pill without link
	}}
	body := renderToBuf(t, ctx, homeLogstream(vm)) // exakten templ-Namen aus home.templ übernehmen

	if !strings.Contains(body, `href="/nodes/n1"`) || !strings.Contains(body, "flow") {
		t.Errorf("live pill must link the node, got: %s", body)
	}
	if !strings.Contains(body, "●") {
		t.Errorf("live pill must carry the repo form-glyph, got: %s", body)
	}
	if !strings.Contains(body, "auf") {
		t.Errorf("connector word missing, got: %s", body)
	}
	if !strings.Contains(body, "Altprojekt") || strings.Contains(body, `href="/nodes/"`) {
		t.Errorf("deleted-node pill must render unlinked, got: %s", body)
	}
}
```
> Signatur-Realität prüfen: nimmt das Logstream-templ `HomeVM` oder Einzelfelder? An den Bestand anpassen, Assertions verbindlich.

- [ ] **Step 3: Run — fails.** `go test ./internal/adapter/webui/ -run TestHomeLogstream_TargetPill` → FAIL (Pill fehlt).

- [ ] **Step 4: Pill rendern** — `internal/adapter/webui/home.templ`, in der Logstream-Zeile nach dem Verb-`<span>` (vor dem Label/Href-Block) einfügen:

```templ
					if row.TargetName != "" {
						<span class="text-muted">{ components.T(ctx, "activity.on") }</span>
						@activityTargetPill(row)
					}
```

und am Datei-Ende die Komponente (Pill-Klassen spiegeln `nodeKindBadge`, nodes.templ):

```templ
// activityTargetPill renders the form-coded target of a session activity row:
// linked pill with kind glyph for a live node, plain snapshot pill otherwise.
templ activityTargetPill(row ActivityRowVM) {
	{{ k := NodeKindStyle(row.TargetKind) }}
	if row.TargetHref != "" {
		<a href={ templ.SafeURL(row.TargetHref) } class={ "inline-flex shrink-0 items-center gap-1.5 rounded-md border px-2 py-0.5 text-[.75rem] font-medium hover:underline", kindToneClass(k.Tone) }>
			<span aria-hidden="true">{ k.Glyph }</span> { row.TargetName }
		</a>
	} else {
		<span class="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-line px-2 py-0.5 text-[.75rem] font-medium text-muted">
			{ row.TargetName }
		</span>
	}
}
```
> Achtung Fallback: `NodeKindStyle("")` liefert die default-Badge (Glyph „·") — der else-Zweig nutzt sie bewusst NICHT (kein Glyph, neutraler Rahmen), damit ein gelöschter Node nicht als Repo verkleidet wird.

- [ ] **Step 5: Bestehende Label-Zeile für Session-Rows tot?** Prüfen: der bestehende `else if row.Label != ""`-Zweig bleibt für node/document/dayoff-Rows zuständig; Session-Rows haben `Label == ""` (Task 2 leert es) → keine Doppel-Anzeige. Keine Code-Änderung, nur Sichtprüfung (im Report notieren).

- [ ] **Step 6: Generate + Run — passes**

Run: `make generate && go test ./internal/adapter/webui/ -run TestHomeLogstream -v`
Expected: PASS.

- [ ] **Step 7: Web + CI + Commit** (Pill-Klassen existieren bereits von `nodeKindBadge` — `make web` nur falls verify-css meckert; dann app.css mitcommitten)

```bash
make ci
git add -A ':!/.mcp.json'
git commit -m "feat(webui): activity rows render the form-coded target pill (auf ● flow)"
```

---

## Task 4: Wiring-Verifikation + Done-Gate + Holistic Review

- [ ] **Step 1: Wiring-Audit**

```bash
rg -c "sessionEventData" internal/adapter/httpserver/worktime.go internal/adapter/httpserver/webui.go internal/adapter/httpserver/webui_home.go internal/adapter/httpserver/webui_worktime.go internal/adapter/httpserver/webui_cockpit.go
rg -n "BuildActivityRows" internal/adapter/httpserver/webui_home.go
rg -n "activityTargetPill" internal/adapter/webui/home.templ
```

Expected: worktime 4 + webui 2 + home 2 + webui_worktime 2 + cockpit 5 Helper-Aufrufe; 2 BuildActivityRows-Caller mit names/kinds; Pill-Render + Definition.

- [ ] **Step 2: Voll-CI** — `make generate && make ci` (Coverage ≥ 75 % notieren).

- [ ] **Step 3: Live-Done-Gate (Dev-Stack, scripted Dex-Login wie Slice 2)**
  1. Timer im Cockpit auf einen Repo-Node starten → `GET /api/v1/activity?limit=3` (Bearer): jüngster Eintrag `kind=session.started` mit `nodeRef` + `label` = Node-Name.
  2. Home laden (`/`): Logstream-Zeile zeigt „startete einen Timer **auf ● {name}**" als Link auf `/nodes/{id}`; Klick-URL stimmt.
  3. Timer stoppen → „stoppte den Timer auf …"-Pill erscheint (SSE-Reload des Fragments).
  4. Unbooked-Start via Home-Button → Zeile ohne Pill (wie vorher, kein Regressions-Müll).
  5. Node löschen, dessen Sessions Aktivität haben → alte Zeilen zeigen die Snapshot-Pill ohne Link.
- [ ] **Step 4: Browser-Dogfood (Soenne)** — Pulse-Gefühl der Zeilen, de/en-Wording „auf".
- [ ] **Step 5: Holistic Review (Opus)** — BASE = `f997804`. Fokus: alle 15 Emit-Sites tragen konsistent Data (keine vergessene Site — `rg "EventSession" internal/adapter/httpserver | rg Emit` gegen die Task-1-Liste), Helper degradiert statt zu failen, Label-Clearing verliert keine Information für nicht-session-Rows, Pill-XSS (nur templ-escaped Strings, SafeURL), i18n-Parität.
- [ ] **Step 6: Follow-up-Commit nur bei Review-Findings.**

---

## Self-Review (plan author)
**Spec-Coverage (§5.3):** Emit-Sites tragen Ziel (node+name+kind) → Task 1 (alle 15 Sites einzeln aufgeführt, Bulk/Delete als dokumentierte Nicht-Ziele); NodeRef wird gelesen + Row-VM-Ziel-Feld → Task 2; Ziel-Pill form-codiert → Task 3 (NodeKindStyle-Glyph+Tone, Mensch/Agent-Avatar existiert schon); Wiring/Gate/Review → Task 4. `activityFor` unverändert (mappt node/name bereits — im Architecture-Absatz festgehalten). ✓
**Placeholder-Scan:** Harness-adaptive Stellen (Task 1 Step 1/10, Task 3 Step 2) benennen konkrete Suchbefehle + verbindliche Assertions statt TODOs; keine „TBD"/„similar to". ✓
**Typ-Konsistenz:** `sessionEventData(ctx, ownerID, sessionID string, nodeID *string) map[string]any` überall; `BuildActivityRows(entries, names map[string]string, kinds map[string]domain.NodeKind, now)`; VM-Felder `TargetName/TargetKind/TargetHref` in Task 2 definiert, Task 3 konsumiert exakt diese; `nodeMaps`-Rückgabe (names, colors, kinds, err) wie webui_wissen.go:231. ✓
**Reuse:** Pill spiegelt `nodeKindBadge`-Klassen; Lookup nutzt bestehendes `s.nodeMaps`; kein neues Port/Store; ein GetNode pro Mutation als einziger Zusatz-Read.
