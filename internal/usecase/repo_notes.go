package usecase

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// RepoNotes orchestrates RepoNote read/write. Save resolves the working
// directory to a CanonicalKey, get-or-creates the Repo row, then upserts the
// note locally + enqueues a push for the sync worker.
type RepoNotes struct {
	repos      ports.RepoStore
	notes      ports.RepoNoteStore
	queue      ports.WriteQueue
	resolver   RemoteResolver
	pushSignal func()
}

// NewRepoNotes constructs the use case. resolver may be nil — in that case
// every CanonicalKey ends up as a path: hash, which still works on a single
// device.
func NewRepoNotes(repos ports.RepoStore, notes ports.RepoNoteStore,
	queue ports.WriteQueue, resolver RemoteResolver,
) *RepoNotes {
	return &RepoNotes{repos: repos, notes: notes, queue: queue, resolver: resolver}
}

// SetPushSignal attaches a callback fired after each push enqueue so the
// background sync worker can drain immediately instead of waiting for its
// next tick. Mirrors ActiveSessions.SetPushSignal.
func (u *RepoNotes) SetPushSignal(fn func()) { u.pushSignal = fn }

func (u *RepoNotes) signalPush() {
	if u.pushSignal != nil {
		u.pushSignal()
	}
}

// GetForPwd resolves pwd to a Repo + RepoNote.
//
// Behaviour:
//   - Resolves pwd → CanonicalKey via the resolver.
//   - Ensures a Repo row exists for (userID, canonicalKey); returns it.
//   - Returns the RepoNote for that Repo, or a zero-value RepoNote and nil
//     error when no note exists yet. Callers distinguish "no note" via the
//     RepoNote.ID == "" check.
func (u *RepoNotes) GetForPwd(userID, pwd string) (domain.RepoNote, domain.Repo, error) {
	key, err := CanonicalKey(pwd, u.resolver)
	if err != nil {
		return domain.RepoNote{}, domain.Repo{}, err
	}
	display := filepath.Base(pwd)
	if display == "." || display == "/" {
		display = key
	}
	repo, err := u.repos.EnsureByCanonicalKey(userID, key, display)
	if err != nil {
		return domain.RepoNote{}, domain.Repo{}, fmt.Errorf("ensure repo: %w", err)
	}
	note, err := u.notes.GetByRepo(userID, repo.ID)
	if errors.Is(err, ports.ErrRepoNoteNotFound) {
		return domain.RepoNote{}, repo, nil
	}
	if err != nil {
		return domain.RepoNote{}, repo, fmt.Errorf("get note: %w", err)
	}
	return note, repo, nil
}

// Save writes content for the repo resolved from pwd. Preserves the
// existing RepoNote.ID across updates so the same row syncs forward
// instead of producing a new row per save.
func (u *RepoNotes) Save(userID, pwd, content string) (domain.RepoNote, error) {
	existing, repo, err := u.GetForPwd(userID, pwd)
	if err != nil {
		return domain.RepoNote{}, err
	}
	n := domain.RepoNote{
		ID:        existing.ID,
		RepoID:    repo.ID,
		UserID:    userID,
		Content:   content,
		Version:   existing.Version,
		UpdatedAt: time.Now().UTC(),
	}
	if n.ID == "" {
		n.ID = newUUID()
	}
	if err := u.notes.Upsert(n); err != nil {
		return domain.RepoNote{}, fmt.Errorf("local upsert: %w", err)
	}
	if u.queue != nil {
		payload, encErr := json.Marshal(n)
		if encErr == nil {
			_, _ = u.queue.Enqueue("repo_notes", n.ID, payload, n.Version)
			u.signalPush()
		}
	}
	return n, nil
}

// ListRepos returns every known Repo for the user, ordered by Version ASC.
// Used by the MCP resource catalog and the WebUI `/repos` index.
func (u *RepoNotes) ListRepos(userID string) ([]domain.Repo, error) {
	const pageSize = 1000
	var all []domain.Repo
	var since int64
	for {
		page, high, hasMore, err := u.repos.PullSince(userID, since, pageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if !hasMore || high == since {
			break
		}
		since = high
	}
	return all, nil
}
