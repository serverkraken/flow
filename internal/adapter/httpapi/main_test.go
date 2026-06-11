package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
	"github.com/serverkraken/flow/internal/adapter/httpserver"
	"github.com/serverkraken/flow/internal/adapter/oidcserver"
	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/testutil/oidctest"
	"github.com/serverkraken/flow/internal/testutil/pgtest"
)

var pgTestStore *pgstore.Store

func TestMain(m *testing.M) {
	os.Exit(func() int {
		ctx := context.Background()
		dsn, terminate, err := pgtest.StartContainer(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pgtest:", err)
			return 1
		}
		defer terminate()
		s, err := pgstore.Open(ctx, dsn)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pgstore open:", err)
			return 1
		}
		defer s.Close()
		pgTestStore = s
		return m.Run()
	}())
}

// memTokens is a minimal in-memory TokenStore for test clients.
type memTokens struct {
	tok ports.Tokens
	ok  bool
}

func (m *memTokens) Get(_ string) (ports.Tokens, error) {
	if !m.ok {
		return ports.Tokens{}, ports.ErrTokenNotFound
	}
	return m.tok, nil
}
func (m *memTokens) Put(_ string, t ports.Tokens) error { m.tok, m.ok = t, true; return nil }
func (m *memTokens) Delete(_ string) error              { m.ok = false; return nil }

// testAPI holds everything a test needs.
type testAPI struct {
	Client    *httpapi.Client
	URL       string
	MintToken func(sub string) string
	Sub       string // dex static user sub
}

// newTestAPI starts a fresh httptest.Server with the full bearer-protected API
// wired against the shared pgTestStore and a per-test dex instance.
func newTestAPI(t *testing.T) *testAPI {
	t.Helper()
	ctx := context.Background()

	dex := oidctest.StartDex(t)

	prov, err := oidcserver.NewProvider(ctx, oidcserver.ProviderConfig{
		Issuers:           []string{dex.Issuer},
		AcceptedClientIDs: []string{dex.ClientID, dex.CLIClientID},
	})
	if err != nil {
		t.Fatalf("oidcserver.NewProvider: %v", err)
	}
	access := oidcserver.NewSubAllowlist([]string{dex.StaticUser.Sub})

	sessions := pgstore.NewSessions(pgTestStore)
	settings := pgstore.NewSettings(pgTestStore)
	active := pgstore.NewActiveSessions(pgTestStore, sessions, settings)
	projects := pgstore.NewProjects(pgTestStore)
	users := pgstore.NewUsers(pgTestStore)
	documents := pgstore.NewDocuments(pgTestStore)
	dayoffs := pgstore.NewDayOffs(pgTestStore)

	srv := httpserver.NewWithAuth(httpserver.AuthDeps{
		Provider: prov,
		Access:   access,
		Users:    users,
		Ready:    func() error { return nil },
		Meta: httpserver.MetaResponse{
			ServerVersion:    "9.9.9",
			MinClientVersion: "0.0.1",
		},
		WorktimeAPI: &httpserver.WorktimeAPIDeps{
			Sessions: sessions,
			Active:   active,
			Settings: settings,
		},
		ProjectsAPI: &httpserver.ProjectsAPIDeps{
			Projects: projects,
		},
		DocumentsAPI: &httpserver.DocumentsAPIDeps{
			Store: documents,
		},
		MiscAPI: &httpserver.DayOffsSettingsAPIDeps{
			DayOffs:  dayoffs,
			Settings: settings,
		},
	})

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	mintToken := func(_ string) string {
		// Mint via ROPC (dex password grant) — only works for StaticUser credentials.
		form := url.Values{}
		form.Set("grant_type", "password")
		form.Set("username", dex.StaticUser.Email)
		form.Set("password", dex.StaticUser.Password)
		form.Set("scope", "openid email profile")
		req, _ := http.NewRequest(http.MethodPost, dex.Issuer+"/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth(dex.ClientID, dex.ClientSecret)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("ROPC token request: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode/100 != 2 {
			t.Skipf("ROPC unavailable on dex (status %d) — skip", resp.StatusCode)
		}
		var raw struct {
			IDToken string `json:"id_token"`
		}
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("decode token response: %v", err)
		}
		return raw.IDToken
	}

	tok := mintToken(dex.StaticUser.Sub)
	store := &memTokens{tok: ports.Tokens{AccessToken: tok}, ok: true}

	cli := httpapi.New(httpapi.Config{
		BaseURL: ts.URL,
		Tokens:  store,
		Slot:    "test",
		Version: "9.9.9",
	})

	return &testAPI{
		Client:    cli,
		URL:       ts.URL,
		MintToken: mintToken,
		Sub:       dex.StaticUser.Sub,
	}
}
