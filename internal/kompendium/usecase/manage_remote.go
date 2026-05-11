package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// ErrRemoteURLRequired signals that SetRemote was called without a URL.
var ErrRemoteURLRequired = errors.New("remote URL is required")

// ErrRemoteURLScheme signals that SetRemote was called with a URL whose
// scheme is not a recognised git transport (https://, ssh://, git://,
// file://, or scp-style git@host:path).
//
// Why: git supports the `ext::<command>` and `ext::ssh <command>`
// transports which execute arbitrary shell commands on every fetch /
// push. A user socially engineered into running
// `flow kompendium remote --url 'ext::sh -c …'` would otherwise hand
// over remote-code-execution-on-sync. Review finding S3.
var ErrRemoteURLScheme = errors.New("remote URL scheme not permitted")

// ManageRemote bundles the get + set operations for the notebook's
// "origin" git remote. A single use case keeps the CLI thin — `remote`
// (no args) calls Get, `remote set <url>` calls Set.
type ManageRemote struct {
	Store  ports.NoteStore
	Remote ports.NotebookRemote
}

// NewManageRemote wires the use case with its required ports.
func NewManageRemote(store ports.NoteStore, remote ports.NotebookRemote) *ManageRemote {
	return &ManageRemote{Store: store, Remote: remote}
}

// GetOutput reports the configured origin URL, or empty string when
// none is set (the use case translates ErrNoRemoteConfigured to a
// nil-error empty result so the CLI can decide how to render it).
type GetOutput struct {
	Root string
	URL  string
}

// Get returns the configured origin URL.
func (u *ManageRemote) Get(ctx context.Context) (GetOutput, error) {
	root := u.Store.Root()
	url, err := u.Remote.GetRemote(ctx, root)
	if err != nil {
		if errors.Is(err, ports.ErrNoRemoteConfigured) {
			return GetOutput{Root: root, URL: ""}, nil
		}
		return GetOutput{Root: root}, fmt.Errorf("get remote: %w", err)
	}
	return GetOutput{Root: root, URL: url}, nil
}

// SetInput carries the new origin URL.
type SetInput struct {
	URL string
}

// SetOutput reports the resolved root and the URL that was written.
type SetOutput struct {
	Root string
	URL  string
}

// Set writes a new origin URL.
func (u *ManageRemote) Set(ctx context.Context, in SetInput) (SetOutput, error) {
	url := strings.TrimSpace(in.URL)
	if url == "" {
		return SetOutput{}, ErrRemoteURLRequired
	}
	if !isAllowedRemoteURL(url) {
		return SetOutput{}, fmt.Errorf("%w: %q", ErrRemoteURLScheme, url)
	}
	root := u.Store.Root()
	if err := u.Remote.SetRemote(ctx, root, url); err != nil {
		return SetOutput{Root: root}, fmt.Errorf("set remote: %w", err)
	}
	return SetOutput{Root: root, URL: url}, nil
}

// isAllowedRemoteURL accepts only the standard git URL shapes and
// rejects `ext::` / `ext::ssh` (arbitrary shell exec on fetch) and any
// other unfamiliar scheme. See ErrRemoteURLScheme for the rationale.
func isAllowedRemoteURL(url string) bool {
	switch {
	case strings.HasPrefix(url, "https://"),
		strings.HasPrefix(url, "http://"),
		strings.HasPrefix(url, "ssh://"),
		strings.HasPrefix(url, "git://"),
		strings.HasPrefix(url, "file://"):
		return true
	}
	// scp-style: user@host:path/to/repo (no slash before the colon).
	if i := strings.IndexByte(url, ':'); i > 0 {
		head := url[:i]
		if !strings.ContainsAny(head, "/\\") && strings.Contains(head, "@") {
			return true
		}
	}
	return false
}
