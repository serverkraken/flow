package domain

import (
	"fmt"
	"regexp"
	"time"
)

// DocumentType is the kind of compendium note.
type DocumentType string

const (
	DocDaily         DocumentType = "daily"
	DocProject       DocumentType = "project"
	DocFree          DocumentType = "free"
	DocAgent         DocumentType = "agent"         // DEPRECATED (B3d): split into DocSpec/DocPlan; kept valid until prod 0-agent
	DocMemory        DocumentType = "memory"        // agent-owned
	DocInstruction   DocumentType = "instruction"   // agent-owned (CLAUDE.md)
	DocSkill         DocumentType = "skill"         // agent-owned
	DocPlan          DocumentType = "plan"          // agent-owned
	DocSpec          DocumentType = "spec"          // agent-owned (B3d: was agent)
	DocActiveContext DocumentType = "activecontext" // agent-owned (B3d: per-repo active context)
)

// DocumentTypes returns every valid document type in canonical order. It is the
// single source of truth for the type set; valid() and external validators
// (flow-mcp's type filter) both derive from it, so a new type is added here once.
func DocumentTypes() []DocumentType {
	return []DocumentType{
		DocDaily, DocProject, DocFree, DocAgent,
		DocMemory, DocInstruction, DocSkill, DocPlan,
		DocSpec, DocActiveContext,
	}
}

// HumanOwned reports whether documents of this type are authored by the human
// (daily / project / free notes) rather than the agent. It drives flow-mcp's
// write guard: mutating a human-owned document needs explicit confirmation.
// Expressed as a positive set so any future (agent) type is unguarded by default.
func (t DocumentType) HumanOwned() bool {
	switch t {
	case DocDaily, DocProject, DocFree:
		return true
	default:
		return false
	}
}

// ContextEligible reports whether documents of this type participate in the
// curated agent context. Other types can still be searched and archived, but
// assigning auto/immer/nie to them would have no semantic effect.
func (t DocumentType) ContextEligible() bool {
	switch t {
	case DocMemory, DocInstruction, DocActiveContext:
		return true
	default:
		return false
	}
}

func (t DocumentType) valid() bool {
	for _, v := range DocumentTypes() {
		if t == v {
			return true
		}
	}
	return false
}

// ContextMode is a document's agent-context membership mode: auto (type-driven,
// the pre-L5.5 behavior), immer (highest-priority context, bypasses tag-gate
// and pin but remains subject to the hard cap), or nie (never composed but
// fully visible in Wissen/search).
type ContextMode string

const (
	ContextModeAuto  ContextMode = "auto"
	ContextModeImmer ContextMode = "immer"
	ContextModeNie   ContextMode = "nie"
)

// OrAuto returns the mode, defaulting an empty (zero-value) mode to auto — the
// pgstore Create binding uses it so a Document built without an explicit mode
// never binds '' (which the CHECK constraint forbids).
func (m ContextMode) OrAuto() ContextMode {
	if m == "" {
		return ContextModeAuto
	}
	return m
}

// Valid reports whether m is one of the three known modes (used by the write
// use case to reject bad API input before it hits the CHECK constraint).
func (m ContextMode) Valid() bool {
	switch m {
	case ContextModeAuto, ContextModeImmer, ContextModeNie:
		return true
	default:
		return false
	}
}

// Document is a compendium note. Path is a human-readable slug, unique per
// owner(+project). Tags/Role/Extra are carried by the schema from M2a but
// exercised by later slices (M2c tags, M3 brief role, M2d search).
type Document struct {
	ID        string         `json:"id"`
	OwnerID   string         `json:"-"`
	NodeID *string        `json:"projectId,omitempty"`
	Type      DocumentType   `json:"type"`
	Path      string         `json:"path"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Tags      []string       `json:"tags,omitempty"`
	Date      *time.Time     `json:"date,omitempty"`
	Role      *string        `json:"role,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
	Pinned     bool       `json:"pinned"`
	Archived   bool       `json:"archived"`
	ArchivedAt *time.Time `json:"archivedAt,omitempty"`
	// Priority is the manual context-ranking priority (higher = ranked earlier
	// within the memory pool; default 0). Set by ReorderContextDocs. Create
	// binds it explicitly (zero-value 0 for new docs, since docCols is the
	// shared INSERT column list); UpsertByPath omits it (own column list → DB
	// default 0).
	Priority  int            `json:"priority"`
	// ContextMode is the per-document agent-context membership mode (auto/immer/nie;
	// default auto). Set by SetContextMode. Create binds it via OrAuto() (empty→'auto',
	// since the CHECK forbids ''); UpsertByPath omits it (own column list → DB default 'auto').
	ContextMode ContextMode `json:"contextMode"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	// UpdatedByKind/UpdatedByRef stamp who last wrote this document (actor.Kind
	// as string + actor.Actor.Ref). Both empty means unknown/pre-L3 (NULL in
	// storage) — the provenance line then renders without an actor. Set by the
	// write use cases (CreateDocument/UpdateDocument/UpsertDocumentByPath) from
	// actor.FromContext(ctx); SetPinned/SetArchived deliberately do not restamp
	// (pin/archive is not authorship).
	UpdatedByKind string `json:"updatedByKind,omitempty"`
	UpdatedByRef  string `json:"updatedByRef,omitempty"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:[-_][a-z0-9]+)*(?:/[a-z0-9]+(?:[-_][a-z0-9]+)*)*$`)

// SlugOK reports whether s is a valid hierarchical slug: lowercase
// alphanumeric segments joined by '/', words separated by single '-' or '_'. No
// leading/trailing/double slash, no spaces or uppercase.
func SlugOK(s string) bool {
	return s != "" && slugRe.MatchString(s)
}

// DailyPath is the canonical slug for a daily note on day d.
func DailyPath(d time.Time) string {
	return "daily/" + d.Format("2006-01-02")
}

// Validate checks the document invariants (type, project rule, slug form,
// daily date). It does not check ID/owner presence — the use case stamps those.
func (d Document) Validate() error {
	if !d.Type.valid() {
		return fmt.Errorf("%w: bad type %q", ErrInvalidDocument, d.Type)
	}
	if d.Type == DocProject && (d.NodeID == nil || *d.NodeID == "") {
		return fmt.Errorf("%w: project document needs a projectId", ErrInvalidDocument)
	}
	if d.Type == DocDaily && d.Date == nil {
		return fmt.Errorf("%w: daily document needs a date", ErrInvalidDocument)
	}
	if !SlugOK(d.Path) {
		return fmt.Errorf("%w: invalid path %q", ErrInvalidDocument, d.Path)
	}
	return nil
}
