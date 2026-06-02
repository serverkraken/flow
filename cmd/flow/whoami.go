package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/spf13/cobra"
)

func newWhoamiCmd() *cobra.Command {
	var serverURL string
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Aktuell eingeloggten User vom Server abrufen",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store := keyringadapter.New()
			tok, err := store.Get(slotNameFor(serverURL))
			if errors.Is(err, ports.ErrTokenNotFound) {
				return errors.New("nicht eingeloggt — bitte `flow login` ausführen")
			}
			if err != nil {
				return err
			}
			req, _ := http.NewRequestWithContext(cmd.Context(), http.MethodGet, serverURL+"/api/v1/me-bearer", nil)
			req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server response: %s", resp.Status)
			}
			var out map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				return err
			}
			cmd.Printf("Sub:   %v\nEmail: %v\nName:  %v\n", out["sub"], out["email"], out["name"])
			return nil
		},
	}
	cmd.Flags().StringVar(&serverURL, "server", envOrDefault("FLOW_SERVER_URL", "http://localhost:8080"), "flow-server base URL")
	return cmd
}
