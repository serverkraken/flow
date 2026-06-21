package main

import (
	"context"
	"log/slog"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientauth"
)

// bootClient builds the authenticated client and verifies the token against the
// server. On any failure it returns authed=false so the server still starts and
// every tool surfaces a clean "run flow login" message instead of crashing.
func bootClient(ctx context.Context, log *slog.Logger) (*apiclient.Client, bool) {
	client, err := clientauth.Client(ctx)
	if err != nil {
		log.Warn("not authenticated; tools will require login", "err", err)
		return nil, false
	}
	if _, err := client.Whoami(ctx); err != nil {
		log.Warn("token rejected by server; tools will require login", "err", err)
		return client, false
	}
	return client, true
}
