// Package testutil provides in-memory fakes for the ports, for use in tests.
package testutil

import (
	"context"
	"crypto/sha256"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

type FakeClock struct{ T time.Time }

func (c FakeClock) Now() time.Time { return c.T }

type FakeIDGen struct {
	mu sync.Mutex
	n  int
}

func (g *FakeIDGen) NewID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return "id-" + itoa(g.n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

type FakeUserStore struct {
	mu        sync.Mutex
	bySub     map[string]domain.User
	getSubErr error
}

func NewFakeUserStore() *FakeUserStore { return &FakeUserStore{bySub: map[string]domain.User{}} }

func (s *FakeUserStore) UpsertBySub(_ context.Context, u domain.User) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bySub[u.OIDCSub] = u
	return u, nil
}

// SetGetBySubErr makes GetBySub fail with err regardless of what's stored —
// for simulating a transient store failure (e.g. a DB restart) as distinct
// from a genuinely absent row.
func (s *FakeUserStore) SetGetBySubErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getSubErr = err
}

func (s *FakeUserStore) GetBySub(_ context.Context, sub string) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getSubErr != nil {
		return domain.User{}, s.getSubErr
	}
	u, ok := s.bySub[sub]
	if !ok {
		return domain.User{}, ports.ErrUserNotFound
	}
	return u, nil
}

func (s *FakeUserStore) GetByID(_ context.Context, id string) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.bySub {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.User{}, ports.ErrUserNotFound
}

type FakeVerifier struct {
	ID  ports.Identity
	Err error
}

func (v FakeVerifier) Verify(context.Context, string) (ports.Identity, error) {
	return v.ID, v.Err
}

// FakeNodeStore is an in-memory ports.NodeStore.
type FakeNodeStore struct {
	mu        sync.Mutex
	m         map[string]domain.Node // keyed by id
	deleteErr error
}

func (s *FakeNodeStore) SetDeleteError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteErr = err
}

func NewFakeNodeStore() *FakeNodeStore {
	return &FakeNodeStore{m: map[string]domain.Node{}}
}

func (s *FakeNodeStore) Create(_ context.Context, p domain.Node) (domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.slugTaken(p.OwnerID, p.ParentID, p.Slug, p.ID) {
		return domain.Node{}, ports.ErrNodeSlugTaken
	}
	s.m[p.ID] = p
	return p, nil
}

// slugTaken mirrors pgstore's per-sibling uniqueness: a slug clashes only with
// another node of the same owner and the same parent (NULL parents form one set).
func (s *FakeNodeStore) slugTaken(owner string, parentID *string, slug, exceptID string) bool {
	for _, n := range s.m {
		if n.OwnerID != owner || n.ID == exceptID || n.Slug != slug {
			continue
		}
		if (n.ParentID == nil) == (parentID == nil) && (parentID == nil || *n.ParentID == *parentID) {
			return true
		}
	}
	return false
}

func (s *FakeNodeStore) List(_ context.Context, ownerID string) ([]domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Node
	for _, p := range s.m {
		if p.OwnerID == ownerID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *FakeNodeStore) Get(_ context.Context, ownerID, id string) (domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.m[id]
	if !ok || p.OwnerID != ownerID {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	return p, nil
}

func (s *FakeNodeStore) Update(_ context.Context, ownerID string, p domain.Node) (domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.m[p.ID]
	if !ok || existing.OwnerID != ownerID {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	// mirror pgstore: rate and parent_id are not mutated here
	existing.Name = p.Name
	existing.Slug = p.Slug
	existing.Color = p.Color
	existing.Glyph = p.Glyph
	existing.Description = p.Description
	existing.UpstreamGit = p.UpstreamGit
	existing.OriginSlug = p.OriginSlug
	existing.Status = p.Status
	existing.Extra = p.Extra
	existing.CountsTowardTarget = p.CountsTowardTarget
	existing.Icon = p.Icon
	existing.LogoRef = p.LogoRef
	existing.UpdatedAt = p.UpdatedAt
	s.m[p.ID] = existing
	return existing, nil
}

func (s *FakeNodeStore) SetRate(_ context.Context, ownerID, id string, rate *domain.Money) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.m[id]
	if !ok || p.OwnerID != ownerID {
		return ports.ErrNodeNotFound
	}
	p.Rate = rate
	s.m[id] = p
	return nil
}

func (s *FakeNodeStore) Delete(_ context.Context, ownerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return s.deleteErr
	}
	n, ok := s.m[id]
	if !ok || n.OwnerID != ownerID {
		return ports.ErrNodeNotFound
	}
	for _, other := range s.m {
		if other.ParentID != nil && *other.ParentID == id {
			return ports.ErrNodeHasChildren
		}
	}
	delete(s.m, id)
	return nil
}

func (s *FakeNodeStore) Children(_ context.Context, ownerID string, parentID *string) ([]domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Node
	for _, n := range s.m {
		if n.OwnerID != ownerID {
			continue
		}
		if parentID == nil {
			if n.ParentID == nil {
				out = append(out, n)
			}
		} else if n.ParentID != nil && *n.ParentID == *parentID {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *FakeNodeStore) Ancestors(_ context.Context, ownerID, nodeID string) ([]domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Node
	cur := nodeID
	seen := map[string]bool{}
	for !seen[cur] {
		seen[cur] = true
		n, ok := s.m[cur]
		if !ok || n.OwnerID != ownerID {
			break
		}
		out = append(out, n)
		if n.ParentID == nil {
			break
		}
		cur = *n.ParentID
	}
	return out, nil
}

func (s *FakeNodeStore) Reparent(_ context.Context, ownerID, id string, parentID *string) (domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.m[id]
	if !ok || n.OwnerID != ownerID {
		return domain.Node{}, ports.ErrNodeNotFound
	}
	seen := map[string]bool{}
	for cur := parentID; cur != nil; {
		if *cur == id {
			return domain.Node{}, ports.ErrNodeCycle
		}
		if seen[*cur] {
			break
		}
		seen[*cur] = true
		parent, ok := s.m[*cur]
		if !ok || parent.OwnerID != ownerID {
			return domain.Node{}, ports.ErrNodeNotFound
		}
		cur = parent.ParentID
	}
	if s.slugTaken(ownerID, parentID, n.Slug, id) {
		return domain.Node{}, ports.ErrNodeSlugTaken
	}
	n.ParentID = parentID
	s.m[id] = n
	return n, nil
}

// Subtree returns nodeID and all transitive descendants (BFS, children sorted by name).
func (s *FakeNodeStore) Subtree(_ context.Context, ownerID, nodeID string) ([]domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, ok := s.m[nodeID]
	if !ok || root.OwnerID != ownerID {
		return nil, nil
	}
	out := []domain.Node{root}
	frontier := []string{nodeID}
	seen := map[string]bool{nodeID: true}
	for len(frontier) > 0 {
		cur := frontier[0]
		frontier = frontier[1:]
		var kids []domain.Node
		for _, n := range s.m {
			if n.OwnerID == ownerID && n.ParentID != nil && *n.ParentID == cur {
				kids = append(kids, n)
			}
		}
		sort.Slice(kids, func(i, j int) bool { return kids[i].Name < kids[j].Name })
		for _, k := range kids {
			if seen[k.ID] {
				continue
			}
			seen[k.ID] = true
			out = append(out, k)
			frontier = append(frontier, k.ID)
		}
	}
	return out, nil
}

// FakeSessionStore is an in-memory ports.SessionStore enforcing one running
// session per owner, like the Postgres partial index.
type FakeSessionStore struct {
	mu sync.Mutex
	m  map[string]domain.WorkSession // keyed by id
}

func NewFakeSessionStore() *FakeSessionStore {
	return &FakeSessionStore{m: map[string]domain.WorkSession{}}
}

func (s *FakeSessionStore) Create(_ context.Context, ws domain.WorkSession) (domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fakeSessionCreate(s.m, ws)
}

func (s *FakeSessionStore) Running(_ context.Context, ownerID string) (domain.WorkSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.m {
		if e.OwnerID == ownerID && e.Stop == nil {
			return e, true, nil
		}
	}
	return domain.WorkSession{}, false, nil
}

func (s *FakeSessionStore) Get(_ context.Context, ownerID, id string) (domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok || e.OwnerID != ownerID {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	return e, nil
}

func (s *FakeSessionStore) Stop(_ context.Context, ownerID, id string, nodeID *string, stop time.Time) (domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fakeSessionStop(s.m, ownerID, id, nodeID, stop)
}

// Update mirrors pgstore: it overwrites project/note/start/stop but NOT Tags
// (tags persist via FakeTagStore / the taggings junction). The record's existing
// Tags are preserved so stats/round-trip tests that seed Tags keep them.
func (s *FakeSessionStore) Update(_ context.Context, ownerID, id string, nodeID *string, note string, start time.Time, stop *time.Time) (domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fakeSessionUpdate(s.m, ownerID, id, nodeID, note, start, stop)
}

func (s *FakeSessionStore) Delete(_ context.Context, ownerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fakeSessionDelete(s.m, ownerID, id)
}

// WithinTransaction mirrors pgstore's rollback semantics over a cloned map.
// The writer assumes the outer lock is held for the complete callback.
func (s *FakeSessionStore) WithinTransaction(ctx context.Context, fn func(ports.SessionWriter) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := cloneSessions(s.m)
	if err := fn(fakeSessionWriter{m: s.m}); err != nil {
		s.m = snapshot
		return err
	}
	return nil
}

type fakeSessionWriter struct{ m map[string]domain.WorkSession }

func (w fakeSessionWriter) Create(_ context.Context, ws domain.WorkSession) (domain.WorkSession, error) {
	return fakeSessionCreate(w.m, ws)
}

func (w fakeSessionWriter) Stop(_ context.Context, ownerID, id string, nodeID *string, stop time.Time) (domain.WorkSession, error) {
	return fakeSessionStop(w.m, ownerID, id, nodeID, stop)
}

func (w fakeSessionWriter) Update(_ context.Context, ownerID, id string, nodeID *string, note string, start time.Time, stop *time.Time) (domain.WorkSession, error) {
	return fakeSessionUpdate(w.m, ownerID, id, nodeID, note, start, stop)
}

func (w fakeSessionWriter) Delete(_ context.Context, ownerID, id string) error {
	return fakeSessionDelete(w.m, ownerID, id)
}

func (w fakeSessionWriter) SetTags(_ context.Context, ownerID, sessionID string, tags []string) ([]string, error) {
	e, ok := w.m[sessionID]
	if !ok || e.OwnerID != ownerID {
		return nil, ports.ErrSessionNotFound
	}
	e.Tags = domain.NormalizeTags(tags)
	w.m[sessionID] = e
	return append([]string(nil), e.Tags...), nil
}

func fakeSessionCreate(m map[string]domain.WorkSession, ws domain.WorkSession) (domain.WorkSession, error) {
	if _, exists := m[ws.ID]; exists {
		return domain.WorkSession{}, errors.New("fake: duplicate session id")
	}
	if ws.Stop == nil {
		for _, e := range m {
			if e.OwnerID == ws.OwnerID && e.Stop == nil {
				return domain.WorkSession{}, domain.ErrAlreadyRunning
			}
		}
	}
	ws.Tags = append([]string(nil), ws.Tags...)
	m[ws.ID] = ws
	return ws, nil
}

func fakeSessionStop(m map[string]domain.WorkSession, ownerID, id string, nodeID *string, stop time.Time) (domain.WorkSession, error) {
	e, ok := m[id]
	if !ok || e.OwnerID != ownerID || e.Stop != nil {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	e.Stop = &stop
	e.NodeID = nodeID
	m[id] = e
	return e, nil
}

func fakeSessionUpdate(m map[string]domain.WorkSession, ownerID, id string, nodeID *string, note string, start time.Time, stop *time.Time) (domain.WorkSession, error) {
	e, ok := m[id]
	if !ok || e.OwnerID != ownerID {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	e.NodeID, e.Note, e.Start = nodeID, note, start
	e.Stop = nil
	if stop != nil {
		t := *stop
		e.Stop = &t
	}
	m[id] = e
	return e, nil
}

func fakeSessionDelete(m map[string]domain.WorkSession, ownerID, id string) error {
	e, ok := m[id]
	if !ok || e.OwnerID != ownerID {
		return ports.ErrSessionNotFound
	}
	delete(m, id)
	return nil
}

func cloneSessions(in map[string]domain.WorkSession) map[string]domain.WorkSession {
	out := make(map[string]domain.WorkSession, len(in))
	for id, ws := range in {
		if ws.NodeID != nil {
			n := *ws.NodeID
			ws.NodeID = &n
		}
		if ws.Stop != nil {
			t := *ws.Stop
			ws.Stop = &t
		}
		ws.Tags = append([]string(nil), ws.Tags...)
		out[id] = ws
	}
	return out
}

func (s *FakeSessionStore) List(_ context.Context, ownerID string, since time.Time) ([]domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.WorkSession
	for _, e := range s.m {
		if e.OwnerID == ownerID && !e.Start.Before(since) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *FakeSessionStore) ListRange(_ context.Context, ownerID string, since, until time.Time) ([]domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.WorkSession
	for _, e := range s.m {
		if e.OwnerID == ownerID && !e.Start.Before(since) && e.Start.Before(until) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *FakeSessionStore) ListPage(_ context.Context, ownerID string, limit, offset int) ([]domain.WorkSession, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []domain.WorkSession
	for _, e := range s.m {
		if e.OwnerID == ownerID {
			all = append(all, e)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Start.After(all[j].Start) })
	total := len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (s *FakeSessionStore) TagTimes(_ context.Context, ownerID string, from, to time.Time) ([]domain.TagTime, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	totals := map[string]int{}
	for _, ws := range s.m {
		if ws.OwnerID != ownerID {
			continue
		}
		if !from.IsZero() && ws.Start.Before(from) {
			continue
		}
		if !to.IsZero() && !ws.Start.Before(to) {
			continue
		}
		stop := time.Now()
		if ws.Stop != nil {
			stop = *ws.Stop
		}
		mins := int(stop.Sub(ws.Start).Minutes())
		if mins < 0 {
			mins = 0
		}
		for _, tag := range ws.Tags {
			totals[tag] += mins
		}
	}
	out := make([]domain.TagTime, 0, len(totals))
	for tag, mins := range totals {
		out = append(out, domain.TagTime{Tag: tag, Minutes: mins})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Minutes != out[j].Minutes {
			return out[i].Minutes > out[j].Minutes
		}
		return out[i].Tag < out[j].Tag
	})
	return out, nil
}

// LastBookedByNode returns the newest stopped-and-booked session start per node,
// owner-scoped. Mirrors the pgstore query (running/unbooked excluded).
func (s *FakeSessionStore) LastBookedByNode(_ context.Context, ownerID string) (map[string]time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]time.Time{}
	for _, ws := range s.m {
		if ws.OwnerID != ownerID || ws.NodeID == nil || ws.Stop == nil {
			continue
		}
		if cur, ok := out[*ws.NodeID]; !ok || ws.Start.After(cur) {
			out[*ws.NodeID] = ws.Start
		}
	}
	return out, nil
}

// FakeDayOffStore is an in-memory ports.DayOffStore keyed by (owner, yyyy-mm-dd).
type FakeDayOffStore struct {
	mu sync.Mutex
	m  map[string]domain.DayOff
}

func NewFakeDayOffStore() *FakeDayOffStore { return &FakeDayOffStore{m: map[string]domain.DayOff{}} }

func dayKey(owner string, day time.Time) string { return owner + ":" + day.Format("2006-01-02") }

func (s *FakeDayOffStore) Add(_ context.Context, ownerID string, d domain.DayOff) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[dayKey(ownerID, d.Date)] = d
	return nil
}

func (s *FakeDayOffStore) Delete(_ context.Context, ownerID string, day time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, dayKey(ownerID, day))
	return nil
}

func (s *FakeDayOffStore) ListRange(_ context.Context, ownerID string, from, to time.Time) ([]domain.DayOff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.DayOff
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if v, ok := s.m[dayKey(ownerID, d)]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

// FakeUserSettingsStore is an in-memory ports.UserSettingsStore with lazy NW default.
type FakeUserSettingsStore struct {
	mu       sync.Mutex
	land     map[string]string               // userID -> bundesland
	target   map[string]int                  // userID -> defaultTargetMin
	weekdays map[string]map[time.Weekday]int // userID -> weekday overrides
}

func NewFakeUserSettingsStore() *FakeUserSettingsStore {
	return &FakeUserSettingsStore{
		land:     map[string]string{},
		target:   map[string]int{},
		weekdays: map[string]map[time.Weekday]int{},
	}
}

func (s *FakeUserSettingsStore) Get(_ context.Context, userID string) (domain.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	land, ok := s.land[userID]
	if !ok {
		land = "NW"
	}
	defMin, ok := s.target[userID]
	if !ok {
		defMin = domain.DefaultDailyTargetMin
	}
	wds := map[time.Weekday]int{}
	for k, v := range s.weekdays[userID] {
		wds[k] = v
	}
	return domain.Settings{UserID: userID, Bundesland: land, DefaultTargetMin: defMin, WeekdayTargetMin: wds}, nil
}

func (s *FakeUserSettingsStore) SetBundesland(_ context.Context, userID, land string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.land[userID] = land
	return nil
}

func (s *FakeUserSettingsStore) SetTargetConfig(_ context.Context, userID string, defaultMin int, weekday map[time.Weekday]int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.target[userID] = defaultMin
	wds := map[time.Weekday]int{}
	for k, v := range weekday {
		wds[k] = v
	}
	s.weekdays[userID] = wds
	return nil
}

// FakeFeedTokenStore is an in-memory ports.FeedTokenStore.
type FakeFeedTokenStore struct {
	mu sync.Mutex
	m  map[string]domain.FeedToken // active tokens by token string
}

func NewFakeFeedTokenStore() *FakeFeedTokenStore {
	return &FakeFeedTokenStore{m: map[string]domain.FeedToken{}}
}

func (s *FakeFeedTokenStore) Create(_ context.Context, ft domain.FeedToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[ft.Token] = ft
	return nil
}

func (s *FakeFeedTokenStore) Resolve(_ context.Context, token string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ft, ok := s.m[token]
	if !ok {
		return "", ports.ErrFeedTokenNotFound
	}
	return ft.UserID, nil
}

func (s *FakeFeedTokenStore) ListByUser(_ context.Context, userID string) ([]domain.FeedToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.FeedToken
	for _, ft := range s.m {
		if ft.UserID == userID {
			out = append(out, ft)
		}
	}
	return out, nil
}

func (s *FakeFeedTokenStore) Revoke(_ context.Context, userID, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ft, ok := s.m[token]; ok && ft.UserID == userID {
		delete(s.m, token)
	}
	return nil
}

// FakeDocumentStore is an in-memory ports.DocumentStore.
// Create returns ErrDocumentExists on an (owner, coalesce(nodeID,""), path) collision.
type FakeDocumentStore struct {
	mu         sync.Mutex
	m          map[string]domain.Document // keyed by id
	seq        int                        // counter for UpsertByPath-generated ids
	links      map[string][]string        // srcDocID -> target paths
	chunks     map[string][]fakeChunk     // docID -> chunks
	chunksHash map[string]string          // docID -> stamped hash
	embedFail  map[string]fakeEmbedFail
}

func NewFakeDocumentStore() *FakeDocumentStore {
	return &FakeDocumentStore{
		m:          map[string]domain.Document{},
		links:      map[string][]string{},
		chunks:     map[string][]fakeChunk{},
		chunksHash: map[string]string{},
		embedFail:  map[string]fakeEmbedFail{},
	}
}

type fakeChunk struct {
	content string
	emb     []float32
}

type fakeEmbedFail struct {
	attempts  int
	nextRetry time.Time
	lastErr   string
	dead      bool
}

func fakeDocHash(d domain.Document) string {
	sum := sha256.Sum256([]byte(itoa(len(d.Title)) + ":" + d.Title + d.Body))
	return string(sum[:])
}

// SnapshotHash returns the current content snapshot hash for embedding tests.
func (s *FakeDocumentStore) SnapshotHash(docID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fakeDocHash(s.m[docID])
}

func docCollisionKey(ownerID string, nodeID *string, path string) string {
	proj := ""
	if nodeID != nil {
		proj = *nodeID
	}
	return ownerID + "\x00" + proj + "\x00" + path
}

func (s *FakeDocumentStore) Create(_ context.Context, d domain.Document) (domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := docCollisionKey(d.OwnerID, d.NodeID, d.Path)
	for _, existing := range s.m {
		if docCollisionKey(existing.OwnerID, existing.NodeID, existing.Path) == want {
			return domain.Document{}, ports.ErrDocumentExists
		}
	}
	d.ContextMode = d.ContextMode.OrAuto()
	s.m[d.ID] = d
	return d, nil
}

func (s *FakeDocumentStore) Get(_ context.Context, ownerID, id string) (domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.m[id]
	if !ok || d.OwnerID != ownerID {
		return domain.Document{}, ports.ErrDocumentNotFound
	}
	return d, nil
}

func (s *FakeDocumentStore) List(_ context.Context, ownerID string, nodeID *string, tags ...string) ([]domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Document
	for _, d := range s.m {
		if d.OwnerID == ownerID && !d.Archived && matchesNode(d.NodeID, nodeID) && hasAllTags(d.Tags, tags) {
			out = append(out, d)
		}
	}
	// newest-first by UpdatedAt
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].UpdatedAt.After(out[i].UpdatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (s *FakeDocumentStore) ListPage(ctx context.Context, ownerID string, nodeID *string, limit, offset int, tags ...string) ([]domain.Document, int, error) {
	all, err := s.List(ctx, ownerID, nodeID, tags...)
	if err != nil {
		return nil, 0, err
	}
	total := len(all)
	if limit <= 0 {
		limit = total
	}
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (s *FakeDocumentStore) ListLibraryPage(_ context.Context, ownerID string, query ports.DocumentLibraryQuery) (ports.DocumentLibraryPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	allowedNodes := make(map[string]bool, len(query.NodeIDs))
	for _, id := range query.NodeIDs {
		allowedNodes[id] = true
	}
	allowedTypes := make(map[domain.DocumentType]bool, len(query.Types))
	for _, typ := range query.Types {
		allowedTypes[typ] = true
	}

	var filtered []domain.Document
	for _, d := range s.m {
		if d.OwnerID != ownerID || !hasAllTags(d.Tags, query.Tags) {
			continue
		}
		if query.UnassignedOnly {
			if d.NodeID != nil {
				continue
			}
		} else if query.FilterNodeIDs && (d.NodeID == nil || !allowedNodes[*d.NodeID]) {
			continue
		}
		if len(allowedTypes) > 0 && !allowedTypes[d.Type] {
			continue
		}
		filtered = append(filtered, d)
	}

	if strings.TrimSpace(query.Search) != "" {
		return s.searchLibraryPageLocked(filtered, query), nil
	}

	page := ports.DocumentLibraryPage{}
	var matching []domain.Document
	for _, d := range filtered {
		if d.Archived {
			page.ArchivedTotal++
		} else {
			page.ActiveTotal++
		}
		if libraryStatusMatches(d, query.Status) {
			matching = append(matching, d)
		}
	}
	setDocumentLibraryFacets(&page, matching)

	sort.SliceStable(matching, func(i, j int) bool {
		if query.Status == ports.DocumentLibraryArchived {
			left, right := matching[i].ArchivedAt, matching[j].ArchivedAt
			if left != nil && right != nil && !left.Equal(*right) {
				return left.After(*right)
			}
		}
		if matching[i].UpdatedAt.Equal(matching[j].UpdatedAt) {
			return matching[i].ID < matching[j].ID
		}
		return matching[i].UpdatedAt.After(matching[j].UpdatedAt)
	})
	page.Total = len(matching)
	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > len(matching) {
		offset = len(matching)
	}
	end := offset + limit
	if end > len(matching) {
		end = len(matching)
	}
	page.Documents = append([]domain.Document(nil), matching[offset:end]...)
	return page, nil
}

func (s *FakeDocumentStore) searchLibraryPageLocked(filtered []domain.Document, query ports.DocumentLibraryQuery) ports.DocumentLibraryPage {
	type ranked struct {
		doc     domain.Document
		snippet string
		score   float64
	}
	q := strings.ToLower(strings.TrimSpace(query.Search))
	byID := make(map[string]*ranked, len(filtered))
	keyword := make([]ranked, 0, len(filtered))
	for _, d := range filtered {
		text := d.Title + " " + d.Body
		idx := strings.Index(strings.ToLower(text), q)
		if idx < 0 {
			continue
		}
		keyword = append(keyword, ranked{doc: d, snippet: fakeSnippet(text, idx, len(query.Search))})
	}
	sort.SliceStable(keyword, func(i, j int) bool {
		if keyword[i].doc.UpdatedAt.Equal(keyword[j].doc.UpdatedAt) {
			return keyword[i].doc.ID < keyword[j].doc.ID
		}
		return keyword[i].doc.UpdatedAt.After(keyword[j].doc.UpdatedAt)
	})
	for i := range keyword {
		hit := keyword[i]
		hit.score = 1 / float64(60+i+1)
		copy := hit
		byID[hit.doc.ID] = &copy
	}

	if len(query.Embedding) > 0 {
		semantic := make([]ranked, 0, len(filtered))
		for _, d := range filtered {
			best := -1.0
			bestContent := ""
			for _, chunk := range s.chunks[d.ID] {
				sim := cosine(query.Embedding, chunk.emb)
				if sim > best {
					best = sim
					bestContent = chunk.content
				}
			}
			if bestContent != "" {
				semantic = append(semantic, ranked{doc: d, snippet: bestContent, score: 1 - best})
			}
		}
		sort.SliceStable(semantic, func(i, j int) bool {
			if semantic[i].score == semantic[j].score {
				return semantic[i].doc.ID < semantic[j].doc.ID
			}
			return semantic[i].score < semantic[j].score
		})
		for i, hit := range semantic {
			score := 1 / float64(60+i+1)
			if existing := byID[hit.doc.ID]; existing != nil {
				existing.score += score
				continue
			}
			hit.score = score
			copy := hit
			byID[hit.doc.ID] = &copy
		}
	}

	all := make([]ranked, 0, len(byID))
	page := ports.DocumentLibraryPage{}
	for _, hit := range byID {
		all = append(all, *hit)
		if hit.doc.Archived {
			page.ArchivedTotal++
		} else {
			page.ActiveTotal++
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		if !all[i].doc.UpdatedAt.Equal(all[j].doc.UpdatedAt) {
			return all[i].doc.UpdatedAt.After(all[j].doc.UpdatedAt)
		}
		return all[i].doc.ID < all[j].doc.ID
	})
	matching := make([]ranked, 0, len(all))
	matchingDocs := make([]domain.Document, 0, len(all))
	for _, hit := range all {
		if libraryStatusMatches(hit.doc, query.Status) {
			matching = append(matching, hit)
			matchingDocs = append(matchingDocs, hit.doc)
		}
	}
	setDocumentLibraryFacets(&page, matchingDocs)
	page.Total = len(matching)
	limit, offset := libraryPageBounds(query, len(matching))
	end := offset + limit
	if end > len(matching) {
		end = len(matching)
	}
	for _, hit := range matching[offset:end] {
		page.Results = append(page.Results, domain.SearchHit{Document: hit.doc, Snippet: hit.snippet})
	}
	return page
}

func setDocumentLibraryFacets(page *ports.DocumentLibraryPage, docs []domain.Document) {
	page.TypeTotals = make(map[domain.DocumentType]int)
	for _, d := range docs {
		page.TypeTotals[d.Type]++
	}
	page.TagTotals = domain.CollectTags(docs)
}

func libraryStatusMatches(d domain.Document, status ports.DocumentLibraryStatus) bool {
	switch status {
	case ports.DocumentLibraryArchived:
		return d.Archived
	case ports.DocumentLibraryAll:
		return true
	default:
		return !d.Archived
	}
}

func libraryPageBounds(query ports.DocumentLibraryQuery, total int) (int, int) {
	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	return limit, offset
}

func matchesNode(docPID, filter *string) bool {
	if filter == nil {
		return true
	}
	if *filter == "none" {
		return docPID == nil
	}
	return docPID != nil && *docPID == *filter
}

func hasAllTags(have, want []string) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			if h == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (s *FakeDocumentStore) Update(_ context.Context, d domain.Document) (domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.m[d.ID]
	if !ok || existing.OwnerID != d.OwnerID {
		return domain.Document{}, ports.ErrDocumentNotFound
	}
	// mirror pgstore: title/body/type/path/tags/extra/updated_at are mutable.
	// type and path are included so maintenance ops (RedesignDocTypes) can reclassify docs.
	existing.Title = d.Title
	existing.Body = d.Body
	existing.Type = d.Type
	existing.Path = d.Path
	existing.Tags = d.Tags
	existing.Extra = d.Extra
	existing.UpdatedAt = d.UpdatedAt
	existing.UpdatedByKind = d.UpdatedByKind
	existing.UpdatedByRef = d.UpdatedByRef
	s.m[d.ID] = existing
	delete(s.embedFail, d.ID)
	return existing, nil
}

func (s *FakeDocumentStore) Move(_ context.Context, d domain.Document) (domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.m[d.ID]
	if !ok || existing.OwnerID != d.OwnerID {
		return domain.Document{}, ports.ErrDocumentNotFound
	}
	want := docCollisionKey(d.OwnerID, d.NodeID, d.Path)
	for id, other := range s.m {
		if id != d.ID && docCollisionKey(other.OwnerID, other.NodeID, other.Path) == want {
			return domain.Document{}, ports.ErrDocumentExists
		}
	}
	existing.Type = d.Type
	existing.NodeID = d.NodeID
	existing.Path = d.Path
	existing.Date = d.Date
	existing.UpdatedAt = d.UpdatedAt
	existing.UpdatedByKind = d.UpdatedByKind
	existing.UpdatedByRef = d.UpdatedByRef
	s.m[d.ID] = existing
	return existing, nil
}

func (s *FakeDocumentStore) Delete(_ context.Context, ownerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.m[id]
	if !ok || d.OwnerID != ownerID {
		return ports.ErrDocumentNotFound
	}
	delete(s.links, id)
	delete(s.chunks, id)
	delete(s.chunksHash, id)
	delete(s.embedFail, id)
	delete(s.m, id)
	return nil
}

func (s *FakeDocumentStore) ReplaceLinks(_ context.Context, srcDocID, _ string, targets []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(targets) == 0 {
		delete(s.links, srcDocID)
		return nil
	}
	cp := make([]string, len(targets))
	copy(cp, targets)
	s.links[srcDocID] = cp
	return nil
}

// LinksFor returns the recorded wikilink target paths for a document id.
// Test-only introspection accessor.
func (s *FakeDocumentStore) LinksFor(docID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.links[docID]))
	copy(out, s.links[docID])
	return out
}

type fakeDocumentStoreSnapshot struct {
	m          map[string]domain.Document
	seq        int
	links      map[string][]string
	chunks     map[string][]fakeChunk
	chunksHash map[string]string
	embedFail  map[string]fakeEmbedFail
}

func (s *FakeDocumentStore) snapshot() fakeDocumentStoreSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := fakeDocumentStoreSnapshot{
		m:          make(map[string]domain.Document, len(s.m)),
		seq:        s.seq,
		links:      make(map[string][]string, len(s.links)),
		chunks:     make(map[string][]fakeChunk, len(s.chunks)),
		chunksHash: make(map[string]string, len(s.chunksHash)),
		embedFail:  make(map[string]fakeEmbedFail, len(s.embedFail)),
	}
	for id, d := range s.m {
		d.Tags = append([]string(nil), d.Tags...)
		if d.Extra != nil {
			extra := make(map[string]any, len(d.Extra))
			for key, value := range d.Extra {
				extra[key] = value
			}
			d.Extra = extra
		}
		out.m[id] = d
	}
	for id, links := range s.links {
		out.links[id] = append([]string(nil), links...)
	}
	for id, chunks := range s.chunks {
		cloned := make([]fakeChunk, len(chunks))
		for i := range chunks {
			cloned[i] = fakeChunk{content: chunks[i].content, emb: append([]float32(nil), chunks[i].emb...)}
		}
		out.chunks[id] = cloned
	}
	for id, hash := range s.chunksHash {
		out.chunksHash[id] = hash
	}
	for id, failure := range s.embedFail {
		out.embedFail[id] = failure
	}
	return out
}

func (s *FakeDocumentStore) restore(snapshot fakeDocumentStoreSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = snapshot.m
	s.seq = snapshot.seq
	s.links = snapshot.links
	s.chunks = snapshot.chunks
	s.chunksHash = snapshot.chunksHash
	s.embedFail = snapshot.embedFail
}

func (s *FakeDocumentStore) Backlinks(_ context.Context, ownerID, targetPath string) ([]domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Document
	for srcID, targets := range s.links {
		d, ok := s.m[srcID]
		if !ok || d.OwnerID != ownerID {
			continue
		}
		for _, tgt := range targets {
			if tgt == targetPath {
				out = append(out, d)
				break
			}
		}
	}
	return out, nil
}

func (s *FakeDocumentStore) Search(_ context.Context, ownerID, q string, nodeID *string, tags []string) ([]domain.SearchHit, error) {
	return s.SearchQuery(context.Background(), ownerID, ports.DocumentSearchQuery{Text: q, NodeID: nodeID, Tags: tags})
}

func (s *FakeDocumentStore) SearchQuery(_ context.Context, ownerID string, query ports.DocumentSearchQuery) ([]domain.SearchHit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ql := strings.ToLower(query.Text)
	var out []domain.SearchHit
	for _, d := range s.m {
		if d.OwnerID != ownerID || d.Archived || !matchesNode(d.NodeID, query.NodeID) || !hasAllTags(d.Tags, query.Tags) || (query.Type != "" && d.Type != query.Type) {
			continue
		}
		hay := strings.ToLower(d.Title + " " + d.Body)
		idx := strings.Index(hay, ql)
		if ql == "" || idx < 0 {
			continue
		}
		out = append(out, domain.SearchHit{Document: d, Snippet: fakeSnippet(d.Title+" "+d.Body, idx, len(query.Text))})
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].UpdatedAt.After(out[i].UpdatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if query.Limit > 0 && len(out) > query.Limit {
		out = out[:query.Limit]
	}
	return out, nil
}

// fakeSnippet wraps the matched [start,start+n) span of text with the shared
// highlight sentinels — enough to exercise host snippet rendering in tests.
func fakeSnippet(text string, start, n int) string {
	end := start + n
	if end > len(text) {
		end = len(text)
	}
	return text[:start] + domain.HighlightStart + text[start:end] + domain.HighlightEnd + text[end:]
}

func (s *FakeDocumentStore) StaleDocuments(_ context.Context, limit int) ([]ports.StaleDoc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var out []ports.StaleDoc
	for _, d := range s.m {
		if s.chunksHash[d.ID] == fakeDocHash(d) {
			continue
		}
		attempts := 0
		if f, ok := s.embedFail[d.ID]; ok {
			if f.dead || f.nextRetry.After(now) {
				continue
			}
			attempts = f.attempts
		}
		out = append(out, ports.StaleDoc{Doc: d, Attempts: attempts, SnapshotHash: fakeDocHash(d)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Doc.UpdatedAt.Equal(out[j].Doc.UpdatedAt) {
			return out[i].Doc.ID < out[j].Doc.ID
		}
		return out[i].Doc.UpdatedAt.Before(out[j].Doc.UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *FakeDocumentStore) ReplaceChunks(_ context.Context, docID, ownerID, snapshotHash string, contents []string, embeddings [][]float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.m[docID]
	if !ok || d.OwnerID != ownerID {
		return ports.ErrDocumentNotFound
	}
	if fakeDocHash(d) != snapshotHash {
		return ports.ErrEmbedStaleSnapshot
	}
	cs := make([]fakeChunk, len(contents))
	for i := range contents {
		cs[i] = fakeChunk{content: contents[i], emb: embeddings[i]}
	}
	s.chunks[docID] = cs
	s.chunksHash[docID] = snapshotHash
	delete(s.embedFail, docID)
	return nil
}

func (s *FakeDocumentStore) RecordEmbedFailure(_ context.Context, docID, ownerID, snapshotHash string, attempts int, nextRetryAt time.Time, dead bool, lastErr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.m[docID]
	if !ok || d.OwnerID != ownerID {
		return ports.ErrDocumentNotFound
	}
	if fakeDocHash(d) != snapshotHash {
		return ports.ErrEmbedStaleSnapshot
	}
	s.embedFail[docID] = fakeEmbedFail{attempts: attempts, nextRetry: nextRetryAt, lastErr: lastErr, dead: dead}
	return nil
}

func (s *FakeDocumentStore) ClearEmbedFailure(_ context.Context, docID, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.embedFail, docID)
	return nil
}

func (s *FakeDocumentStore) EmbedStatus(_ context.Context, ownerID, docID string) (domain.EmbedStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.m[docID]
	if !ok || d.OwnerID != ownerID {
		return domain.EmbedStatus{}, ports.ErrDocumentNotFound
	}
	if f, ok := s.embedFail[docID]; ok {
		st := domain.EmbedStatus{Attempts: f.attempts, LastError: f.lastErr}
		if f.dead {
			st.State = domain.EmbedFailed
		} else {
			st.State = domain.EmbedRetrying
			nr := f.nextRetry
			st.NextRetry = &nr
		}
		return st, nil
	}
	if s.chunksHash[docID] != fakeDocHash(d) {
		return domain.EmbedStatus{State: domain.EmbedPending}, nil
	}
	return domain.EmbedStatus{State: domain.EmbedOK}, nil
}

func (s *FakeDocumentStore) SemanticSearch(_ context.Context, ownerID string, query []float32, nodeID *string, tags []string, limit int) ([]domain.SemanticHit, error) {
	return s.SemanticSearchQuery(context.Background(), ownerID, query, ports.DocumentSearchQuery{NodeID: nodeID, Tags: tags, Limit: limit})
}

func (s *FakeDocumentStore) SemanticSearchQuery(_ context.Context, ownerID string, embedding []float32, query ports.DocumentSearchQuery) ([]domain.SemanticHit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var hits []domain.SemanticHit
	for _, d := range s.m {
		if d.OwnerID != ownerID || d.Archived || !matchesNode(d.NodeID, query.NodeID) || !hasAllTags(d.Tags, query.Tags) || (query.Type != "" && d.Type != query.Type) {
			continue
		}
		best := -1.0
		bestContent := ""
		for _, c := range s.chunks[d.ID] {
			sim := cosine(embedding, c.emb)
			if sim > best {
				best = sim
				bestContent = c.content
			}
		}
		if bestContent == "" {
			continue
		}
		hits = append(hits, domain.SemanticHit{Document: d, Snippet: bestContent, Distance: float32(1 - best)})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Distance < hits[j].Distance })
	if query.Limit > 0 && len(hits) > query.Limit {
		hits = hits[:query.Limit]
	}
	return hits, nil
}

func (s *FakeDocumentStore) SetPinned(_ context.Context, ownerID, id string, pinned bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.m[id]
	if !ok || d.OwnerID != ownerID {
		return ports.ErrDocumentNotFound
	}
	d.Pinned = pinned
	s.m[id] = d
	return nil
}

func (s *FakeDocumentStore) SetPriority(_ context.Context, ownerID, id string, priority int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.m[id]
	if !ok || d.OwnerID != ownerID {
		return ports.ErrDocumentNotFound
	}
	d.Priority = priority
	s.m[id] = d
	return nil
}

func (s *FakeDocumentStore) ReorderPriorities(_ context.Context, ownerID string, orderedIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range orderedIDs {
		d, ok := s.m[id]
		if !ok || d.OwnerID != ownerID {
			return ports.ErrDocumentNotFound
		}
	}
	for i, id := range orderedIDs {
		d := s.m[id]
		d.Priority = len(orderedIDs) - i
		s.m[id] = d
	}
	return nil
}

func (s *FakeDocumentStore) SetContextMode(_ context.Context, ownerID, id string, mode domain.ContextMode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.m[id]
	if !ok || d.OwnerID != ownerID {
		return ports.ErrDocumentNotFound
	}
	d.ContextMode = mode
	s.m[id] = d
	return nil
}

func (s *FakeDocumentStore) SetArchived(_ context.Context, ownerID, id string, archived bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.m[id]
	if !ok || d.OwnerID != ownerID {
		return ports.ErrDocumentNotFound
	}
	d.Archived = archived
	if archived {
		now := time.Now()
		d.ArchivedAt = &now
		d.Pinned = false
	} else {
		d.ArchivedAt = nil
	}
	s.m[id] = d
	return nil
}

func (s *FakeDocumentStore) CurateDocuments(_ context.Context, ownerID string, mutation ports.DocumentCurationMutation) ([]domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := uniqueFakeDocumentIDs(mutation.IDs)
	if len(ids) == 0 || (mutation.Archived == nil) == (mutation.ContextMode == nil) || mutation.At.IsZero() {
		return nil, domain.ErrInvalidDocument
	}
	for _, id := range ids {
		doc, ok := s.m[id]
		if !ok || doc.OwnerID != ownerID {
			return nil, ports.ErrDocumentNotFound
		}
		if mutation.ContextMode != nil && !doc.Type.ContextEligible() {
			return nil, domain.ErrInvalidDocument
		}
	}
	out := make([]domain.Document, 0, len(ids))
	for _, id := range ids {
		doc := s.m[id]
		doc.UpdatedByKind = mutation.ActorKind
		doc.UpdatedByRef = mutation.ActorRef
		if mutation.Archived != nil {
			doc.Archived = *mutation.Archived
			doc.UpdatedAt = mutation.At
			if *mutation.Archived {
				at := mutation.At
				doc.ArchivedAt = &at
				doc.Pinned = false
			} else {
				doc.ArchivedAt = nil
			}
		} else {
			doc.ContextMode = mutation.ContextMode.OrAuto()
		}
		s.m[id] = doc
		out = append(out, doc)
	}
	return out, nil
}

func uniqueFakeDocumentIDs(raw []string) []string {
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, id := range raw {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (s *FakeDocumentStore) ListArchived(_ context.Context, ownerID string) ([]domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Document
	for _, d := range s.m {
		if d.OwnerID == ownerID && d.Archived {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		var ai, aj time.Time
		if out[i].ArchivedAt != nil {
			ai = *out[i].ArchivedAt
		}
		if out[j].ArchivedAt != nil {
			aj = *out[j].ArchivedAt
		}
		return ai.After(aj)
	})
	return out, nil
}

func (s *FakeDocumentStore) UpsertByPath(_ context.Context, ownerID string, nodeID *string, typ domain.DocumentType, path, title, body string, pinned, archived bool, updatedByKind, updatedByRef string) (string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nv := ""
	if nodeID != nil {
		nv = *nodeID
	}
	for _, d := range s.m { // find existing by (owner, coalesce(node,''), path)
		dn := ""
		if d.NodeID != nil {
			dn = *d.NodeID
		}
		if d.OwnerID == ownerID && dn == nv && d.Path == path {
			d.Title, d.Body, d.Type = title, body, typ // preserve pinned, archived, id; mirror pgstore ON CONFLICT
			d.UpdatedByKind, d.UpdatedByRef = updatedByKind, updatedByRef
			s.m[d.ID] = d
			delete(s.embedFail, d.ID)
			return d.ID, time.Time{}, nil
		}
	}
	s.seq++
	id := "fdoc-" + itoa(s.seq)
	s.m[id] = domain.Document{
		ID: id, OwnerID: ownerID, NodeID: nodeID, Type: typ, Path: path, Title: title, Body: body,
		Pinned: pinned, Archived: archived, UpdatedByKind: updatedByKind, UpdatedByRef: updatedByRef,
		ContextMode: domain.ContextModeAuto, // mirrors pgstore's own column list → DB default 'auto'
	}
	return id, time.Time{}, nil
}

func (s *FakeDocumentStore) ListForContext(_ context.Context, ownerID string, nodeIDs []string, includeGlobal bool, types []domain.DocumentType) ([]domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inNodes := map[string]bool{}
	for _, n := range nodeIDs {
		inNodes[n] = true
	}
	inTypes := map[domain.DocumentType]bool{}
	for _, t := range types {
		inTypes[t] = true
	}
	var out []domain.Document
	for _, d := range s.m {
		if d.OwnerID != ownerID || d.Archived || !inTypes[d.Type] {
			continue
		}
		switch {
		case d.NodeID == nil:
			if includeGlobal {
				out = append(out, d)
			}
		case inNodes[*d.NodeID]:
			out = append(out, d)
		}
	}
	return out, nil
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return -1
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return -1
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// FakeTagStore is an in-memory ports.TagStore.
type FakeTagStore struct {
	mu      sync.Mutex
	display map[string]string          // owner|slug -> display
	links   map[string]map[string]bool // owner|type|id -> set of slugs
	idgen   int
}

func NewFakeTagStore() *FakeTagStore {
	return &FakeTagStore{display: map[string]string{}, links: map[string]map[string]bool{}}
}

type fakeTagStoreSnapshot struct {
	display map[string]string
	links   map[string]map[string]bool
	idgen   int
}

func (s *FakeTagStore) snapshot() fakeTagStoreSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := fakeTagStoreSnapshot{
		display: make(map[string]string, len(s.display)),
		links:   make(map[string]map[string]bool, len(s.links)),
		idgen:   s.idgen,
	}
	for key, value := range s.display {
		out.display[key] = value
	}
	for key, values := range s.links {
		copyValues := make(map[string]bool, len(values))
		for slug, present := range values {
			copyValues[slug] = present
		}
		out.links[key] = copyValues
	}
	return out
}

func (s *FakeTagStore) restore(snapshot fakeTagStoreSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.display = snapshot.display
	s.links = snapshot.links
	s.idgen = snapshot.idgen
}

func (s *FakeTagStore) key(owner string, typ domain.TaggableType, id string) string {
	return owner + "|" + string(typ) + "|" + id
}

func (s *FakeTagStore) SetTags(_ context.Context, ownerID string, typ domain.TaggableType, id string, raw []string) ([]domain.Tag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	set := map[string]bool{}
	var out []domain.Tag
	seen := map[string]bool{}
	for _, rawStr := range raw {
		slug, ok := domain.NormalizeTag(rawStr)
		if !ok || seen[slug] {
			continue
		}
		seen[slug] = true
		set[slug] = true
		dk := ownerID + "|" + slug
		if _, exists := s.display[dk]; !exists {
			s.display[dk] = rawStr // first-seen RAW casing
		}
		s.idgen++
		out = append(out, domain.Tag{ID: "ft", OwnerID: ownerID, Slug: slug, Display: s.display[dk]})
	}
	s.links[s.key(ownerID, typ, id)] = set
	return out, nil
}

func (s *FakeTagStore) TagsFor(_ context.Context, ownerID string, typ domain.TaggableType, id string) ([]domain.Tag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Tag
	for slug := range s.links[s.key(ownerID, typ, id)] {
		out = append(out, domain.Tag{OwnerID: ownerID, Slug: slug, Display: s.display[ownerID+"|"+slug]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

func (s *FakeTagStore) TagsForMany(ctx context.Context, ownerID string, typ domain.TaggableType, ids []string) (map[string][]domain.Tag, error) {
	out := map[string][]domain.Tag{}
	for _, id := range ids {
		t, _ := s.TagsFor(ctx, ownerID, typ, id)
		if len(t) > 0 {
			out[id] = t
		}
	}
	return out, nil
}

func (s *FakeTagStore) FilterIDs(_ context.Context, ownerID string, typ domain.TaggableType, slugs []string, mode domain.TagMatch) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := domain.NormalizeTags(slugs)
	var out []string
	prefix := ownerID + "|" + string(typ) + "|"
	for k, set := range s.links {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		n := 0
		for _, w := range want {
			if set[w] {
				n++
			}
		}
		ok := (mode == domain.TagMatchAll && n == len(want)) || (mode == domain.TagMatchAny && n > 0)
		if ok {
			out = append(out, strings.TrimPrefix(k, prefix))
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s *FakeTagStore) ListTags(_ context.Context, ownerID string, scope domain.TagScope) ([]domain.TagCount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := map[string]int{}
	for k, set := range s.links {
		parts := strings.SplitN(k, "|", 3)
		if parts[0] != ownerID {
			continue
		}
		if scope.Type != nil && parts[1] != string(*scope.Type) {
			continue
		}
		for slug := range set {
			counts[slug]++
		}
	}
	var out []domain.TagCount
	for slug, n := range counts {
		out = append(out, domain.TagCount{Tag: slug, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out, nil
}

func (s *FakeTagStore) ClearTaggable(_ context.Context, ownerID string, typ domain.TaggableType, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.links, s.key(ownerID, typ, id))
	return nil
}

func (s *FakeTagStore) MergeTags(_ context.Context, ownerID, fromSlug, intoSlug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	from, _ := domain.NormalizeTag(fromSlug)
	into, _ := domain.NormalizeTag(intoSlug)
	prefix := ownerID + "|"
	for k, set := range s.links {
		if strings.HasPrefix(k, prefix) && set[from] {
			delete(set, from)
			set[into] = true
		}
	}
	return nil
}

// FakeProjectBindingStore is an in-memory ports.ProjectBindingStore.
// Upsert replaces an existing row by kind-key: (owner, remote_slug) for
// BindingRemote, or (owner, machine_id, path) for BindingPath.
type FakeProjectBindingStore struct {
	mu    sync.Mutex
	items []domain.ProjectBinding
}

func NewFakeProjectBindingStore() *FakeProjectBindingStore { return &FakeProjectBindingStore{} }

// BindRemote is a test-only convenience that upserts a remote-slug→node binding.
func (s *FakeProjectBindingStore) BindRemote(ctx context.Context, ownerID, remoteSlug, nodeID string) error {
	_, err := s.Upsert(ctx, domain.ProjectBinding{
		OwnerID: ownerID, NodeID: nodeID,
		Kind: domain.BindingRemote, RemoteSlug: remoteSlug,
	})
	return err
}

// bindingKey returns the natural key for upsert/delete matching.
func bindingKey(b domain.ProjectBinding) string {
	if b.Kind == domain.BindingRemote {
		return "remote\x00" + b.OwnerID + "\x00" + b.RemoteSlug
	}
	return "path\x00" + b.OwnerID + "\x00" + b.MachineID + "\x00" + b.Path
}

func (s *FakeProjectBindingStore) Upsert(_ context.Context, b domain.ProjectBinding) (domain.ProjectBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := bindingKey(b)
	for i, existing := range s.items {
		if bindingKey(existing) == key {
			s.items[i] = b
			return b, nil
		}
	}
	s.items = append(s.items, b)
	return b, nil
}

func (s *FakeProjectBindingStore) DeleteRemote(_ context.Context, ownerID, remoteSlug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := bindingKey(domain.ProjectBinding{Kind: domain.BindingRemote, OwnerID: ownerID, RemoteSlug: remoteSlug})
	out := s.items[:0]
	for _, b := range s.items {
		if bindingKey(b) != want {
			out = append(out, b)
		}
	}
	s.items = out
	return nil
}

func (s *FakeProjectBindingStore) DeletePath(_ context.Context, ownerID, machineID, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := bindingKey(domain.ProjectBinding{Kind: domain.BindingPath, OwnerID: ownerID, MachineID: machineID, Path: path})
	out := s.items[:0]
	for _, b := range s.items {
		if bindingKey(b) != want {
			out = append(out, b)
		}
	}
	s.items = out
	return nil
}

func (s *FakeProjectBindingStore) List(_ context.Context, ownerID string) ([]domain.ProjectBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.ProjectBinding
	for _, b := range s.items {
		if b.OwnerID == ownerID {
			cp := b
			out = append(out, cp)
		}
	}
	return out, nil
}

// All returns every stored binding regardless of owner (test introspection).
func (s *FakeProjectBindingStore) All() []domain.ProjectBinding {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.ProjectBinding, len(s.items))
	copy(out, s.items)
	return out
}

func (s *FakeProjectBindingStore) ListByProject(_ context.Context, ownerID, nodeID string) ([]domain.ProjectBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.ProjectBinding
	for _, b := range s.items {
		if b.OwnerID == ownerID && b.NodeID == nodeID {
			cp := b
			out = append(out, cp)
		}
	}
	return out, nil
}

// FakeEmbedder returns deterministic unit vectors derived from a hash of each
// text — identical text yields an identical vector, so similarity *ordering* is
// reproducible without a real model. It does NOT model semantic nearness; tests
// assert wiring/ordering, not embedding quality. Set Err to simulate Ollama down.
// Set FailFunc for per-call error injection (checked before Err).
type FakeEmbedder struct {
	Dim      int
	Err      error
	FailFunc func(texts []string) error // optional per-call hook (checked before Err)
}

func NewFakeEmbedder() *FakeEmbedder { return &FakeEmbedder{Dim: 768} }

func (f *FakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if f.FailFunc != nil {
		if err := f.FailFunc(texts); err != nil {
			return nil, err
		}
	}
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = pseudoVec(t, f.Dim)
	}
	return out, nil
}

func pseudoVec(s string, dim int) []float32 {
	h := sha256.Sum256([]byte(s))
	v := make([]float32, dim)
	var norm float64
	for i := 0; i < dim; i++ {
		x := float32(int(h[i%len(h)])-128) / 128.0
		x += float32((i*2654435761)%1000) / 1000.0
		v[i] = x
		norm += float64(x) * float64(x)
	}
	if n := float32(math.Sqrt(norm)); n > 0 {
		for i := range v {
			v[i] /= n
		}
	}
	return v
}

var _ ports.TagStore = (*FakeTagStore)(nil)

// FakeNodeLogoStore is an in-memory ports.NodeLogoStore (keyed by node ID).
type FakeNodeLogoStore struct {
	mu    sync.Mutex
	logos map[string]domain.NodeLogo
}

// NewFakeNodeLogoStore builds an empty in-memory logo store.
func NewFakeNodeLogoStore() *FakeNodeLogoStore {
	return &FakeNodeLogoStore{logos: map[string]domain.NodeLogo{}}
}

func (s *FakeNodeLogoStore) Put(_ context.Context, l domain.NodeLogo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logos[l.NodeID] = l
	return nil
}

func (s *FakeNodeLogoStore) Get(_ context.Context, ownerID, nodeID string) (domain.NodeLogo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.logos[nodeID]
	if !ok || l.OwnerID != ownerID {
		return domain.NodeLogo{}, ports.ErrNodeLogoNotFound
	}
	return l, nil
}

func (s *FakeNodeLogoStore) Delete(_ context.Context, ownerID, nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.logos[nodeID]; ok && l.OwnerID == ownerID {
		delete(s.logos, nodeID)
	}
	return nil
}

const (
	NodeAggregateFailNode   = "node"
	NodeAggregateFailRate   = "rate"
	NodeAggregateFailTags   = "tags"
	NodeAggregateFailLogo   = "logo"
	NodeAggregateFailCommit = "commit"
)

// FakeNodeAggregateStore provides an atomic in-memory transaction boundary
// across the existing node, logo and tag fakes. FailStage is a test hook that
// injects a failure after earlier stages have run; snapshots are restored just
// like a database rollback.
type FakeNodeAggregateStore struct {
	mu        sync.Mutex
	Nodes     *FakeNodeStore
	Logos     *FakeNodeLogoStore
	Tags      *FakeTagStore
	FailStage string
}

func NewFakeNodeAggregateStore(nodes *FakeNodeStore, logos *FakeNodeLogoStore, tags *FakeTagStore) *FakeNodeAggregateStore {
	return &FakeNodeAggregateStore{Nodes: nodes, Logos: logos, Tags: tags}
}

type fakeNodeAggregateSnapshot struct {
	nodes      map[string]domain.Node
	logos      map[string]domain.NodeLogo
	tagDisplay map[string]string
	tagLinks   map[string]map[string]bool
	tagIDGen   int
}

func (s *FakeNodeAggregateStore) snapshot() fakeNodeAggregateSnapshot {
	s.Nodes.mu.Lock()
	nodes := make(map[string]domain.Node, len(s.Nodes.m))
	for k, v := range s.Nodes.m {
		nodes[k] = v
	}
	s.Nodes.mu.Unlock()
	s.Logos.mu.Lock()
	logos := make(map[string]domain.NodeLogo, len(s.Logos.logos))
	for k, v := range s.Logos.logos {
		logos[k] = v
	}
	s.Logos.mu.Unlock()
	s.Tags.mu.Lock()
	display := make(map[string]string, len(s.Tags.display))
	for k, v := range s.Tags.display {
		display[k] = v
	}
	links := make(map[string]map[string]bool, len(s.Tags.links))
	for k, v := range s.Tags.links {
		cp := make(map[string]bool, len(v))
		for slug, present := range v {
			cp[slug] = present
		}
		links[k] = cp
	}
	idgen := s.Tags.idgen
	s.Tags.mu.Unlock()
	return fakeNodeAggregateSnapshot{nodes: nodes, logos: logos, tagDisplay: display, tagLinks: links, tagIDGen: idgen}
}

func (s *FakeNodeAggregateStore) restore(snap fakeNodeAggregateSnapshot) {
	s.Nodes.mu.Lock()
	s.Nodes.m = snap.nodes
	s.Nodes.mu.Unlock()
	s.Logos.mu.Lock()
	s.Logos.logos = snap.logos
	s.Logos.mu.Unlock()
	s.Tags.mu.Lock()
	s.Tags.display = snap.tagDisplay
	s.Tags.links = snap.tagLinks
	s.Tags.idgen = snap.tagIDGen
	s.Tags.mu.Unlock()
}

func (s *FakeNodeAggregateStore) fail(stage string) error {
	if s.FailStage == stage {
		return errors.New("fake node aggregate failure: " + stage)
	}
	return nil
}

func (s *FakeNodeAggregateStore) CreateAggregate(ctx context.Context, n domain.Node, changes ports.NodeAggregateChanges) (domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := s.snapshot()
	rollback := func(err error) (domain.Node, error) {
		s.restore(snap)
		return domain.Node{}, err
	}
	if changes.SetRate {
		n.Rate = changes.Rate
	}
	if changes.Logo == ports.NodeLogoPut {
		if changes.LogoValue.OwnerID != n.OwnerID || changes.LogoValue.NodeID != n.ID {
			return rollback(errors.New("fake node aggregate create logo identity mismatch"))
		}
		n.LogoRef = changes.LogoValue.Ref
	}
	created, err := s.Nodes.Create(ctx, n)
	if err != nil {
		return rollback(err)
	}
	if err := s.fail(NodeAggregateFailNode); err != nil {
		return rollback(err)
	}
	if changes.SetRate {
		if err := s.fail(NodeAggregateFailRate); err != nil {
			return rollback(err)
		}
	}
	if changes.SetTags {
		if _, err := s.Tags.SetTags(ctx, n.OwnerID, domain.TaggableNode, n.ID, changes.Tags); err != nil {
			return rollback(err)
		}
		if err := s.fail(NodeAggregateFailTags); err != nil {
			return rollback(err)
		}
	}
	if changes.Logo == ports.NodeLogoPut {
		if err := s.Logos.Put(ctx, changes.LogoValue); err != nil {
			return rollback(err)
		}
		if err := s.fail(NodeAggregateFailLogo); err != nil {
			return rollback(err)
		}
	}
	if err := s.fail(NodeAggregateFailCommit); err != nil {
		return rollback(err)
	}
	return created, nil
}

func (s *FakeNodeAggregateStore) UpdateAggregate(ctx context.Context, ownerID, nodeID string, mutate func(domain.Node) (domain.Node, ports.NodeAggregateChanges, error)) (domain.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := s.snapshot()
	rollback := func(err error) (domain.Node, error) {
		s.restore(snap)
		return domain.Node{}, err
	}
	current, err := s.Nodes.Get(ctx, ownerID, nodeID)
	if err != nil {
		return domain.Node{}, err
	}
	n, changes, err := mutate(current)
	if err != nil {
		return domain.Node{}, err
	}
	switch changes.Logo {
	case ports.NodeLogoKeep:
		n.LogoRef = current.LogoRef
	case ports.NodeLogoPut:
		if changes.LogoValue.OwnerID != current.OwnerID || changes.LogoValue.NodeID != current.ID {
			return rollback(errors.New("fake node aggregate update logo identity mismatch"))
		}
		n.LogoRef = changes.LogoValue.Ref
	case ports.NodeLogoDelete:
		n.LogoRef = ""
	}
	updated, err := s.Nodes.Update(ctx, ownerID, n)
	if err != nil {
		return rollback(err)
	}
	if err := s.fail(NodeAggregateFailNode); err != nil {
		return rollback(err)
	}
	if changes.SetRate {
		if err := s.Nodes.SetRate(ctx, ownerID, nodeID, changes.Rate); err != nil {
			return rollback(err)
		}
		if err := s.fail(NodeAggregateFailRate); err != nil {
			return rollback(err)
		}
	}
	if changes.SetTags {
		if _, err := s.Tags.SetTags(ctx, ownerID, domain.TaggableNode, nodeID, changes.Tags); err != nil {
			return rollback(err)
		}
		if err := s.fail(NodeAggregateFailTags); err != nil {
			return rollback(err)
		}
	}
	switch changes.Logo {
	case ports.NodeLogoPut:
		if err := s.Logos.Put(ctx, changes.LogoValue); err != nil {
			return rollback(err)
		}
	case ports.NodeLogoDelete:
		if err := s.Logos.Delete(ctx, ownerID, nodeID); err != nil {
			return rollback(err)
		}
	}
	if changes.Logo != ports.NodeLogoKeep {
		if err := s.fail(NodeAggregateFailLogo); err != nil {
			return rollback(err)
		}
	}
	if err := s.fail(NodeAggregateFailCommit); err != nil {
		return rollback(err)
	}
	if changes.SetRate {
		updated.Rate = changes.Rate
	}
	return updated, nil
}

var _ ports.NodeAggregateStore = (*FakeNodeAggregateStore)(nil)

// artifactKey is the (owner,node,slug) composite key FakeArtifactStore uses,
// mirroring the DB's UNIQUE (owner_id, node_id, slug) constraint.
type artifactKey struct{ owner, node, slug string }

// FakeArtifactStore is an in-memory ports.ArtifactStore (keyed by owner+node+slug).
type FakeArtifactStore struct {
	mu        sync.Mutex
	artifacts map[artifactKey]domain.Artifact
}

// NewFakeArtifactStore builds an empty in-memory artifact store.
func NewFakeArtifactStore() *FakeArtifactStore {
	return &FakeArtifactStore{artifacts: map[artifactKey]domain.Artifact{}}
}

func (s *FakeArtifactStore) Put(_ context.Context, a domain.Artifact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts[artifactKey{a.OwnerID, a.NodeID, a.Slug}] = a
	return nil
}

func (s *FakeArtifactStore) Create(_ context.Context, a domain.Artifact, maxBytes int64) (domain.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	var slugs []string
	for key, stored := range s.artifacts {
		if stored.OwnerID == a.OwnerID {
			total += stored.SizeBytes
		}
		if key.owner == a.OwnerID && key.node == a.NodeID {
			slugs = append(slugs, key.slug)
		}
	}
	if total > maxBytes || a.SizeBytes > maxBytes-total {
		return domain.Artifact{}, ports.ErrArtifactQuotaExceeded
	}
	a.Slug = domain.NextArtifactSlug(a.Slug, slugs)
	s.artifacts[artifactKey{a.OwnerID, a.NodeID, a.Slug}] = a
	a.Bytes = nil
	return a, nil
}

func (s *FakeArtifactStore) Replace(_ context.Context, a domain.Artifact, maxBytes int64) (domain.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := artifactKey{a.OwnerID, a.NodeID, a.Slug}
	old, ok := s.artifacts[key]
	if !ok {
		return domain.Artifact{}, ports.ErrArtifactNotFound
	}
	var total int64
	for _, stored := range s.artifacts {
		if stored.OwnerID == a.OwnerID {
			total += stored.SizeBytes
		}
	}
	retained := total - old.SizeBytes
	if retained < 0 || retained > maxBytes || a.SizeBytes > maxBytes-retained {
		return domain.Artifact{}, ports.ErrArtifactQuotaExceeded
	}
	a.ID = old.ID
	a.CreatedAt = old.CreatedAt
	a.CreatedByKind = old.CreatedByKind
	a.CreatedByRef = old.CreatedByRef
	s.artifacts[key] = a
	a.Bytes = nil
	return a, nil
}

func (s *FakeArtifactStore) Get(_ context.Context, ownerID, nodeID, slug string) (domain.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.artifacts[artifactKey{ownerID, nodeID, slug}]
	if !ok {
		return domain.Artifact{}, ports.ErrArtifactNotFound
	}
	return a, nil
}

func (s *FakeArtifactStore) GetMeta(ctx context.Context, ownerID, nodeID, slug string) (domain.Artifact, error) {
	a, err := s.Get(ctx, ownerID, nodeID, slug)
	if err != nil {
		return domain.Artifact{}, err
	}
	a.Bytes = nil
	return a, nil
}

func (s *FakeArtifactStore) List(_ context.Context, ownerID string, nodeIDs ...string) ([]domain.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nodeSet := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		nodeSet[id] = true
	}
	var out []domain.Artifact
	for _, a := range s.artifacts {
		if a.OwnerID != ownerID || !nodeSet[a.NodeID] {
			continue
		}
		meta := a
		meta.Bytes = nil
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// ListFree returns owner-global (node-less) artifact META (no bytes), newest first.
func (s *FakeArtifactStore) ListFree(_ context.Context, ownerID string) ([]domain.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Artifact
	for _, a := range s.artifacts {
		if a.OwnerID != ownerID || a.NodeID != "" {
			continue
		}
		meta := a
		meta.Bytes = nil
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *FakeArtifactStore) Rename(_ context.Context, ownerID, nodeID, slug, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := artifactKey{ownerID, nodeID, slug}
	a, ok := s.artifacts[key]
	if !ok {
		return ports.ErrArtifactNotFound
	}
	a.Name = name
	s.artifacts[key] = a
	return nil
}

func (s *FakeArtifactStore) Delete(_ context.Context, ownerID, nodeID, slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := artifactKey{ownerID, nodeID, slug}
	if _, ok := s.artifacts[key]; !ok {
		return ports.ErrArtifactNotFound
	}
	delete(s.artifacts, key)
	return nil
}

func (s *FakeArtifactStore) ExistingSlugs(_ context.Context, ownerID, nodeID string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for k := range s.artifacts {
		if k.owner == ownerID && k.node == nodeID {
			out = append(out, k.slug)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s *FakeArtifactStore) TotalBytes(_ context.Context, ownerID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	for _, a := range s.artifacts {
		if a.OwnerID == ownerID {
			total += a.SizeBytes
		}
	}
	return total, nil
}

var _ ports.ArtifactStore = (*FakeArtifactStore)(nil)

// ErrFakeDocumentAggregate is returned by an injected aggregate failure stage.
var ErrFakeDocumentAggregate = errors.New("fake document aggregate failure")

// FakeDocumentAggregateStore composes the document and tag fakes behind one
// rollback boundary. FailStage may be document, links, tags, curation, or
// commit.
type FakeDocumentAggregateStore struct {
	mu        sync.Mutex
	Docs      *FakeDocumentStore
	Tags      *FakeTagStore
	FailStage string
}

func NewFakeDocumentAggregateStore(docs *FakeDocumentStore, tags *FakeTagStore) *FakeDocumentAggregateStore {
	if docs == nil {
		docs = NewFakeDocumentStore()
	}
	if tags == nil {
		tags = NewFakeTagStore()
	}
	return &FakeDocumentAggregateStore{Docs: docs, Tags: tags}
}

func (s *FakeDocumentAggregateStore) transaction(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	docSnapshot := s.Docs.snapshot()
	tagSnapshot := s.Tags.snapshot()
	err := fn()
	if err == nil && s.FailStage == "commit" {
		err = ErrFakeDocumentAggregate
	}
	if err != nil {
		s.Docs.restore(docSnapshot)
		s.Tags.restore(tagSnapshot)
	}
	return err
}

func (s *FakeDocumentAggregateStore) fail(stage string) error {
	if s.FailStage == stage {
		return ErrFakeDocumentAggregate
	}
	return nil
}

func (s *FakeDocumentAggregateStore) CreateDocumentAggregate(ctx context.Context, d domain.Document, changes ports.DocumentAggregateChanges) (out domain.Document, err error) {
	err = s.transaction(func() error {
		if err := s.fail("document"); err != nil {
			return err
		}
		created, err := s.Docs.Create(ctx, d)
		if err != nil {
			return err
		}
		if err := s.fail("links"); err != nil {
			return err
		}
		if err := s.Docs.ReplaceLinks(ctx, created.ID, created.OwnerID, changes.Links); err != nil {
			return err
		}
		if changes.Tags != nil {
			if err := s.fail("tags"); err != nil {
				return err
			}
			tags, err := s.Tags.SetTags(ctx, created.OwnerID, domain.TaggableDocument, created.ID, *changes.Tags)
			if err != nil {
				return err
			}
			created.Tags = fakeTagSlugs(tags)
			if _, err := s.Docs.Update(ctx, created); err != nil {
				return err
			}
		}
		out = created
		return nil
	})
	return out, err
}

func (s *FakeDocumentAggregateStore) UpdateDocumentAggregate(ctx context.Context, ownerID, id string, mutate func(domain.Document) (domain.Document, ports.DocumentAggregateChanges, error)) (out domain.Document, err error) {
	err = s.transaction(func() error {
		current, err := s.Docs.Get(ctx, ownerID, id)
		if err != nil {
			return err
		}
		next, changes, err := mutate(current)
		if err != nil {
			return err
		}
		if next.ID != current.ID || next.OwnerID != current.OwnerID {
			return errors.New("fake document aggregate identity changed")
		}
		if err := s.fail("document"); err != nil {
			return err
		}
		updated, err := s.Docs.Update(ctx, next)
		if err != nil {
			return err
		}
		if err := s.fail("links"); err != nil {
			return err
		}
		if err := s.Docs.ReplaceLinks(ctx, updated.ID, updated.OwnerID, changes.Links); err != nil {
			return err
		}
		if changes.Tags != nil {
			if err := s.fail("tags"); err != nil {
				return err
			}
			tags, err := s.Tags.SetTags(ctx, updated.OwnerID, domain.TaggableDocument, updated.ID, *changes.Tags)
			if err != nil {
				return err
			}
			updated.Tags = fakeTagSlugs(tags)
			if _, err := s.Docs.Update(ctx, updated); err != nil {
				return err
			}
		}
		out = updated
		return nil
	})
	return out, err
}

func (s *FakeDocumentAggregateStore) UpsertDocumentAggregate(ctx context.Context, in ports.DocumentAggregateUpsert) (out domain.Document, err error) {
	err = s.transaction(func() error {
		if err := s.fail("document"); err != nil {
			return err
		}
		id, _, err := s.Docs.UpsertByPath(ctx, in.OwnerID, in.NodeID, in.Type, in.Path, in.Title, in.Body, in.Pinned, in.Archived, in.UpdatedByKind, in.UpdatedByRef)
		if err != nil {
			return err
		}
		if err := s.fail("links"); err != nil {
			return err
		}
		if err := s.Docs.ReplaceLinks(ctx, id, in.OwnerID, in.Changes.Links); err != nil {
			return err
		}
		var tags []domain.Tag
		if in.Changes.Tags != nil {
			if err := s.fail("tags"); err != nil {
				return err
			}
			tags, err = s.Tags.SetTags(ctx, in.OwnerID, domain.TaggableDocument, id, *in.Changes.Tags)
			if err != nil {
				return err
			}
		}
		if err := s.fail("curation"); err != nil {
			return err
		}
		if err := s.Docs.SetPinned(ctx, in.OwnerID, id, in.Pinned); err != nil {
			return err
		}
		if err := s.Docs.SetArchived(ctx, in.OwnerID, id, in.Archived); err != nil {
			return err
		}
		out, err = s.Docs.Get(ctx, in.OwnerID, id)
		if err != nil {
			return err
		}
		if in.Changes.Tags != nil {
			out.Tags = fakeTagSlugs(tags)
			if _, err := s.Docs.Update(ctx, out); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

func (s *FakeDocumentAggregateStore) DeleteDocumentAggregate(ctx context.Context, ownerID, id string) error {
	return s.transaction(func() error {
		if _, err := s.Docs.Get(ctx, ownerID, id); err != nil {
			return err
		}
		if err := s.fail("tags"); err != nil {
			return err
		}
		if err := s.Tags.ClearTaggable(ctx, ownerID, domain.TaggableDocument, id); err != nil {
			return err
		}
		if err := s.fail("document"); err != nil {
			return err
		}
		return s.Docs.Delete(ctx, ownerID, id)
	})
}

func fakeTagSlugs(tags []domain.Tag) []string {
	out := make([]string, len(tags))
	for i := range tags {
		out[i] = tags[i].Slug
	}
	return out
}

var _ ports.DocumentAggregateStore = (*FakeDocumentAggregateStore)(nil)
