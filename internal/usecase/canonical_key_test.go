package usecase

import (
	"strings"
	"testing"
)

// fakeResolver returns RemoteURL=ok or signals "no remote" for tests.
type fakeResolver struct {
	url string
	ok  bool
}

func (f fakeResolver) RemoteURL(_ string) (string, bool) { return f.url, f.ok }

func TestCanonicalKey_GitAtForm(t *testing.T) {
	got, err := CanonicalKey("/anywhere", fakeResolver{url: "git@github.com:Foo/Bar.git", ok: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != "git:github.com/foo/bar" {
		t.Errorf("got %q, want git:github.com/foo/bar", got)
	}
}

func TestCanonicalKey_HTTPSForm(t *testing.T) {
	got, _ := CanonicalKey("/anywhere", fakeResolver{url: "https://gitlab.com/Foo/Bar.git", ok: true})
	if got != "git:gitlab.com/foo/bar" {
		t.Errorf("got %q", got)
	}
}

func TestCanonicalKey_SSHForm(t *testing.T) {
	got, _ := CanonicalKey("/anywhere", fakeResolver{url: "ssh://git@bitbucket.org/foo/Bar", ok: true})
	if got != "git:bitbucket.org/foo/bar" {
		t.Errorf("got %q", got)
	}
}

func TestCanonicalKey_NoRemote_FallsBackToPath(t *testing.T) {
	got, err := CanonicalKey("/tmp/some-local-dir", fakeResolver{ok: false})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "path:") {
		t.Errorf("expected path: prefix, got %q", got)
	}
	if len(got) != len("path:")+64 { // sha256 hex = 64 chars
		t.Errorf("expected 64-hex digest, got %q (len %d)", got, len(got)-5)
	}
}

func TestCanonicalKey_NoRemote_Stable(t *testing.T) {
	a, _ := CanonicalKey("/some/path", fakeResolver{ok: false})
	b, _ := CanonicalKey("/some/path", fakeResolver{ok: false})
	if a != b {
		t.Errorf("non-deterministic path-hash: %q vs %q", a, b)
	}
}

func TestCanonicalKey_NoResolver_TreatsAsNoRemote(t *testing.T) {
	got, err := CanonicalKey("/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "path:") {
		t.Errorf("nil resolver should produce path: key, got %q", got)
	}
}

func TestCanonicalKey_EmptyRemoteString_TreatedAsNoRemote(t *testing.T) {
	got, _ := CanonicalKey("/x", fakeResolver{url: "  ", ok: true})
	if !strings.HasPrefix(got, "path:") {
		t.Errorf("whitespace remote should be ignored, got %q", got)
	}
}
