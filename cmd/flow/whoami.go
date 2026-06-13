package main

import (
	"fmt"
	"os"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/spf13/cobra"
)

func whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated flow user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			base := envOr("FLOW_SERVER_URL", "http://localhost:8080")
			token := os.Getenv("FLOW_TOKEN") // device-flow login lands in M1
			if token == "" {
				return fmt.Errorf("set FLOW_TOKEN (device-flow login comes in M1)")
			}
			u, err := apiclient.New(base, token).Whoami(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Printf("%s <%s> (%s)\n", u.DisplayName, u.Email, u.Username)
			return nil
		},
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
