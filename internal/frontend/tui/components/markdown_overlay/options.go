package markdown_overlay

// config holds the resolved Option set. Unexported; callers configure
// via With* funcs declared alongside each feature.
type config struct {
	closeKeys []string
}

// Option configures a Model at New time. Composable.
type Option func(*config)

func defaultConfig() config {
	return config{
		closeKeys: []string{"q", "esc", "b"},
	}
}
