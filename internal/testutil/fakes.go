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
	mu    sync.Mutex
	bySub map[string]domain.User
}

func NewFakeUserStore() *FakeUserStore { return &FakeUserStore{bySub: map[string]domain.User{}} }

func (s *FakeUserStore) UpsertBySub(_ context.Context, u domain.User) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bySub[u.OIDCSub] = u
	return u, nil
}

func (s *FakeUserStore) GetBySub(_ context.Context, sub string) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	mu sync.Mutex
	m  map[string]domain.Node // keyed by id
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
	for {
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
	if s.slugTaken(ownerID, parentID, n.Slug, id) {
		return domain.Node{}, ports.ErrNodeSlugTaken
	}
	n.ParentID = parentID
	s.m[id] = n
	return n, nil
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
	if ws.Stop == nil {
		for _, e := range s.m {
			if e.OwnerID == ws.OwnerID && e.Stop == nil {
				return domain.WorkSession{}, errors.New("fake: running session exists")
			}
		}
	}
	s.m[ws.ID] = ws
	return ws, nil
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
	e, ok := s.m[id]
	if !ok || e.OwnerID != ownerID || e.Stop != nil {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	e.Stop = &stop
	e.NodeID = nodeID
	s.m[id] = e
	return e, nil
}

// Update mirrors pgstore: it overwrites project/note/start/stop but NOT Tags
// (tags persist via FakeTagStore / the taggings junction). The record's existing
// Tags are preserved so stats/round-trip tests that seed Tags keep them.
func (s *FakeSessionStore) Update(_ context.Context, ownerID, id string, nodeID *string, note string, start time.Time, stop *time.Time) (domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok || e.OwnerID != ownerID {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	e.NodeID = nodeID
	e.Note = note
	e.Start = start
	if stop != nil {
		t := *stop
		e.Stop = &t
	} else {
		e.Stop = nil
	}
	s.m[id] = e
	return e, nil
}

func (s *FakeSessionStore) Delete(_ context.Context, ownerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok || e.OwnerID != ownerID {
		return ports.ErrSessionNotFound
	}
	delete(s.m, id)
	return nil
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
	sum := sha256.Sum256([]byte(d.Title + d.Body))
	return string(sum[:])
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
		if d.OwnerID == ownerID && matchesNode(d.NodeID, nodeID) && hasAllTags(d.Tags, tags) {
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
	// mirror pgstore: only title/body/tags/extra/updated_at are mutable
	existing.Title = d.Title
	existing.Body = d.Body
	existing.Tags = d.Tags
	existing.Extra = d.Extra
	existing.UpdatedAt = d.UpdatedAt
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
	s.mu.Lock()
	defer s.mu.Unlock()
	ql := strings.ToLower(q)
	var out []domain.SearchHit
	for _, d := range s.m {
		if d.OwnerID != ownerID || !matchesNode(d.NodeID, nodeID) || !hasAllTags(d.Tags, tags) {
			continue
		}
		hay := strings.ToLower(d.Title + " " + d.Body)
		idx := strings.Index(hay, ql)
		if ql == "" || idx < 0 {
			continue
		}
		out = append(out, domain.SearchHit{Document: d, Snippet: fakeSnippet(d.Title+" "+d.Body, idx, len(q))})
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].UpdatedAt.After(out[i].UpdatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
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
		out = append(out, ports.StaleDoc{Doc: d, Attempts: attempts})
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

func (s *FakeDocumentStore) ReplaceChunks(_ context.Context, docID, _ string, contents []string, embeddings [][]float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cs := make([]fakeChunk, len(contents))
	for i := range contents {
		cs[i] = fakeChunk{content: contents[i], emb: embeddings[i]}
	}
	s.chunks[docID] = cs
	if d, ok := s.m[docID]; ok {
		s.chunksHash[docID] = fakeDocHash(d)
	}
	delete(s.embedFail, docID)
	return nil
}

func (s *FakeDocumentStore) RecordEmbedFailure(_ context.Context, docID, _ string, attempts int, nextRetryAt time.Time, dead bool, lastErr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()
	var hits []domain.SemanticHit
	for _, d := range s.m {
		if d.OwnerID != ownerID || !matchesNode(d.NodeID, nodeID) || !hasAllTags(d.Tags, tags) {
			continue
		}
		best := -1.0
		bestContent := ""
		for _, c := range s.chunks[d.ID] {
			sim := cosine(query, c.emb)
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
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
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
	display map[string]string              // owner|slug -> display
	links   map[string]map[string]bool     // owner|type|id -> set of slugs
	idgen   int
}

func NewFakeTagStore() *FakeTagStore {
	return &FakeTagStore{display: map[string]string{}, links: map[string]map[string]bool{}}
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
