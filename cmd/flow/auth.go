package main

import (
	"context"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientauth"
)

// clientFromStore delegates to the shared clientauth.Client so the CLI and
// flow-mcp build identical authenticated clients. Kept as a local alias so the
// CLI's many call sites stay unchanged.
func clientFromStore(ctx context.Context) (*apiclient.Client, error) {
	return clientauth.Client(ctx)
}
