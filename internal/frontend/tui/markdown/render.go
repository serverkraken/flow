// Public Markdown rendering API. Builds a goldmark parser configured
// with the GFM + footnote extensions plus a custom inline parser for
// `[[id]]` wikilinks, runs it through the custom NodeRenderer
// (renderer.go + blocks.go + inline.go + …), then post-processes the
// ANSI output to wrap bare URLs as OSC 8 hyperlinks so they stay
// clickable across line wraps.

package markdown

import (
	"bytes"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
)

// Option configures one Render call. Functional-options pattern keeps
// `Render(source, width)` (the signature minimal call sites use)
// compiling unchanged.
type Option func(*options)

// options carries the resolved per-call settings. Internal — callers
// only see the With… helpers.
type options struct {
	frontmatter *Frontmatter
	backlinks   []BacklinkRef
	resolver    ports.WikilinkResolver
	noColor     bool
	nerdFont    bool
	// palette is the canonical color set the renderer reads from.
	// Defaulted in buildOptions when the caller omits WithPalette so
	// existing call-sites don't need to change.
	palette theme.Palette
}

// WithFrontmatter renders a styled card above the body. Pass nil (or
// omit) to skip the card. Used today by the full-screen viewer; the
// preview pane intentionally omits it.
func WithFrontmatter(fm *Frontmatter) Option {
	return func(o *options) { o.frontmatter = fm }
}

// WithBacklinks renders a "Referenced by" footer below the body. An
// empty slice is treated the same as omission — no footer.
func WithBacklinks(refs []BacklinkRef) Option {
	return func(o *options) { o.backlinks = refs }
}

// WithWikilinks supplies the resolver consulted for `[[id]]` lookups.
// Without it, all wikilinks render as broken (red, no OSC 8).
func WithWikilinks(r ports.WikilinkResolver) Option {
	return func(o *options) { o.resolver = r }
}

// WithNoColor forces ANSI colour output off. The NO_COLOR env var is
// also honoured implicitly (buildOptions reads it). Use this option
// to override env (e.g. in tests).
func WithNoColor(noColor bool) Option {
	return func(o *options) { o.noColor = noColor }
}

// WithNerdFont swaps Unicode-standard glyphs for Nerd-Font icons in
// headings, code-fence labels, and link decorations. Default off so
// the renderer never assumes a specific font.
func WithNerdFont(nerdFont bool) Option {
	return func(o *options) { o.nerdFont = nerdFont }
}

// WithPalette overrides the canonical default palette for this Render
// call. Useful when a screen wants to render notes against the same
// palette its enclosing surface uses (e.g. a per-test override for
// contrast / NO_COLOR fixtures). Omit to use theme.Default.
func WithPalette(p theme.Palette) Option {
	return func(o *options) { o.palette = p }
}

// Render parses source as CommonMark + GFM and returns ANSI output
// reflowed to width cells. Bare URLs in plain text are wrapped as
// OSC 8 hyperlinks. width <= 0 returns "".
func Render(source string, width int, opts ...Option) (string, error) {
	if width <= 0 {
		return "", nil
	}
	o := buildOptions(opts)
	src := []byte(source)

	nr := newNodeRenderer(width, o)
	// Priority 100 puts our NodeRenderer ahead of every renderer the
	// GFM extension installs (table / strikethrough / tasklist / linkify
	// register at priority 500). Without that, goldmark dispatches
	// tables to GFM's bundled HTML renderer and our renderTable never
	// fires.
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Footnote),
		goldmark.WithParserOptions(
			parser.WithInlineParsers(util.Prioritized(wikiLinkParser{}, 100)),
		),
		goldmark.WithRenderer(renderer.NewRenderer(
			renderer.WithNodeRenderers(util.Prioritized(nr, 100)),
		)),
	)

	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		return source, err
	}
	body := trimTrailingBlankLines(buf.String())
	body = WrapURLs(body)
	if card := nr.renderFrontmatterCard(o.frontmatter); card != "" {
		body = card + "\n" + body
	}
	if footer := nr.renderBacklinksFooter(o.backlinks); footer != "" {
		body = strings.TrimRight(body, "\n") + footer
	}
	// NO_COLOR / WithNoColor: lipgloss v2 has no Renderer to swap, so
	// strip SGR codes after rendering. The glyph + whitespace
	// hierarchy still carries the layout. OSC 8 hyperlinks survive —
	// ansi.Strip removes CSI/SGR sequences but leaves OSC 8 link
	// wrappers intact.
	if o.noColor {
		body = ansi.Strip(body)
	}
	return body, nil
}

// buildOptions collects opts, fills in defaults, and resolves NO_COLOR.
// The env var is read here (not in WithNoColor) so an explicit
// WithNoColor(false) cannot accidentally re-enable color when the user
// asked for none via env.
func buildOptions(opts []Option) options {
	o := options{}
	for _, fn := range opts {
		fn(&o)
	}
	if os.Getenv("NO_COLOR") != "" {
		o.noColor = true
	}
	// Empty Palette name signals "caller didn't pick one"; default to
	// the canonical so legacy call-sites stay one-line.
	if o.palette.Name == "" {
		o.palette = theme.Default
	}
	return o
}
