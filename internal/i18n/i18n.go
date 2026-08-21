// Package i18n is a dependency-free translation layer for the WebUI.
// Catalogs are Go maps (de primary, en stub). Strings resolve by the locale
// carried in context; missing keys fall back to the default locale, then to
// the key itself so a missing string is visible, never blank.
package i18n

import (
	"context"
	"strconv"
	"strings"
)

// Locale identifies a UI language.
type Locale string

const (
	DE Locale = "de"
	EN Locale = "en"

	// Default is used when no locale is in context and as the fallback catalog.
	Default = DE
)

// Plural holds the two CLDR categories German and English need.
type Plural struct{ One, Other string }

type catalog struct {
	strings map[string]string
	plurals map[string]Plural
}

// catalogs is populated by the per-language files' init() funcs.
var catalogs = map[Locale]catalog{}

func register(l Locale, c catalog) { catalogs[l] = c }

type ctxKey int

const localeKey ctxKey = 0

// WithLocale stores the locale in ctx for T/Tn.
func WithLocale(ctx context.Context, l Locale) context.Context {
	return context.WithValue(ctx, localeKey, l)
}

// FromContext returns the ctx locale or Default.
func FromContext(ctx context.Context) Locale {
	if l, ok := ctx.Value(localeKey).(Locale); ok && l != "" {
		return l
	}
	return Default
}

// T returns the translation of key for the ctx locale, falling back to the
// Default locale and finally to key.
func T(ctx context.Context, key string) string {
	l := FromContext(ctx)
	if s, ok := catalogs[l].strings[key]; ok {
		return s
	}
	if s, ok := catalogs[Default].strings[key]; ok {
		return s
	}
	return key
}

// HasKey reports whether any catalog carries key, as a plain string or as a
// plural set. T and Tn fall back to returning the key itself when it is
// missing — on screen that shows up as "cockpit.rail.facts" where a heading
// belongs, and nothing else notices. The used-key test uses this to catch it.
func HasKey(key string) bool {
	for _, c := range catalogs {
		if _, ok := c.strings[key]; ok {
			return true
		}
		if _, ok := c.plurals[key]; ok {
			return true
		}
	}
	return false
}

// Tn returns the singular/plural form of key for n in the ctx locale.
// "{{.N}}" in the chosen form is replaced by n.
func Tn(ctx context.Context, key string, n int) string {
	l := FromContext(ctx)
	p, ok := catalogs[l].plurals[key]
	if !ok {
		p, ok = catalogs[Default].plurals[key]
	}
	if !ok {
		return key
	}
	form := p.Other
	if n == 1 {
		form = p.One
	}
	return strings.ReplaceAll(form, "{{.N}}", strconv.Itoa(n))
}
