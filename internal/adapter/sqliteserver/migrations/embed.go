// Package migrations embeds the server-side SQL migrations so the
// adapter can run them with no external file dependency.
package migrations

import "embed"

// FS contains all SQL migration files used by sqliteserver.Open at startup.
// The embed directive bundles every *.sql in this directory into the binary
// so deployment doesn't need to ship the migration files separately.
//
//go:embed *.sql
var FS embed.FS
