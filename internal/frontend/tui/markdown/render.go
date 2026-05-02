// Public Markdown rendering API. Builds a goldmark parser configured
// with the GFM + footnote extensions plus a custom inline parser for
// `[[id]]` wikilinks, runs it through the custom NodeRenderer
// (renderer.go + blocks.go + inline.go + …), then post-processes the
// ANSI output to wrap bare URLs as OSC 8 hyperlinks so they stay
// clickable across line wraps.

package markdown

import (
	"bytes"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

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
	lip         *lipgloss.Renderer
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

// WithNoColor forces ANSI colour output off. NO_COLOR env var is also
// honoured implicitly via termenv. Use this option to override env
// (e.g. in tests).
func WithNoColor(noColor bool) Option {
	return func(o *options) { o.noColor = noColor }
}

// WithNerdFont swaps Unicode-standard glyphs for Nerd-Font icons in
// headings, code-fence labels, and link decorations. Default off so
// the renderer never assumes a specific font.
func WithNerdFont(nerdFont bool) Option {
	return func(o *options) { o.nerdFont = nerdFont }
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
	return body, nil
}

// buildOptions collects opts, fills in defaults, and resolves the
// effective lipgloss renderer. NO_COLOR (env or option) selects an
// Ascii-profile renderer so styled.Render() emits no SGR codes; the
// glyph + whitespace hierarchy still carries the layout.
func buildOptions(opts []Option) options {
	o := options{}
	for _, fn := range opts {
		fn(&o)
	}
	noColor := o.noColor || termenv.EnvNoColor()
	if noColor {
		o.lip = lipgloss.NewRenderer(io.Discard, termenv.WithProfile(termenv.Ascii))
	} else {
		o.lip = lipgloss.DefaultRenderer()
	}
	return o
}
