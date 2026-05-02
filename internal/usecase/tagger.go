package usecase

import (
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Tagger surfaces tag-based suggestions to the TUI: most-recently-used
// tags, top-by-usage tags, and recurring (start, duration, tag) templates.
//
// All three reads share the same SessionStore.LoadAll source — the load
// happens inside each method so the use case stays stateless. If the
// adapter caches its session list, those calls are cheap.
type Tagger struct {
	Sessions ports.SessionStore
}

// Recent returns up to n most-recently-used tags, newest first. Empty
// tags are filtered out, duplicates collapsed.
func (t *Tagger) Recent(n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	sessions, err := t.Sessions.LoadAll()
	if err != nil {
		return nil, err
	}
	return domain.RecentTags(sessions, n), nil
}

// TopUsage returns up to n tags ordered by total session count desc, ties
// broken by most-recent use.
func (t *Tagger) TopUsage(n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	sessions, err := t.Sessions.LoadAll()
	if err != nil {
		return nil, err
	}
	return domain.TopUsageTags(sessions, n), nil
}

// RecentTemplates returns up to n recurring session shapes (start, duration,
// tag) seen at least twice. The 15-minute grid bucketing absorbs typical
// drift in start times.
func (t *Tagger) RecentTemplates(n int) ([]domain.SessionTemplate, error) {
	if n <= 0 {
		return nil, nil
	}
	sessions, err := t.Sessions.LoadAll()
	if err != nil {
		return nil, err
	}
	return domain.SessionTemplatesOf(sessions, n), nil
}
