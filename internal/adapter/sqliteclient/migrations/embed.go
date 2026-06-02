// Package migrations embeds the client-side SQL migrations so the
// adapter can run them with no external file dependency.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
