package main

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/adapter/tokenstore"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/spf13/cobra"
)

func logoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored flow credentials",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := clearStoredToken(cmd.Context(), tokenstore.Open()); err != nil {
				return err
			}
			fmt.Println("Logged out.")
			return nil
		},
	}
}

func clearStoredToken(ctx context.Context, store ports.TokenStore) error {
	return store.WithLock(ctx, func(session ports.TokenStoreSession) error {
		return session.Clear()
	})
}
