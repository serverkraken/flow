package main

import (
	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
	"github.com/spf13/cobra"
)

func newLogoutCmd() *cobra.Command {
	var serverURL string
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Token aus dem Keychain entfernen",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store := keyringadapter.New()
			if err := store.Delete(slotNameFor(serverURL)); err != nil {
				return err
			}
			cmd.Println("✓ Token entfernt.")
			return nil
		},
	}
	cmd.Flags().StringVar(&serverURL, "server", envOrDefault("FLOW_SERVER_URL", "http://localhost:8080"), "flow-server base URL")
	return cmd
}
