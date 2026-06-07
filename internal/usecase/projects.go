package usecase

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Projects orchestrates Worktime-Project operations: list (driving the
// TUI picker), create (incl. inline-create from the picker), rename,
// archive. Calls TouchLastUsed when a Session starts (called via
// usecase.ActiveSessions.Start, Task 12).
type Projects struct {
	users      ports.UserStore
	projects   ports.ProjectStore
	queue      ports.WriteQueue // optional; nil disables server-push enqueue
	pushSignal func()           // optional; wakes the sync worker after enqueue
}

// NewProjects constructs a Projects use case. queue may be nil for callers that
// don't sync (tests, offline tools); when set, Create/Rename/Archive enqueue a
// "projects" push so the row reaches the server.
func NewProjects(users ports.UserStore, projects ports.ProjectStore, queue ports.WriteQueue) *Projects {
	return &Projects{users: users, projects: projects, queue: queue}
}

// SetPushSignal attaches a callback fired after each push enqueue so the sync
// worker drains immediately rather than waiting for its next tick. Mirrors
// ActiveSessions/RepoNotes. nil-tolerant.
func (p *Projects) SetPushSignal(fn func()) { p.pushSignal = fn }

func (p *Projects) signalPush() {
	if p.pushSignal != nil {
		p.pushSignal()
	}
}

// enqueueProject queues a "projects" push for the sync worker. expectedVersion
// is the version the server is expected to currently hold (0 for a brand-new
// project — the server rejects a non-zero expected version on insert). Without
// this, a locally-created project never reaches the server and every session or
// active_session that references it fails its push with a FOREIGN KEY 500.
// nil-queue tolerant; enqueue failures are swallowed (the row is already
// persisted locally and a later mutation or backfill will re-enqueue).
func (p *Projects) enqueueProject(pr domain.Project, expectedVersion int64) {
	if p.queue == nil {
		return
	}
	payload, err := json.Marshal(pr)
	if err != nil {
		return
	}
	if _, err := p.queue.Enqueue("projects", pr.ID, payload, expectedVersion); err != nil {
		return
	}
	p.signalPush()
}

// ListActive returns active Projects MRU-first, used by the TUI picker.
func (p *Projects) ListActive(userID string) ([]domain.Project, error) {
	return p.projects.ListActive(userID)
}

// ListAll returns all Projects including archived ones, used by `flow projects list --archived`.
func (p *Projects) ListAll(userID string) ([]domain.Project, error) {
	return p.projects.ListAll(userID)
}

// Create creates a new Project with auto-generated slug.
//
// Slug rules: lowercase ASCII, spaces → "-", non-[a-z0-9-] stripped,
// collapsed dashes. If the slug collides with an existing one for this
// User, suffix "-2", "-3", ... until unique.
func (p *Projects) Create(userID, name string) (domain.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Project{}, errors.New("project name required")
	}
	base := SlugFromName(name)
	slug := base
	i := 2
	for {
		_, err := p.projects.GetBySlug(userID, slug)
		if errors.Is(err, ports.ErrProjectNotFound) {
			break
		}
		if err != nil {
			return domain.Project{}, err
		}
		slug = base + "-" + strconv.Itoa(i)
		i++
	}
	pr, err := p.projects.EnsureBySlug(userID, name, slug)
	if err != nil {
		return domain.Project{}, err
	}
	// Brand-new project: the server has no row yet, so the expected version is
	// 0. Enqueuing here is what lets sessions/active_sessions on this project
	// sync at all (they carry a server-side FK to projects).
	p.enqueueProject(pr, pr.Version)
	return pr, nil
}

// Rename changes the human-readable name only — slug stays stable.
func (p *Projects) Rename(userID, id, newName string) error {
	pr, err := p.projects.GetByID(userID, id)
	if err != nil {
		return err
	}
	expectedVersion := pr.Version // server's current version, before the local bump
	pr.Name = strings.TrimSpace(newName)
	pr.Version++ // local optimistic bump; server may overwrite
	if err := p.projects.Upsert(pr); err != nil {
		return err
	}
	p.enqueueProject(pr, expectedVersion)
	return nil
}

// Archive soft-deletes a Project and syncs the archive to the server.
func (p *Projects) Archive(userID, id string) error {
	pr, err := p.projects.GetByID(userID, id)
	if err != nil {
		return err
	}
	expectedVersion := pr.Version
	if err := p.projects.Archive(userID, id); err != nil {
		return err
	}
	now := time.Now().UTC()
	pr.ArchivedAt = &now
	p.enqueueProject(pr, expectedVersion)
	return nil
}

// BackfillUnsynced enqueues a "projects" push for every locally-stored project
// that has never reached the server (Version == 0). It exists for projects
// created before project-sync was wired: without it those projects — and every
// session/active_session that references them — can never sync. The caller MUST
// guard this to run exactly once (a duplicate push of an already-created
// project hits a version conflict and halts the queue). Returns how many were
// enqueued.
func (p *Projects) BackfillUnsynced(userID string) (int, error) {
	if p.queue == nil {
		return 0, nil
	}
	all, err := p.projects.ListAll(userID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, pr := range all {
		if pr.Version != 0 {
			continue
		}
		payload, err := json.Marshal(pr)
		if err != nil {
			continue
		}
		if _, err := p.queue.Enqueue("projects", pr.ID, payload, 0); err != nil {
			continue
		}
		n++
	}
	return n, nil
}

// MarkUsedNow updates LastUsedAt — called from active_sessions.Start.
func (p *Projects) MarkUsedNow(userID, id string) error {
	return p.projects.TouchLastUsed(userID, id)
}

// SlugFromName is the canonical slug-generation. Exposed so the picker
// can preview "the slug we'd assign" for inline-create.
//
// Rules: lowercase ASCII only; spaces, hyphens and underscores become
// a single "-"; all other characters are stripped; leading/trailing
// dashes removed. Returns "unnamed" for inputs that reduce to empty.
func SlugFromName(name string) string {
	var sb strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			sb.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_':
			if !prevDash && sb.Len() > 0 {
				sb.WriteRune('-')
				prevDash = true
			}
		}
	}
	s := strings.TrimRight(sb.String(), "-")
	if s == "" {
		s = "unnamed"
	}
	return s
}
