// Package migrations contains embedded SQL migration files.
package migrations

import "embed"

// FS embeds all SQL migration files.
//
//go:embed *.sql
var FS embed.FS
