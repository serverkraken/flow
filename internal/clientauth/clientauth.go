// Package clientauth builds an authenticated flow apiclient from the stored
// OIDC token (or FLOW_TOKEN), shared by the flow CLI and flow-mcp.
package clientauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/adapter/oidcdevice"
	"github.com/serverkraken/flow/internal/adapter/tokenstore"
	"github.com/serverkraken/flow/internal/clientconfig"
	"github.com/serverkraken/flow/internal/ports"
)

const refreshTimeout = 30 * time.Second

// ErrNotLoggedIn marks "no usable stored credential" — the caller should prompt
// the user to run `flow login`.
var ErrNotLoggedIn = errors.New("not logged in — run `flow login`")

type refreshTokenFunc func(context.Context, ports.Token) (*oauth2.Token, error)

// coordinatedSource keeps the valid-token path process-local and coordinates
// every expired-token transition through the store's cross-process lock.
type coordinatedSource struct {
	baseCtx context.Context
	cfg     clientconfig.Config
	store   ports.TokenStore

	mu       sync.Mutex
	last     ports.Token
	terminal error
	refresh  refreshTokenFunc
}

func newCoordinatedSource(
	ctx context.Context,
	cfg clientconfig.Config,
	store ports.TokenStore,
	loaded ports.Token,
) *coordinatedSource {
	return &coordinatedSource{baseCtx: ctx, cfg: cfg, store: store, last: loaded}
}

// Token implements oauth2.TokenSource for direct users. HTTP requests use
// tokenContext through coordinatedTransport so their own deadline wins.
func (s *coordinatedSource) Token() (*oauth2.Token, error) {
	return s.tokenContext(s.baseCtx)
}

func (s *coordinatedSource) tokenContext(callCtx context.Context) (*oauth2.Token, error) {
	s.mu.Lock()
	if s.terminal != nil {
		err := s.terminal
		s.mu.Unlock()
		return nil, err
	}
	if cached := oauthToken(s.last); cached.Valid() {
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	// The caller bounds lock waiting and every pre-flight check. Once the OAuth
	// request starts, however, Authentik may already have irreversibly rotated
	// the refresh token. That mutation must run through persistence even if the
	// originating HTTP request times out meanwhile.
	ctx, cancel := context.WithTimeout(mergeContext(callCtx, s.baseCtx), refreshTimeout)
	defer cancel()
	var result *oauth2.Token
	err := s.store.WithLock(ctx, func(session ports.TokenStoreSession) error {
		current, ok, err := session.Load()
		if err != nil {
			return fmt.Errorf("clientauth: load token after lock: %w", err)
		}
		if !ok || current.AccessToken == "" {
			s.setLast(ports.Token{})
			return ErrNotLoggedIn
		}
		s.setLast(current)
		if stored := oauthToken(current); stored.Valid() {
			result = stored
			return nil
		}
		if s.cfg.OIDCIssuer == "" && s.refresh == nil {
			return fmt.Errorf("access token expired and FLOW_OIDC_ISSUER is not set: %w", ErrNotLoggedIn)
		}

		if err := ctx.Err(); err != nil {
			return err
		}
		refreshCtx, refreshCancel := context.WithTimeout(context.WithoutCancel(ctx), refreshTimeout)
		defer refreshCancel()
		result, err = s.refreshLocked(refreshCtx, session, current)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *coordinatedSource) refreshLocked(
	ctx context.Context,
	session ports.TokenStoreSession,
	current ports.Token,
) (*oauth2.Token, error) {
	tok, err := s.refreshOne(ctx, current)
	if err == nil {
		return s.persist(session, current, tok)
	}
	if !isInvalidGrant(err) {
		return nil, err
	}

	// A non-cooperating old binary may have written a successor while this
	// process refreshed. Re-read even though all new binaries hold the lock.
	latest, ok, loadErr := session.Load()
	if loadErr != nil {
		return nil, fmt.Errorf("clientauth: refresh token rejected (%v), reload token: %w", err, loadErr)
	}
	if !ok || latest.RefreshToken == "" || latest.RefreshToken == current.RefreshToken {
		if clearErr := session.Clear(); clearErr != nil {
			return nil, fmt.Errorf(
				"clientauth: refresh token rejected (%v), clear stored token: %w",
				err, clearErr,
			)
		}
		terminal := fmt.Errorf("clientauth: refresh token rejected (%v): %w", err, ErrNotLoggedIn)
		s.mu.Lock()
		s.last = ports.Token{}
		s.terminal = terminal
		s.mu.Unlock()
		return nil, terminal
	}

	// Never clear a successor discovered while handling a stale rejection.
	// Adopt it, use it directly when valid, or attempt exactly one refresh.
	s.setLast(latest)
	if stored := oauthToken(latest); stored.Valid() {
		return stored, nil
	}
	tok, err = s.refreshOne(ctx, latest)
	if err != nil {
		return nil, fmt.Errorf("clientauth: refresh newer stored token: %w", err)
	}
	return s.persist(session, latest, tok)
}

func (s *coordinatedSource) refreshOne(ctx context.Context, current ports.Token) (*oauth2.Token, error) {
	if s.refresh != nil {
		return s.refresh(ctx, current)
	}
	flow, err := oidcdevice.New(ctx, s.cfg.OIDCIssuer, s.cfg.CliClientID)
	if err != nil {
		return nil, err
	}
	return flow.TokenSource(ctx, oauthToken(current)).Token()
}

func (s *coordinatedSource) persist(
	session ports.TokenStoreSession,
	current ports.Token,
	tok *oauth2.Token,
) (*oauth2.Token, error) {
	if tok == nil {
		return nil, errors.New("clientauth: token source returned a nil token")
	}
	refresh := tok.RefreshToken
	if refresh == "" {
		refresh = current.RefreshToken
	}
	next := ports.Token{AccessToken: tok.AccessToken, RefreshToken: refresh, Expiry: tok.Expiry}
	if !sameToken(current, next) {
		if err := session.Save(next); err != nil {
			return nil, fmt.Errorf("clientauth: persist token: %w", err)
		}
	}
	s.setLast(next)
	return oauthToken(next), nil
}

func (s *coordinatedSource) setLast(token ports.Token) {
	s.mu.Lock()
	s.last = token
	s.mu.Unlock()
}

func oauthToken(t ports.Token) *oauth2.Token {
	return &oauth2.Token{AccessToken: t.AccessToken, RefreshToken: t.RefreshToken, Expiry: t.Expiry}
}

func sameToken(a, b ports.Token) bool {
	return a.AccessToken == b.AccessToken &&
		a.RefreshToken == b.RefreshToken &&
		a.Expiry.Equal(b.Expiry)
}

func isInvalidGrant(err error) bool {
	var retrieveErr *oauth2.RetrieveError
	return errors.As(err, &retrieveErr) && retrieveErr.ErrorCode == "invalid_grant"
}

// mergeContext keeps process-lifetime values used during client construction
// while cancellation and deadlines come from the active HTTP request.
type mergedContext struct {
	context.Context
	values context.Context
}

func (c mergedContext) Value(key any) any {
	if value := c.Context.Value(key); value != nil {
		return value
	}
	return c.values.Value(key)
}

func mergeContext(callCtx, values context.Context) context.Context {
	if callCtx == nil {
		callCtx = context.Background()
	}
	if values == nil {
		values = context.Background()
	}
	return mergedContext{Context: callCtx, values: values}
}

type coordinatedTransport struct {
	source *coordinatedSource
	base   http.RoundTripper
}

func (t coordinatedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tok, err := t.source.tokenContext(req.Context())
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	tok.SetAuthHeader(clone)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

// Client builds an authenticated apiclient. FLOW_TOKEN wins as a static,
// completely store- and lock-free bearer override.
func Client(ctx context.Context) (*apiclient.Client, error) {
	return client(ctx, os.Getenv, tokenstore.Open)
}

func client(
	ctx context.Context,
	getenv func(string) string,
	openStore func() ports.TokenStore,
) (*apiclient.Client, error) {
	cfg := clientconfig.Load(getenv)
	if token := getenv("FLOW_TOKEN"); token != "" {
		if cfg.InsecureTLS {
			return apiclient.NewInsecure(cfg.ServerURL, token), nil
		}
		return apiclient.New(cfg.ServerURL, token), nil
	}
	store := openStore()
	var loaded ports.Token
	var ok bool
	loadCtx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()
	if err := store.WithLock(loadCtx, func(session ports.TokenStoreSession) error {
		var err error
		loaded, ok, err = session.Load()
		return err
	}); err != nil {
		return nil, fmt.Errorf("clientauth: load stored token: %w", err)
	}
	if !ok || loaded.AccessToken == "" {
		return nil, ErrNotLoggedIn
	}
	source := newCoordinatedSource(ctx, cfg, store, loaded)
	base := http.DefaultTransport
	if cfg.InsecureTLS {
		base = apiclient.InsecureBase()
	}
	return apiclient.NewTransport(cfg.ServerURL, coordinatedTransport{source: source, base: base}), nil
}
