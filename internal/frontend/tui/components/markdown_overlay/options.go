package markdown_overlay

// config holds the resolved Option set. Unexported; callers configure
// via With* funcs declared alongside each feature.
type config struct {
	title        string
	source       string
	enableSearch bool
	closeKeys    []string
}

// Option configures a Model at New time. Composable.
type Option func(*config)

func defaultConfig() config {
	return config{
		closeKeys: []string{"q", "esc", "b"},
	}
}

// WithTitle sets the title shown in the chrome's title row and status
// bar's path segment.
func WithTitle(title string) Option {
	return func(c *config) { c.title = title }
}

// WithSource sets the initial markdown body. Equivalent to calling
// SetSource after New; offered as an option so simple call sites stay
// declarative.
func WithSource(src string) Option {
	return func(c *config) { c.source = src }
}

// WithCloseKeys overrides the default close-key set (q, esc, b). Empty
// keys input is ignored; the default stays in effect.
func WithCloseKeys(keys ...string) Option {
	return func(c *config) {
		if len(keys) > 0 {
			c.closeKeys = append([]string{}, keys...)
		}
	}
}

// WithSearch enables the `/` key, the textinput-driven search mode, the
// match-bar gutter, and the match-counter status-bar segment. Disabled
// by default — explicit opt-in keeps simple overlays free of an
// unwanted key surface.
func WithSearch() Option {
	return func(c *config) { c.enableSearch = true }
}
