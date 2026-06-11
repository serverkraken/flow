package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/adapter/oidcclient"
	"github.com/serverkraken/flow/internal/ports"
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
	OpenBrowser            func(string) error
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
	return nil
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

func newLoginCmd() *cobra.Command {
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
				OpenBrowser:            openBrowserDefault,
			})
		},
	}
	cmd.Flags().StringVar(&serverURL, "server", envOrDefault("FLOW_SERVER_URL", "http://localhost:8080"), "flow-server base URL")
	cmd.Flags().StringVar(&clientID, "client-id", envOrDefault("FLOW_OIDC_CLIENT_ID", "flow-cli"), "OIDC client id")
	return cmd
}
