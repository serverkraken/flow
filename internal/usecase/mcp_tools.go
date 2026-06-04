package usecase

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ToolDef describes a tool the MCPTools dispatcher knows about. The
// composition root converts these into MCP wire-format tool catalogs —
// the usecase layer stays free of any transport types.
type ToolDef struct {
	Name        string
	Description string
	// InputSchema is a JSON-Schema object describing the tool arguments.
	// Built as a plain map so the usecase layer doesn't need a schema
	// builder dependency.
	InputSchema map[string]any
}

// ToolResult is the outcome of a tool call. IsError surfaces a
// user-actionable error (auth missing, repo not found, invalid args)
// distinct from a transport/protocol error — the MCP client renders
// the Text to the user and keeps going.
type ToolResult struct {
	Text    string
	IsError bool
}

// ResourceDef describes a static resource the MCPTools dispatcher
// exposes. Currently used for `flow://repos/<canonical-key>/note`
// entries so MCP clients can auto-attach the right RepoNote when the
// user opens the matching repo folder.
type ResourceDef struct {
	URI         string
	Name        string
	Description string
	MimeType    string
}

// ResourceContent is the body of a ReadResource response.
type ResourceContent struct {
	URI      string
	MimeType string
	Text     string
}

// ErrResourceNotFound is returned by MCPTools.ReadResource when the URI
// does not resolve to a known resource. The adapter layer maps this to
// a JSON-RPC error so the MCP client can distinguish "unknown URI"
// from "empty resource".
var ErrResourceNotFound = errors.New("flow: mcp resource not found")

// WorktimeStatusReader is the slice of WorktimeReader that
// flow_worktime_status needs. Defined as an interface here so MCPTools
// tests don't have to construct a full WorktimeReader (with
// TargetResolver, ConfigReader, DayOffStore, LegacyActiveStore).
// *WorktimeReader satisfies this.
type WorktimeStatusReader interface {
	Today() (domain.Day, error)
}

// repoNoteListLimit caps how many RepoNotes flow_search_notes /
// flow_list_repo_notes pull from the store in one shot. A user with
// more than 5k repo notes is well outside Phase 1's target audience;
// the next milestone (M6 WebUI) can revisit if needed.
const repoNoteListLimit = 5000

// loginRequiredMsg is the canonical message every tool returns when the
// MCP server boots without a valid token. The composition root sets
// Authed=false in that case; tools short-circuit before touching any
// state.
const loginRequiredMsg = "Login required: run `flow login` in a terminal first."

// MCPTools is the dispatch hub for MCP tool/resource calls. Shares the
// same use cases (RepoNotes, ActiveSessions, Sessions, WorktimeReader)
// the CLI/TUI binaries use, so MCP-driven work syncs identically.
//
// Authed=false → every tool returns a "Login required" message and the
// resource catalog is empty. The auth check happens once at boot in
// cmd/flow-mcp/main; tools don't re-check because the keyring
// roundtrip would dominate latency on a per-call basis. A token that
// expires mid-session manifests as a sync-worker push failure, which
// the next `flow status` surfaces — by design we don't try to gate
// every tool call on liveness.
type MCPTools struct {
	UserID string
	Pwd    string
	Authed bool

	Notes    *RepoNotes
	Active   *ActiveSessions
	Sessions *Sessions
	Reader   WorktimeStatusReader

	// RepoNoteStore is the raw store used by flow_search_notes /
	// flow_list_repo_notes — those tools need bulk pull-since access
	// to the RepoNote rows without going through Notes.Save / Get.
	RepoNoteStore ports.RepoNoteStore

	// ProjectStore is used by flow_worktime_status to name the
	// active project(s) in the status text.
	ProjectStore ports.ProjectStore
}

// Catalog returns the static tool catalog. Eight is a placeholder — we
// ship seven tools in M5; the two Kompendium tools (flow_get_note /
// flow_save_note) are deferred to a follow-up plan because they need a
// new note-IO port that doesn't fit in M5's scope.
func (m *MCPTools) Catalog() []ToolDef {
	return []ToolDef{
		{
			Name:        "flow_get_repo_note",
			Description: "Return the RepoNote (CLAUDE.md-style guidance) for the given repo path. Defaults to the MCP-server PWD.",
			InputSchema: objectSchema(map[string]any{
				"repo_path": stringSchema("Absolute path to the repo. Defaults to the MCP-server PWD."),
			}, nil),
		},
		{
			Name:        "flow_save_repo_note",
			Description: "Write the RepoNote body for the given repo path. Overwrites the existing note. Syncs via the shared httpsync worker.",
			InputSchema: objectSchema(map[string]any{
				"content":   stringSchema("New RepoNote body (Markdown)."),
				"repo_path": stringSchema("Absolute path to the repo. Defaults to the MCP-server PWD."),
			}, []string{"content"}),
		},
		{
			Name:        "flow_list_repo_notes",
			Description: "List every Repo known to flow with the size of its RepoNote (in bytes). Useful for discovery before flow_get_repo_note.",
			InputSchema: objectSchema(nil, nil),
		},
		{
			Name:        "flow_search_notes",
			Description: "Substring search across every RepoNote body for the current user. Returns the top matches with surrounding context.",
			InputSchema: objectSchema(map[string]any{
				"query": stringSchema("Substring to look for. Case-insensitive."),
				"limit": numberSchema("Maximum number of matches to return. Defaults to 10."),
			}, []string{"query"}),
		},
		{
			Name:        "flow_worktime_status",
			Description: "Current worktime state: active sessions, today's logged time, today's target. Read-only.",
			InputSchema: objectSchema(nil, nil),
		},
		{
			Name:        "flow_start_session",
			Description: "Start a worktime session for a project. Project resolution: explicit `project` (UUID or slug) → PWD-based slug → MRU → 'Allgemein'.",
			InputSchema: objectSchema(map[string]any{
				"project": stringSchema("Project UUID or slug. Optional."),
				"tag":     stringSchema("Optional tag carried through to the finished Session."),
				"note":    stringSchema("Optional note carried through to the finished Session."),
			}, nil),
		},
		{
			Name:        "flow_stop_session",
			Description: "Stop the active worktime session for the given project. Same project resolution as flow_start_session.",
			InputSchema: objectSchema(map[string]any{
				"project": stringSchema("Project UUID or slug. Optional."),
				"tag":     stringSchema("Override the start-time tag on the finished Session."),
				"note":    stringSchema("Override the start-time note on the finished Session."),
			}, nil),
		},
	}
}

// Call dispatches a tool by name. Unknown tool names return an
// IsError=true result rather than a transport error so the MCP client
// can render the message and continue.
func (m *MCPTools) Call(name string, args map[string]any) ToolResult {
	if !m.Authed {
		return errResult(loginRequiredMsg)
	}
	switch name {
	case "flow_get_repo_note":
		return m.callGetRepoNote(args)
	case "flow_save_repo_note":
		return m.callSaveRepoNote(args)
	case "flow_list_repo_notes":
		return m.callListRepoNotes()
	case "flow_search_notes":
		return m.callSearchNotes(args)
	case "flow_worktime_status":
		return m.callWorktimeStatus()
	case "flow_start_session":
		return m.callStartSession(args)
	case "flow_stop_session":
		return m.callStopSession(args)
	default:
		return errResult("unknown tool: " + name)
	}
}

// ResourceCatalog returns one entry per known Repo, encoded as
// flow://repos/<url-encoded-canonical-key>/note. MCP clients can
// auto-attach the matching RepoNote when the user opens that repo.
func (m *MCPTools) ResourceCatalog() []ResourceDef {
	if !m.Authed {
		return nil
	}
	repos, err := m.Notes.ListRepos(m.UserID)
	if err != nil {
		return nil
	}
	out := make([]ResourceDef, 0, len(repos))
	for _, r := range repos {
		out = append(out, ResourceDef{
			URI:         "flow://repos/" + url.PathEscape(r.CanonicalKey) + "/note",
			Name:        r.DisplayName,
			Description: "RepoNote for " + r.CanonicalKey,
			MimeType:    "text/markdown",
		})
	}
	return out
}

// ReadResource resolves a flow://repos/<key>/note URI to the matching
// RepoNote body. Returns ErrResourceNotFound for unknown URIs.
func (m *MCPTools) ReadResource(uri string) (ResourceContent, error) {
	if !m.Authed {
		return ResourceContent{}, ErrResourceNotFound
	}
	key, ok := parseRepoNoteURI(uri)
	if !ok {
		return ResourceContent{}, ErrResourceNotFound
	}
	repos, err := m.Notes.ListRepos(m.UserID)
	if err != nil {
		return ResourceContent{}, err
	}
	var match *domain.Repo
	for i := range repos {
		if repos[i].CanonicalKey == key {
			match = &repos[i]
			break
		}
	}
	if match == nil {
		return ResourceContent{}, ErrResourceNotFound
	}
	note, err := m.RepoNoteStore.GetByRepo(m.UserID, match.ID)
	if errors.Is(err, ports.ErrRepoNoteNotFound) {
		return ResourceContent{URI: uri, MimeType: "text/markdown", Text: ""}, nil
	}
	if err != nil {
		return ResourceContent{}, err
	}
	return ResourceContent{URI: uri, MimeType: "text/markdown", Text: note.Content}, nil
}

// ---- Tool implementations ----

func (m *MCPTools) callGetRepoNote(args map[string]any) ToolResult {
	pwd := stringArg(args, "repo_path")
	if pwd == "" {
		pwd = m.Pwd
	}
	if pwd == "" {
		return errResult("flow_get_repo_note: PWD is empty and no repo_path was provided")
	}
	note, repo, err := m.Notes.GetForPwd(m.UserID, pwd)
	if err != nil {
		return errResult("flow_get_repo_note: " + err.Error())
	}
	if note.ID == "" {
		return ToolResult{Text: fmt.Sprintf("repo=%s display=%q\n(no RepoNote yet — use flow_save_repo_note to create one)",
			repo.CanonicalKey, repo.DisplayName)}
	}
	return ToolResult{Text: fmt.Sprintf("repo=%s display=%q updated=%s bytes=%d\n%s",
		repo.CanonicalKey, repo.DisplayName,
		note.UpdatedAt.Format(time.RFC3339), len(note.Content),
		note.Content)}
}

func (m *MCPTools) callSaveRepoNote(args map[string]any) ToolResult {
	content, ok := args["content"].(string)
	if !ok {
		return errResult("flow_save_repo_note: 'content' is required and must be a string")
	}
	pwd := stringArg(args, "repo_path")
	if pwd == "" {
		pwd = m.Pwd
	}
	if pwd == "" {
		return errResult("flow_save_repo_note: PWD is empty and no repo_path was provided")
	}
	saved, err := m.Notes.Save(m.UserID, pwd, content)
	if err != nil {
		return errResult("flow_save_repo_note: " + err.Error())
	}
	return ToolResult{Text: fmt.Sprintf("saved bytes=%d updated=%s",
		len(saved.Content), saved.UpdatedAt.Format(time.RFC3339))}
}

func (m *MCPTools) callListRepoNotes() ToolResult {
	repos, err := m.Notes.ListRepos(m.UserID)
	if err != nil {
		return errResult("flow_list_repo_notes: " + err.Error())
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d repo(s)\n", len(repos))
	for _, r := range repos {
		note, err := m.RepoNoteStore.GetByRepo(m.UserID, r.ID)
		bytes := 0
		updated := "never"
		switch {
		case errors.Is(err, ports.ErrRepoNoteNotFound):
			// leave defaults
		case err != nil:
			fmt.Fprintf(&b, "- %s (%s) — error: %v\n", r.DisplayName, r.CanonicalKey, err)
			continue
		default:
			bytes = len(note.Content)
			updated = note.UpdatedAt.Format(time.RFC3339)
		}
		fmt.Fprintf(&b, "- %s (%s) — bytes=%d updated=%s\n", r.DisplayName, r.CanonicalKey, bytes, updated)
	}
	return ToolResult{Text: strings.TrimRight(b.String(), "\n")}
}

func (m *MCPTools) callSearchNotes(args map[string]any) ToolResult {
	query, _ := args["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return errResult("flow_search_notes: 'query' is required")
	}
	limit := intArg(args, "limit", 10)
	if limit <= 0 {
		limit = 10
	}
	notes, _, _, err := m.RepoNoteStore.PullSince(m.UserID, 0, repoNoteListLimit)
	if err != nil {
		return errResult("flow_search_notes: " + err.Error())
	}
	repos, err := m.Notes.ListRepos(m.UserID)
	if err != nil {
		return errResult("flow_search_notes: " + err.Error())
	}
	byID := make(map[string]domain.Repo, len(repos))
	for _, r := range repos {
		byID[r.ID] = r
	}
	q := strings.ToLower(query)
	var hits int
	var b strings.Builder
	for _, n := range notes {
		body := strings.ToLower(n.Content)
		idx := strings.Index(body, q)
		if idx < 0 {
			continue
		}
		hits++
		if hits > limit {
			break
		}
		r := byID[n.RepoID]
		snippet := contextSnippet(n.Content, idx, len(query))
		fmt.Fprintf(&b, "- %s (%s) — %q\n", r.DisplayName, r.CanonicalKey, snippet)
	}
	if hits == 0 {
		return ToolResult{Text: "no matches"}
	}
	return ToolResult{Text: fmt.Sprintf("%d match(es)\n%s", hits, strings.TrimRight(b.String(), "\n"))}
}

func (m *MCPTools) callWorktimeStatus() ToolResult {
	day, err := m.Reader.Today()
	if err != nil {
		return errResult("flow_worktime_status: " + err.Error())
	}
	active, err := m.Active.ListActive(m.UserID)
	if err != nil {
		return errResult("flow_worktime_status: " + err.Error())
	}
	var b strings.Builder
	fmt.Fprintf(&b, "today logged=%s target=%s sessions=%d\n",
		day.Logged.Round(time.Minute), day.Target.Round(time.Minute), len(day.Sessions))
	if len(active) == 0 {
		fmt.Fprintln(&b, "no active session")
	} else {
		fmt.Fprintf(&b, "%d active session(s):\n", len(active))
		for _, a := range active {
			projName := a.ProjectID
			if p, err := m.ProjectStore.GetByID(m.UserID, a.ProjectID); err == nil {
				projName = p.Name + " (" + p.Slug + ")"
			}
			elapsed := time.Since(a.StartedAt).Round(time.Minute)
			fmt.Fprintf(&b, "- %s — started %s on %s (elapsed %s, tag=%q)\n",
				projName, a.StartedAt.Format(time.RFC3339),
				a.StartedOnDevice, elapsed, a.Tag)
		}
	}
	return ToolResult{Text: strings.TrimRight(b.String(), "\n")}
}

func (m *MCPTools) callStartSession(args map[string]any) ToolResult {
	explicit := stringArg(args, "project")
	tag := stringArg(args, "tag")
	note := stringArg(args, "note")
	proj, err := m.Sessions.ResolveProject(m.UserID, explicit, m.Pwd)
	if err != nil {
		return errResult("flow_start_session: project resolve: " + err.Error())
	}
	row, err := m.Active.Start(m.UserID, proj.ID, tag, note)
	if errors.Is(err, ErrActiveSessionExists) {
		return errResult(fmt.Sprintf("flow_start_session: active session already running for %s — stop it first", proj.Name))
	}
	if err != nil {
		return errResult("flow_start_session: " + err.Error())
	}
	return ToolResult{Text: fmt.Sprintf("started project=%s slug=%s at=%s tag=%q",
		proj.Name, proj.Slug, row.StartedAt.Format(time.RFC3339), row.Tag)}
}

func (m *MCPTools) callStopSession(args map[string]any) ToolResult {
	explicit := stringArg(args, "project")
	tag := stringArg(args, "tag")
	note := stringArg(args, "note")
	proj, err := m.Sessions.ResolveProject(m.UserID, explicit, m.Pwd)
	if err != nil {
		return errResult("flow_stop_session: project resolve: " + err.Error())
	}
	sess, err := m.Active.Stop(m.UserID, proj.ID, tag, note)
	if errors.Is(err, ports.ErrActiveSessionNotFound) {
		return errResult(fmt.Sprintf("flow_stop_session: no active session for %s", proj.Name))
	}
	if err != nil {
		return errResult("flow_stop_session: " + err.Error())
	}
	return ToolResult{Text: fmt.Sprintf("stopped project=%s elapsed=%s tag=%q",
		proj.Name, sess.Elapsed.Round(time.Minute), sess.Tag)}
}

// ---- helpers ----

func errResult(msg string) ToolResult {
	return ToolResult{Text: msg, IsError: true}
}

func stringArg(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}

// intArg reads a numeric arg. JSON numbers decode to float64 by default
// (encoding/json), so accept both shapes.
func intArg(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return def
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	out := map[string]any{"type": "object"}
	if properties != nil {
		out["properties"] = properties
	} else {
		out["properties"] = map[string]any{}
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func stringSchema(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func numberSchema(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
}

// contextSnippet returns up to ~80 chars around the match site.
func contextSnippet(s string, idx, qlen int) string {
	const window = 40
	start := idx - window
	if start < 0 {
		start = 0
	}
	end := idx + qlen + window
	if end > len(s) {
		end = len(s)
	}
	snip := s[start:end]
	snip = strings.ReplaceAll(snip, "\n", " ")
	snip = strings.ReplaceAll(snip, "\r", " ")
	return snip
}

// parseRepoNoteURI extracts the URL-escaped canonical key from a
// flow://repos/<key>/note URI. Returns ok=false for any other shape.
func parseRepoNoteURI(uri string) (string, bool) {
	const prefix = "flow://repos/"
	const suffix = "/note"
	if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, suffix) {
		return "", false
	}
	escaped := uri[len(prefix) : len(uri)-len(suffix)]
	if escaped == "" {
		return "", false
	}
	key, err := url.PathUnescape(escaped)
	if err != nil {
		return "", false
	}
	return key, true
}
