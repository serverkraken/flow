package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/adapter/oidcclient"
	"github.com/serverkraken/flow/internal/adapter/sqliteclient"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
	"github.com/spf13/cobra"
)

// loginConfig captures everything runLogin needs. Lifted to a struct so tests
// can inject HTTP server, store, and clock-replacements.
type loginConfig struct {
	ClientID               string
	ClientSecret           string
	DeviceAuthorizationURL string
	TokenURL               string
	HTTPClient             *http.Client
	PollOverride           time.Duration
	Store                  ports.TokenStore
	SlotName               string
	Out                    io.Writer
	In                     io.Reader // stdin for the adoption prompt; nil → skip prompt
	OpenBrowser            func(string) error
	// First-login adoption fields (all optional; omitting them disables the prompt).
	CacheDBPath string // path to the SQLite cache DB; empty → skip adoption
	LocalSub    string // FLOW_LOCAL_USER_SUB default "local"
}

func runLogin(ctx context.Context, c loginConfig) error {
	df := oidcclient.NewDeviceFlow(oidcclient.Config{
		ClientID:               c.ClientID,
		ClientSecret:           c.ClientSecret,
		DeviceAuthorizationURL: c.DeviceAuthorizationURL,
		TokenURL:               c.TokenURL,
		Scopes:                 []string{"openid", "profile", "email", "offline_access"},
		HTTPClient:             c.HTTPClient,
		PollIntervalOverride:   c.PollOverride,
	})
	codes, err := df.Init(ctx)
	if err != nil {
		return fmt.Errorf("device init: %w", err)
	}
	_, _ = fmt.Fprintf(c.Out, "\nUm flow zu autorisieren:\n  1. öffne %s\n  2. gib den Code ein: %s\n\n",
		codes.VerificationURI, codes.UserCode)
	if c.OpenBrowser != nil {
		u := codes.VerificationURIComplete
		if u == "" {
			u = codes.VerificationURI
		}
		_ = c.OpenBrowser(u)
	}
	tok, err := df.PollForToken(ctx, codes)
	if err != nil {
		return fmt.Errorf("device poll: %w", err)
	}
	if err := c.Store.Put(c.SlotName, tok); err != nil {
		return fmt.Errorf("token store: %w", err)
	}
	_, _ = fmt.Fprintln(c.Out, "✓ Login erfolgreich, Token im Keychain gespeichert.")

	// First-login adoption: offer to carry the offline `local` profile into
	// this OIDC identity by re-labelling the local user row (keeping its id so
	// all owned rows stay owned). This is best-effort — any error here must not
	// fail the login that just succeeded.
	tryAdoptLocalProfile(c, tok)

	return nil
}

// tryAdoptLocalProfile checks whether there is an offline `local` profile with
// owned data, and if so prompts the user to carry it into the freshly-minted
// OIDC identity. All errors are swallowed (printed as a warning at most) so
// that a DB or prompt failure never propagates back through runLogin.
func tryAdoptLocalProfile(c loginConfig, tok ports.Tokens) {
	if c.CacheDBPath == "" || c.In == nil {
		return // adoption not configured or stdin not available
	}
	rawToken := tok.IDToken
	if rawToken == "" {
		rawToken = tok.AccessToken
	}
	claims, err := oidcclient.ClaimsFromToken(rawToken)
	if err != nil {
		return // can't decode identity; skip silently
	}

	store, err := sqliteclient.Open(c.CacheDBPath)
	if err != nil {
		return // DB not accessible; skip silently
	}
	defer func() { _ = store.Close() }()

	users := sqliteclient.NewUsers(store)
	localSub := c.LocalSub
	if localSub == "" {
		localSub = "local"
	}
	local, err := users.GetBySub(localSub)
	if err != nil {
		return // no local profile at all
	}
	n, err := users.CountOwnedRows(local.ID)
	if err != nil || n == 0 {
		return // nothing to carry over
	}

	display := claims.Email
	if display == "" {
		display = claims.Sub
	}
	if !promptYesNo(c.Out, c.In, fmt.Sprintf(
		"%d lokale Projekte/Sessions unter dem Offline-Profil gefunden. Unter %s übernehmen? [y/N] ",
		n, display,
	)) {
		return
	}

	id := usecase.NewIdentity(users, localSub)
	adopted, carried, aerr := id.AdoptLocalDataIfFirstLogin(claims.Sub, claims.Email, claims.Name)
	if aerr != nil {
		_, _ = fmt.Fprintf(c.Out, "⚠ Übernahme fehlgeschlagen: %v\n", aerr)
		return
	}
	if adopted {
		_, _ = fmt.Fprintf(c.Out, "✓ %d Einträge unter %s übernommen.\n", carried, claims.Sub)
	}
}

// promptYesNo writes q to out and reads a line from in. Returns true only for
// explicit "y", "yes", "j", or "ja" (case-insensitive). Any read error → false.
func promptYesNo(out io.Writer, in io.Reader, q string) bool {
	_, _ = fmt.Fprint(out, q)
	line, _ := bufio.NewReader(in).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes" || line == "j" || line == "ja"
}

// openBrowserDefault is the production browser-opener used by the CLI.
// Tests substitute a no-op.
func openBrowserDefault(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	return nil
}

// slotNameFor derives a per-server slot so a user can log into multiple
// flow-servers (dev / prod / homelab) without collisions.
func slotNameFor(serverURL string) string {
	return "tokens:" + serverURL
}

func envOrDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// loginCmdOptions holds wiring values threaded in from main.go so the cobra
// RunE closure doesn't need to re-resolve env vars that main already computed.
type loginCmdOptions struct {
	CacheDBPath string // resolved $FLOW_CACHE_DB / XDG path; empty disables adoption
	LocalSub    string // $FLOW_LOCAL_USER_SUB; empty → "local"
}

func newLoginCmd(opts loginCmdOptions) *cobra.Command {
	var (
		serverURL string
		clientID  string
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Beim flow-server anmelden (OIDC Device-Flow)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			deviceURL, tokenURL, err := oidcclient.ResolveEndpoints(cmd.Context(), serverURL, http.DefaultClient)
			if err != nil {
				return err
			}
			if deviceURL == "" {
				return fmt.Errorf("oidc/config: IdP exposes no device_authorization_endpoint")
			}
			return runLogin(cmd.Context(), loginConfig{
				ClientID:               clientID,
				DeviceAuthorizationURL: deviceURL,
				TokenURL:               tokenURL,
				HTTPClient:             http.DefaultClient,
				Store:                  keyringadapter.New(),
				SlotName:               slotNameFor(serverURL),
				Out:                    cmd.OutOrStdout(),
				In:                     cmd.InOrStdin(),
				OpenBrowser:            openBrowserDefault,
				CacheDBPath:            opts.CacheDBPath,
				LocalSub:               opts.LocalSub,
			})
		},
	}
	cmd.Flags().StringVar(&serverURL, "server", envOrDefault("FLOW_SERVER_URL", "http://localhost:8080"), "flow-server base URL")
	cmd.Flags().StringVar(&clientID, "client-id", envOrDefault("FLOW_OIDC_CLIENT_ID", "flow-cli"), "OIDC client id")
	return cmd
}
