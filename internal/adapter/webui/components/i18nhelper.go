// Package components holds the reusable WebUI design-system templ components.
// It is separate from package webui so new components never clash with the
// existing webui exports (Base, AppShell, …). All user-facing strings come
// from internal/i18n via the T/Tn helpers below, which templates call with the
// implicit ctx.
package components

import (
	"context"

	"github.com/serverkraken/flow/internal/i18n"
)

// T is a templ-friendly re-export of i18n.T (templates call components.T(ctx, key)).
func T(ctx context.Context, key string) string { return i18n.T(ctx, key) }

// Tn is a templ-friendly re-export of i18n.Tn for plural strings.
func Tn(ctx context.Context, key string, n int) string { return i18n.Tn(ctx, key, n) }
