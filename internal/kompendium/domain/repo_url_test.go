package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestNormalizeURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want domain.CanonicalURL
	}{
		{"ssh-short with .git", "git@github.com:Foo/Bar.git", "github.com/foo/bar"},
		{"ssh-short without .git", "git@github.com:Foo/Bar", "github.com/foo/bar"},
		{"https with trailing slash", "https://github.com/Foo/Bar/", "github.com/foo/bar"},
		{"https with .git", "https://github.com/foo/bar.git", "github.com/foo/bar"},
		{"http insecure", "http://gitlab.example.com/group/proj.git", "gitlab.example.com/group/proj"},
		{"ssh full with userinfo", "ssh://git@github.com/foo/bar.git", "github.com/foo/bar"},
		{"git protocol", "git://example.com/foo/bar.git", "example.com/foo/bar"},
		{"https with user:pass", "https://user:pass@host/foo/bar.git", "host/foo/bar"},
		{"path containing at sign survives", "https://host/foo/bar@v2", "host/foo/bar@v2"},
		{"deep path with multiple segments", "https://gitlab.com/group/sub/proj.git", "gitlab.com/group/sub/proj"},
		{"already canonical", "github.com/foo/bar", "github.com/foo/bar"},
		{"surrounding whitespace", "  git@github.com:foo/bar.git  ", "github.com/foo/bar"},
		{"uppercase host normalised", "https://GitHub.com/Foo/Bar", "github.com/foo/bar"},
		{"single segment after ssh-short", "git@host:Foo.git", "host/foo"},
		{"path contains colon", "https://host/foo:bar", "host/foo:bar"},

		// Filesystem-path fallback (gitrepo.Detect uses repo root when origin is unset).
		// macOS is case-preserving + Foundation expects the original case, so the
		// path passes through verbatim — only the trailing slash is stripped.
		{"absolute path preserves case", "/Users/dev/notes", "/Users/dev/notes"},
		{"absolute path trailing slash trimmed", "/Users/dev/notes/", "/Users/dev/notes"},
		{"home-relative path preserves case", "~/Notes", "~/Notes"},
		{"deep absolute path with mixed case", "/Users/Dev/Sourcecode/Project", "/Users/Dev/Sourcecode/Project"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.NormalizeURL(tc.in); got != tc.want {
				t.Errorf("NormalizeURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCanonicalURL_String(t *testing.T) {
	t.Parallel()
	c := domain.CanonicalURL("github.com/foo/bar")
	if got, want := c.String(), "github.com/foo/bar"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCanonicalURL_Sanitize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   domain.CanonicalURL
		want string
	}{
		{"github.com/foo/bar", "github.com_foo_bar"},
		{"gitlab.example.com/group/sub/proj", "gitlab.example.com_group_sub_proj"},
		{"host/single", "host_single"},
	}
	for _, tc := range cases {
		t.Run(string(tc.in), func(t *testing.T) {
			t.Parallel()
			if got := tc.in.Sanitize(); got != tc.want {
				t.Errorf("Sanitize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
