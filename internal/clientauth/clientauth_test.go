package clientauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/serverkraken/flow/internal/clientconfig"
	"github.com/serverkraken/flow/internal/ports"
	"golang.org/x/oauth2"
)

type memoryStore struct {
	gate chan struct{}

	token ports.Token
	ok    bool

	lockCalls  int
	loadCalls  int
	saveCalls  int
	clearCalls int
	lockErr    error
	loadErr    error
	saveErr    error
	clearErr   error
	onLoad     func(*memoryStore, int) (ports.Token, bool, error)
}

func newMemoryStore(token ports.Token) *memoryStore {
	return &memoryStore{gate: make(chan struct{}, 1), token: token, ok: token.AccessToken != ""}
}

func (m *memoryStore) WithLock(ctx context.Context, fn func(ports.TokenStoreSession) error) error {
	select {
	case m.gate <- struct{}{}:
		defer func() { <-m.gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	m.lockCalls++
	if m.lockErr != nil {
		return m.lockErr
	}
	return fn(m)
}

func (m *memoryStore) Load() (ports.Token, bool, error) {
	m.loadCalls++
	if m.onLoad != nil {
		return m.onLoad(m, m.loadCalls)
	}
	return m.token, m.ok, m.loadErr
}

func (m *memoryStore) Save(token ports.Token) error {
	m.saveCalls++
	if m.saveErr != nil {
		return m.saveErr
	}
	m.token, m.ok = token, true
	return nil
}

func (m *memoryStore) Clear() error {
	m.clearCalls++
	if m.clearErr != nil {
		return m.clearErr
	}
	m.token, m.ok = ports.Token{}, false
	return nil
}

func (m *memoryStore) snapshot() (ports.Token, bool, int, int, int, int) {
	m.gate <- struct{}{}
	defer func() { <-m.gate }()
	return m.token, m.ok, m.lockCalls, m.loadCalls, m.saveCalls, m.clearCalls
}

func expired(access, refresh string) ports.Token {
	return ports.Token{AccessToken: access, RefreshToken: refresh, Expiry: time.Now().Add(-time.Hour)}
}

func valid(access, refresh string) ports.Token {
	return ports.Token{AccessToken: access, RefreshToken: refresh, Expiry: time.Now().Add(time.Hour)}
}

func invalidGrant() error {
	return &oauth2.RetrieveError{
		Response:  &http.Response{StatusCode: http.StatusBadRequest},
		ErrorCode: "invalid_grant",
	}
}

func TestCoordinatedSourceInvalidCurrentGrantClearsAndStopsRetrying(t *testing.T) {
	old := expired("stale", "revoked")
	store := newMemoryStore(old)
	calls := 0
	source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, old)
	source.refresh = func(context.Context, ports.Token) (*oauth2.Token, error) {
		calls++
		return nil, invalidGrant()
	}

	for i := 0; i < 2; i++ {
		_, err := source.Token()
		if !errors.Is(err, ErrNotLoggedIn) {
			t.Fatalf("Token call %d error = %v, want ErrNotLoggedIn", i+1, err)
		}
	}
	stored, ok, _, _, _, clears := store.snapshot()
	if calls != 1 || clears != 1 || ok || stored != (ports.Token{}) {
		t.Fatalf("calls=%d clears=%d ok=%v stored=%+v", calls, clears, ok, stored)
	}
}

func TestCoordinatedSourceStaleInvalidGrantAdoptsSuccessor(t *testing.T) {
	old := expired("old-access", "old-refresh")
	successor := valid("new-access", "new-refresh")
	store := newMemoryStore(old)
	store.onLoad = func(m *memoryStore, call int) (ports.Token, bool, error) {
		if call == 2 {
			m.token, m.ok = successor, true
		}
		return m.token, m.ok, nil
	}
	source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, old)
	source.refresh = func(context.Context, ports.Token) (*oauth2.Token, error) { return nil, invalidGrant() }

	tok, err := source.Token()
	if err != nil {
		t.Fatal(err)
	}
	stored, ok, _, _, saves, clears := store.snapshot()
	if tok.AccessToken != successor.AccessToken || !ok || !sameToken(stored, successor) || saves != 0 || clears != 0 {
		t.Fatalf("tok=%+v stored=%+v ok=%v saves=%d clears=%d", tok, stored, ok, saves, clears)
	}
}

func TestCoordinatedSourceRefreshesExpiredSuccessorAtMostOnce(t *testing.T) {
	old := expired("old-access", "old-refresh")
	successor := expired("next-access", "next-refresh")
	store := newMemoryStore(old)
	store.onLoad = func(m *memoryStore, call int) (ports.Token, bool, error) {
		if call == 2 {
			m.token, m.ok = successor, true
		}
		return m.token, m.ok, nil
	}
	var attempts []string
	source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, old)
	source.refresh = func(_ context.Context, token ports.Token) (*oauth2.Token, error) {
		attempts = append(attempts, token.RefreshToken)
		if token.RefreshToken == old.RefreshToken {
			return nil, invalidGrant()
		}
		return &oauth2.Token{AccessToken: "final-access", RefreshToken: "final-refresh", Expiry: time.Now().Add(time.Hour)}, nil
	}

	tok, err := source.Token()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(attempts, ","); got != "old-refresh,next-refresh" {
		t.Fatalf("refresh attempts = %q", got)
	}
	stored, _, _, _, saves, clears := store.snapshot()
	if tok.AccessToken != "final-access" || stored.RefreshToken != "final-refresh" || saves != 1 || clears != 0 {
		t.Fatalf("tok=%+v stored=%+v saves=%d clears=%d", tok, stored, saves, clears)
	}
}

func TestCoordinatedSourceNeverClearsRejectedSuccessorDuringStaleRecovery(t *testing.T) {
	old := expired("old-access", "old-refresh")
	successor := expired("next-access", "next-refresh")
	store := newMemoryStore(old)
	store.onLoad = func(m *memoryStore, call int) (ports.Token, bool, error) {
		if call == 2 {
			m.token, m.ok = successor, true
		}
		return m.token, m.ok, nil
	}
	attempts := 0
	source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, old)
	source.refresh = func(context.Context, ports.Token) (*oauth2.Token, error) {
		attempts++
		return nil, invalidGrant()
	}

	if _, err := source.Token(); err == nil || errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("error = %v, want retryable stale-recovery error", err)
	}
	stored, ok, _, _, _, clears := store.snapshot()
	if attempts != 2 || !ok || !sameToken(stored, successor) || clears != 0 {
		t.Fatalf("attempts=%d stored=%+v ok=%v clears=%d", attempts, stored, ok, clears)
	}
}

func TestCoordinatedSourceTransientFailuresRemainRetryableAndPreserveStore(t *testing.T) {
	old := expired("old", "still-valid")
	store := newMemoryStore(old)
	source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, old)
	calls := 0
	source.refresh = func(context.Context, ports.Token) (*oauth2.Token, error) {
		calls++
		return nil, &oauth2.RetrieveError{
			Response:  &http.Response{StatusCode: http.StatusServiceUnavailable},
			ErrorCode: "temporarily_unavailable",
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := source.Token(); err == nil || errors.Is(err, ErrNotLoggedIn) {
			t.Fatalf("Token call %d error = %v", i+1, err)
		}
	}
	stored, ok, _, _, saves, clears := store.snapshot()
	if calls != 2 || !ok || !sameToken(stored, old) || saves != 0 || clears != 0 {
		t.Fatalf("calls=%d stored=%+v ok=%v saves=%d clears=%d", calls, stored, ok, saves, clears)
	}
}

func TestCoordinatedSourceClearFailureRemainsRetryable(t *testing.T) {
	old := expired("old", "revoked")
	store := newMemoryStore(old)
	store.clearErr = errors.New("keyring unavailable")
	source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, old)
	source.refresh = func(context.Context, ports.Token) (*oauth2.Token, error) { return nil, invalidGrant() }
	for i := 0; i < 2; i++ {
		if _, err := source.Token(); err == nil || errors.Is(err, ErrNotLoggedIn) {
			t.Fatalf("Token call %d error = %v, want retryable clear error", i+1, err)
		}
	}
	stored, ok, _, _, _, clears := store.snapshot()
	if !ok || !sameToken(stored, old) || clears != 2 {
		t.Fatalf("stored=%+v ok=%v clears=%d", stored, ok, clears)
	}
}

func TestCoordinatedSourceLockAndLoadFailuresRemainRetryable(t *testing.T) {
	old := expired("old", "refresh")
	for name, configure := range map[string]func(*memoryStore){
		"lock": func(store *memoryStore) { store.lockErr = errors.New("lock unavailable") },
		"load": func(store *memoryStore) { store.loadErr = errors.New("keyring unavailable") },
	} {
		t.Run(name, func(t *testing.T) {
			store := newMemoryStore(old)
			configure(store)
			source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, old)
			for i := 0; i < 2; i++ {
				if _, err := source.Token(); err == nil || errors.Is(err, ErrNotLoggedIn) {
					t.Fatalf("Token call %d error = %v", i+1, err)
				}
			}
		})
	}
}

func TestCoordinatedSourcePersistsRefreshOrExpiryOnlyChanges(t *testing.T) {
	for name, refreshed := range map[string]*oauth2.Token{
		"refresh": {AccessToken: "same-access", RefreshToken: "new-refresh", Expiry: time.Unix(20, 0)},
		"expiry":  {AccessToken: "same-access", RefreshToken: "same-refresh", Expiry: time.Unix(30, 0)},
	} {
		t.Run(name, func(t *testing.T) {
			old := ports.Token{AccessToken: "same-access", RefreshToken: "same-refresh", Expiry: time.Unix(10, 0)}
			store := newMemoryStore(old)
			source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, old)
			source.refresh = func(context.Context, ports.Token) (*oauth2.Token, error) { return refreshed, nil }
			if _, err := source.Token(); err != nil {
				t.Fatal(err)
			}
			stored, _, _, _, saves, _ := store.snapshot()
			if saves != 1 || stored.RefreshToken != refreshed.RefreshToken || !stored.Expiry.Equal(refreshed.Expiry) {
				t.Fatalf("stored=%+v saves=%d", stored, saves)
			}
		})
	}
}

func TestCoordinatedSourcePreservesOmittedRefreshToken(t *testing.T) {
	old := expired("old-access", "keep-refresh")
	store := newMemoryStore(old)
	source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, old)
	source.refresh = func(context.Context, ports.Token) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "new-access", Expiry: time.Now().Add(time.Hour)}, nil
	}
	tok, err := source.Token()
	if err != nil {
		t.Fatal(err)
	}
	if tok.RefreshToken != "keep-refresh" {
		t.Fatalf("refresh token = %q", tok.RefreshToken)
	}
}

func TestCoordinatedSourceValidFastPathDoesNotTouchStore(t *testing.T) {
	current := valid("valid", "refresh")
	store := newMemoryStore(current)
	source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, current)
	tok, err := source.Token()
	if err != nil || tok.AccessToken != "valid" {
		t.Fatalf("Token = %+v, %v", tok, err)
	}
	_, _, locks, loads, _, _ := store.snapshot()
	if locks != 0 || loads != 0 {
		t.Fatalf("fast path used store: locks=%d loads=%d", locks, loads)
	}
}

func TestCoordinatedSourceReloadsValidTokenWithoutIssuer(t *testing.T) {
	local := expired("old", "old-refresh")
	stored := valid("fresh", "fresh-refresh")
	store := newMemoryStore(stored)
	source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, local)
	tok, err := source.Token()
	if err != nil || tok.AccessToken != "fresh" {
		t.Fatalf("Token = %+v, %v", tok, err)
	}
}

func TestCoordinatedSourceExpiredWithoutIssuerIsActionable(t *testing.T) {
	old := expired("old", "refresh")
	store := newMemoryStore(old)
	source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, old)
	_, err := source.Token()
	if err == nil || !strings.Contains(err.Error(), "FLOW_OIDC_ISSUER") {
		t.Fatalf("error = %v", err)
	}
}

func TestTwoSourcesShareOneRotatingRefresh(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                        server.URL,
			"authorization_endpoint":        server.URL + "/auth",
			"token_endpoint":                server.URL + "/token",
			"device_authorization_endpoint": server.URL + "/device/code",
			"jwks_uri":                      server.URL + "/jwks",
		})
	})
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	var once sync.Once
	var refreshRequests atomic.Int32
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		refreshRequests.Add(1)
		once.Do(func() { close(refreshStarted) })
		<-releaseRefresh
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" {
			t.Errorf("unexpected refresh form: %v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})

	old := expired("old-access", "old-refresh")
	store := newMemoryStore(old)
	ctx := oidc.InsecureIssuerURLContext(context.Background(), server.URL)
	cfg := clientconfig.Config{OIDCIssuer: server.URL, CliClientID: "flow-cli"}
	first := newCoordinatedSource(ctx, cfg, store, old)
	second := newCoordinatedSource(ctx, cfg, store, old)
	type result struct {
		token *oauth2.Token
		err   error
	}
	results := make(chan result, 2)
	go func() { token, err := first.Token(); results <- result{token, err} }()
	<-refreshStarted
	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		token, err := second.Token()
		results <- result{token, err}
	}()
	<-secondStarted
	close(releaseRefresh)

	for i := 0; i < 2; i++ {
		got := <-results
		if got.err != nil || got.token.AccessToken != "new-access" || got.token.RefreshToken != "new-refresh" {
			t.Fatalf("result = %+v, %v", got.token, got.err)
		}
	}
	stored, ok, _, _, saves, clears := store.snapshot()
	if refreshRequests.Load() != 1 || !ok || stored.AccessToken != "new-access" || stored.RefreshToken != "new-refresh" || saves != 1 || clears != 0 {
		t.Fatalf("requests=%d stored=%+v ok=%v saves=%d clears=%d", refreshRequests.Load(), stored, ok, saves, clears)
	}
}

func TestRefreshCompletesAndPersistsAfterRequestDeadline(t *testing.T) {
	old := expired("old", "refresh")
	store := newMemoryStore(old)
	source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, old)
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	source.refresh = func(ctx context.Context, _ ports.Token) (*oauth2.Token, error) {
		close(refreshStarted)
		<-releaseRefresh
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return &oauth2.Token{
			AccessToken:  "fresh",
			RefreshToken: "successor",
			Expiry:       time.Now().Add(time.Hour),
		}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	type result struct {
		token *oauth2.Token
		err   error
	}
	done := make(chan result, 1)
	go func() {
		token, err := source.tokenContext(ctx)
		done <- result{token: token, err: err}
	}()
	<-refreshStarted
	<-ctx.Done()
	select {
	case got := <-done:
		close(releaseRefresh)
		t.Fatalf("refresh returned at the request deadline: token=%+v err=%v", got.token, got.err)
	default:
	}
	close(releaseRefresh)
	got := <-done
	if got.err != nil || got.token.AccessToken != "fresh" || got.token.RefreshToken != "successor" {
		t.Fatalf("result = %+v, %v", got.token, got.err)
	}
	stored, ok, _, _, saves, clears := store.snapshot()
	if !ok || stored.AccessToken != "fresh" || stored.RefreshToken != "successor" || saves != 1 || clears != 0 {
		t.Fatalf("stored=%+v ok=%v saves=%d clears=%d", stored, ok, saves, clears)
	}
}

func TestCanceledRequestBeforeRefreshDoesNotMutate(t *testing.T) {
	old := expired("old", "refresh")
	store := newMemoryStore(old)
	source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, old)
	refreshCalls := 0
	source.refresh = func(context.Context, ports.Token) (*oauth2.Token, error) {
		refreshCalls++
		return nil, errors.New("must not refresh")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := source.tokenContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	stored, ok, _, _, saves, clears := store.snapshot()
	if refreshCalls != 0 || !ok || !sameToken(stored, old) || saves != 0 || clears != 0 {
		t.Fatalf("refreshCalls=%d stored=%+v ok=%v saves=%d clears=%d", refreshCalls, stored, ok, saves, clears)
	}
}

func TestSameSourceWaiterHonorsRequestDeadline(t *testing.T) {
	old := expired("old", "refresh")
	store := newMemoryStore(old)
	source := newCoordinatedSource(context.Background(), clientconfig.Config{}, store, old)
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	var once sync.Once
	source.refresh = func(context.Context, ports.Token) (*oauth2.Token, error) {
		once.Do(func() { close(refreshStarted) })
		<-releaseRefresh
		return &oauth2.Token{AccessToken: "fresh", RefreshToken: "next", Expiry: time.Now().Add(time.Hour)}, nil
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := source.Token()
		firstDone <- err
	}()
	<-refreshStarted

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	secondDone := make(chan error, 1)
	go func() {
		_, err := source.tokenContext(ctx)
		secondDone <- err
	}()
	select {
	case err := <-secondDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			close(releaseRefresh)
			t.Fatalf("waiter error = %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		close(releaseRefresh)
		<-firstDone
		t.Fatal("same-source waiter ignored its request deadline")
	}
	close(releaseRefresh)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestClientFLOWTOKENDoesNotOpenStore(t *testing.T) {
	opened := false
	getenv := func(key string) string {
		return map[string]string{
			"FLOW_TOKEN":      "static",
			"FLOW_SERVER_URL": "https://flow.example",
		}[key]
	}
	got, err := client(context.Background(), getenv, func() ports.TokenStore {
		opened = true
		return nil
	})
	if err != nil || got == nil {
		t.Fatalf("client = %v, %v", got, err)
	}
	if opened {
		t.Fatal("FLOW_TOKEN path opened the persistent store")
	}
}

func TestClientInitialLoadUsesLock(t *testing.T) {
	store := newMemoryStore(valid("access", "refresh"))
	getenv := func(key string) string {
		if key == "FLOW_SERVER_URL" {
			return "https://flow.example"
		}
		return ""
	}
	got, err := client(context.Background(), getenv, func() ports.TokenStore { return store })
	if err != nil || got == nil {
		t.Fatalf("client = %v, %v", got, err)
	}
	_, _, locks, loads, _, _ := store.snapshot()
	if locks != 1 || loads != 1 {
		t.Fatalf("initial load locks=%d loads=%d", locks, loads)
	}
}
