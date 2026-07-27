package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated flow user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			u, err := client.Whoami(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Printf("%s <%s> (%s)\n", u.DisplayName, u.Email, u.Username)
			return nil
		},
	}
}
