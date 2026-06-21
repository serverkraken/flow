package main

import (
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/adapter/oidcdevice"
	"github.com/serverkraken/flow/internal/adapter/tokenstore"
	"github.com/serverkraken/flow/internal/clientconfig"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/spf13/cobra"
)

func loginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in via OIDC device flow",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cfg := clientconfig.Load(os.Getenv)
			if cfg.OIDCIssuer == "" {
				return fmt.Errorf("FLOW_OIDC_ISSUER is required to log in")
			}
			flow, err := oidcdevice.New(ctx, cfg.OIDCIssuer, cfg.CliClientID)
			if err != nil {
				return err
			}
			da, err := flow.Start(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("\nTo log in, open:\n\n  %s\n\nand enter the code:  %s\n\n", da.VerificationURI, da.UserCode)
			if da.VerificationURIComplete != "" {
				fmt.Printf("Or open this URL directly:\n\n  %s\n\n", da.VerificationURIComplete)
			}
			fmt.Println("Waiting for approval...")

			tok, err := flow.Poll(ctx, da)
			if err != nil {
				return fmt.Errorf("login failed (the code may have expired or been denied): %w", err)
			}
			if err := tokenstore.Open().Save(ports.Token{
				AccessToken:  tok.AccessToken,
				RefreshToken: tok.RefreshToken,
				Expiry:       tok.Expiry,
			}); err != nil {
				return fmt.Errorf("store token: %w", err)
			}
			// Honor the dev self-signed cert (matches clientFromStore); without
			// this the post-login Whoami fails with x509 against the dev server
			// even though the token was stored fine.
			base := http.DefaultTransport
			if cfg.InsecureTLS {
				base = apiclient.InsecureBase()
			}
			u, err := apiclient.NewTransport(cfg.ServerURL, &oauth2.Transport{
				Source: oauth2.StaticTokenSource(tok),
				Base:   base,
			}).Whoami(ctx)
			if err != nil {
				return fmt.Errorf("token stored but server rejected it: %w", err)
			}
			fmt.Printf("\nLogged in as %s <%s>\n", u.DisplayName, u.Email)
			return nil
		},
	}
}
