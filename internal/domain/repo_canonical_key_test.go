package domain

import "testing"

func TestUnit_RepoCanonicalKeyFromRemote(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"git@github.com:foo/bar.git", "git:github.com/foo/bar"},
		{"git@github.com:foo/bar", "git:github.com/foo/bar"},
		{"https://github.com/Foo/Bar.git", "git:github.com/Foo/Bar"},
		{"https://gitlab.example.com/group/sub/repo", "git:gitlab.example.com/group/sub/repo"},
		{"", ""},
		{"not-a-url", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := RepoCanonicalKeyFromRemote(c.in)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestUnit_RepoCanonicalKeyFromPath_StablePerPath(t *testing.T) {
	t.Parallel()
	a := RepoCanonicalKeyFromPath("/Users/x/code/foo")
	b := RepoCanonicalKeyFromPath("/Users/x/code/foo/")
	c := RepoCanonicalKeyFromPath("/Users/x/code/bar")
	if a == "" {
		t.Fatal("empty key")
	}
	if a != b {
		t.Errorf("clean(path) should normalize trailing slash; %q != %q", a, b)
	}
	if a == c {
		t.Error("different paths should produce different keys")
	}
}
