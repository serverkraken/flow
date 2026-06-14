package main

import (
	"fmt"

	"github.com/serverkraken/flow/internal/adapter/tokenstore"
	"github.com/spf13/cobra"
)

func logoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored flow credentials",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := tokenstore.Open().Clear(); err != nil {
				return err
			}
			fmt.Println("Logged out.")
			return nil
		},
	}
}
