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
	root := u.Store.Root()
	if err := u.Remote.SetRemote(ctx, root, url); err != nil {
		return SetOutput{Root: root}, fmt.Errorf("set remote: %w", err)
	}
	return SetOutput{Root: root, URL: url}, nil
}
