package domain

import "testing"

func TestNormalizeRemoteSlug(t *testing.T) {
	cases := map[string]string{
		"git@github.com:serverkraken/flow.git":          "github.com/serverkraken/flow",
		"git@github.com:serverkraken/flow":              "github.com/serverkraken/flow",
		"ssh://git@github.com/serverkraken/flow.git":    "github.com/serverkraken/flow",
		"https://github.com/serverkraken/flow.git":      "github.com/serverkraken/flow",
		"https://user@gitlab.com:8443/a/b/c.git/":       "gitlab.com/a/b/c",
		"https://github.com/Serverkraken/Flow":          "github.com/serverkraken/flow", // case-folded
	}
	for in, want := range cases {
		got, ok := NormalizeRemoteSlug(in)
		if !ok || got != want {
			t.Errorf("NormalizeRemoteSlug(%q) = %q,%v want %q,true", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "   ", "not a url", "https://"} {
		if got, ok := NormalizeRemoteSlug(bad); ok {
			t.Errorf("NormalizeRemoteSlug(%q) = %q,true want ok=false", bad, got)
		}
	}
}
