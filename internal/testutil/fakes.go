// Package testutil provides in-memory fakes for the ports, for use in tests.
package testutil

import (
	"context"
	"crypto/sha256"
	"errors"
	"math"
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

// FakeProjectStore is an in-memory ports.ProjectStore.
type FakeProjectStore struct {
	mu sync.Mutex
	m  map[string]domain.Project // keyed by id
}

func NewFakeProjectStore() *FakeProjectStore {
	return &FakeProjectStore{m: map[string]domain.Project{}}
}

func (s *FakeProjectStore) Create(_ context.Context, p domain.Project) (domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[p.ID] = p
	return p, nil
}

func (s *FakeProjectStore) List(_ context.Context, ownerID string) ([]domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Project
	for _, p := range s.m {
		if p.OwnerID == ownerID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *FakeProjectStore) Get(_ context.Context, ownerID, id string) (domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.m[id]
	if !ok || p.OwnerID != ownerID {
		return domain.Project{}, ports.ErrProjectNotFound
	}
	return p, nil
}

func (s *FakeProjectStore) SetRate(_ context.Context, ownerID, id string, rate *domain.Money) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.m[id]
	if !ok || p.OwnerID != ownerID {
		return ports.ErrProjectNotFound
	}
	p.Rate = rate
	s.m[id] = p
	return nil
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

func (s *FakeSessionStore) Stop(_ context.Context, ownerID, id string, projectID *string, stop time.Time) (domain.WorkSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[id]
	if !ok || e.OwnerID != ownerID || e.Stop != nil {
		return domain.WorkSession{}, ports.ErrSessionNotFound
	}
	e.Stop = &stop
	e.ProjectID = projectID
	s.m[id] = e
	return e, nil
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
// Create returns ErrDocumentExists on an (owner, coalesce(projectID,""), path) collision.
type FakeDocumentStore struct {
	mu    sync.Mutex
	m     map[string]domain.Document // keyed by id
	links map[string][]string        // srcDocID -> target paths
}

func NewFakeDocumentStore() *FakeDocumentStore {
	return &FakeDocumentStore{m: map[string]domain.Document{}, links: map[string][]string{}}
}

func docCollisionKey(ownerID string, projectID *string, path string) string {
	proj := ""
	if projectID != nil {
		proj = *projectID
	}
	return ownerID + "\x00" + proj + "\x00" + path
}

func (s *FakeDocumentStore) Create(_ context.Context, d domain.Document) (domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := docCollisionKey(d.OwnerID, d.ProjectID, d.Path)
	for _, existing := range s.m {
		if docCollisionKey(existing.OwnerID, existing.ProjectID, existing.Path) == want {
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

func (s *FakeDocumentStore) List(_ context.Context, ownerID string, tags ...string) ([]domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Document
	for _, d := range s.m {
		if d.OwnerID == ownerID && hasAllTags(d.Tags, tags) {
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

func (s *FakeDocumentStore) Search(_ context.Context, ownerID, q string, tags []string) ([]domain.SearchHit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ql := strings.ToLower(q)
	var out []domain.SearchHit
	for _, d := range s.m {
		if d.OwnerID != ownerID || !hasAllTags(d.Tags, tags) {
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

// FakeEmbedder returns deterministic unit vectors derived from a hash of each
// text — identical text yields an identical vector, so similarity *ordering* is
// reproducible without a real model. It does NOT model semantic nearness; tests
// assert wiring/ordering, not embedding quality. Set Err to simulate Ollama down.
type FakeEmbedder struct {
	Dim int
	Err error
}

func NewFakeEmbedder() *FakeEmbedder { return &FakeEmbedder{Dim: 768} }

func (f *FakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
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
