package usecase

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// ActiveSessions orchestrates Start/Stop with optimistic-concurrency against
// the local ActiveSessionStore. Push to server happens via the httpsync
// write_queue (the queue's payload is the new ActiveSession row JSON; sync
// worker handles the 409 race).
type ActiveSessions struct {
	users      ports.UserStore // reserved for future callers (Task 32 wiring)
	projects   ports.ProjectStore
	active     ports.ActiveSessionStore
	sessions   ports.SessionStore
	queue      ports.WriteQueue
	device     string // hostname; informational in ActiveSession.StartedOnDevice
	pushSignal func() // optional; called after Enqueue to wake the worker immediately
}

// NewActiveSessions constructs an ActiveSessions use case. users may be nil
// until Task 32 wires the composition root; it is stored but not yet called.
func NewActiveSessions(
	users ports.UserStore,
	projects ports.ProjectStore,
	active ports.ActiveSessionStore,
	sessions ports.SessionStore,
	queue ports.WriteQueue,
) *ActiveSessions {
	host, _ := os.Hostname()
	return &ActiveSessions{
		users:    users,
		projects: projects,
		active:   active,
		sessions: sessions,
		queue:    queue,
		device:   host,
	}
}

// SetPushSignal attaches a callback that is invoked after each Enqueue call so
// the sync worker can drain the queue immediately rather than waiting for its
// next poll tick. Called by the composition root after the worker is started.
// nil-tolerant: if fn is nil the method is a no-op.
func (a *ActiveSessions) SetPushSignal(fn func()) {
	a.pushSignal = fn
}

// signalPush calls the push-signal callback if one is set.
func (a *ActiveSessions) signalPush() {
	if a.pushSignal != nil {
		a.pushSignal()
	}
}

// Start records an ActiveSession locally and queues a server-start POST.
// In option-2 mode, parallel ActiveSessions across different Projects are
// allowed; this method assumes the caller (CLI or TUI picker) already
// resolved exactly one ProjectID.
//
// tag and note are stored on the local ActiveSession row and forwarded to
// the server so Stop (possibly from another device) can carry them over to
// the finished Session even when no flags are passed at stop time.
//
// If an ActiveSession for (userID, projectID) already exists locally,
// returns ErrActiveSessionExists — caller shows the conflict overlay.
func (a *ActiveSessions) Start(userID, projectID, tag, note string) (domain.ActiveSession, error) {
	if _, err := a.active.Get(userID, projectID); err == nil {
		return domain.ActiveSession{}, ErrActiveSessionExists
	} else if !errors.Is(err, ports.ErrActiveSessionNotFound) {
		return domain.ActiveSession{}, err
	}

	row := domain.ActiveSession{
		UserID:          userID,
		ProjectID:       projectID,
		StartedAt:       time.Now().UTC(),
		StartedOnDevice: a.device,
		Tag:             tag,
		Note:            note,
		Version:         0, // server assigns
	}
	if err := a.active.Upsert(row); err != nil {
		return domain.ActiveSession{}, err
	}
	if err := a.projects.TouchLastUsed(userID, projectID); err != nil {
		return domain.ActiveSession{}, err
	}
	payload, err := encodeActiveStart(row)
	if err != nil {
		return domain.ActiveSession{}, err
	}
	if _, err := a.queue.Enqueue("active_sessions", projectID, payload, 0); err != nil {
		return domain.ActiveSession{}, err
	}
	a.signalPush()
	return row, nil
}

// Stop closes the ActiveSession, creates a finished Session row locally, and
// queues a DELETE to the server. The server's atomic Stop-transaction will
// create the canonical Session row server-side; the next pull reconciles
// (local row gets replaced with server-version row).
//
// The two queue.Enqueue calls at the end use fire-and-forget error handling
// (_, _ = ...) by design: a transient queue write failure must not block
// the local Stop, which has already committed the finished Session row and
// removed the ActiveSession row. The sync worker retries on restart.
func (a *ActiveSessions) Stop(userID, projectID, tag, note string) (domain.Session, error) {
	cur, err := a.active.Get(userID, projectID)
	if err != nil {
		return domain.Session{}, err
	}

	// Empty caller args inherit from the start-time row: `flow worktime start
	// --tag deep` followed by `flow worktime stop` carries "deep" through.
	if tag == "" {
		tag = cur.Tag
	}
	if note == "" {
		note = cur.Note
	}

	now := time.Now().UTC()
	sess := domain.Session{
		ID:        newUUID(),
		UserID:    userID,
		ProjectID: projectID,
		Date:      cur.StartedAt.Truncate(24 * time.Hour),
		Start:     cur.StartedAt,
		Stop:      now,
		Elapsed:   now.Sub(cur.StartedAt),
		Tag:       tag,
		Note:      note,
		Version:   0,
		UpdatedAt: now,
	}
	if err := a.sessions.Upsert(sess); err != nil {
		return domain.Session{}, err
	}
	if err := a.active.Delete(userID, projectID); err != nil {
		return domain.Session{}, err
	}

	// Queue session push. Error is intentionally ignored: Stop is already
	// committed locally; queue failure is recoverable on next sync.
	if payload, encErr := encodeSession(sess); encErr == nil && payload != nil {
		_, _ = a.queue.Enqueue("sessions", sess.ID, payload, 0)
	}

	// Queue active-stop signal with the known server version for If-Match.
	stopPayload := []byte(`{"action":"stop","version":` + strconv.FormatInt(cur.Version, 10) + `}`)
	_, _ = a.queue.Enqueue("active_sessions_stop", projectID, stopPayload, cur.Version)
	a.signalPush()

	return sess, nil
}

// ListActive returns currently running sessions across all projects for the
// given user.
func (a *ActiveSessions) ListActive(userID string) ([]domain.ActiveSession, error) {
	return a.active.ListByUser(userID)
}

// ForceTakeover is what the conflict-overlay calls on `[t]` press — it
// requeues the start with the known server version (If-Match semantics)
// instead of 0, so the server knows we are intentionally overwriting the
// concurrent session on another device.
func (a *ActiveSessions) ForceTakeover(userID, projectID string, currentServerVersion int64) error {
	row := domain.ActiveSession{
		UserID:          userID,
		ProjectID:       projectID,
		StartedAt:       time.Now().UTC(),
		StartedOnDevice: a.device,
	}
	if err := a.active.Upsert(row); err != nil {
		return err
	}
	payload, err := encodeActiveStart(row)
	if err != nil {
		return err
	}
	_, err = a.queue.Enqueue("active_sessions", projectID, payload, currentServerVersion)
	return err
}

// newUUID returns a random UUID v4 string using crypto/rand. The usecase layer
// may not import github.com/google/uuid (depguard strict mode); this minimal
// implementation covers the 8-4-4-4-12 hex format with the version/variant
// bits set per RFC 4122 §4.4.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// ErrActiveSessionExists is returned by Start when a local ActiveSession row
// already exists for the given (userID, projectID) pair. The caller (TUI or
// CLI) should surface the conflict overlay so the user can decide whether to
// take over or leave the existing session running.
var ErrActiveSessionExists = errors.New("flow: active session for this project already exists")

// encodeActiveStart produces the queue payload for an active-session start.
// The shape matches httpsync.Worker's activeStartBody (snake_case JSON);
// json.Marshal-ing domain.ActiveSession directly would yield PascalCase keys
// the worker silently ignores.
func encodeActiveStart(row domain.ActiveSession) ([]byte, error) {
	return json.Marshal(struct {
		Action          string `json:"action"`
		ProjectID       string `json:"project_id"`
		StartedOnDevice string `json:"started_on_device"`
		Tag             string `json:"tag"`
		Note            string `json:"note"`
	}{"start", row.ProjectID, row.StartedOnDevice, row.Tag, row.Note})
}

// encodeSession marshals a domain.Session to JSON for the write queue.
func encodeSession(s domain.Session) ([]byte, error) {
	return json.Marshal(s)
}
