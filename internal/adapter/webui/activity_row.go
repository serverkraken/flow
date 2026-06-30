package webui

import (
	"fmt"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// ActivityRowVM is the view model for one activity-log row on the Home logstream.
type ActivityRowVM struct {
	ActorKind string // "human" | "agent"
	ActorRef  string // display name or agent client name
	VerbKey   string // i18n key, e.g. "activity.verb.document.updated" — resolved in templ
	Label     string // empty if nil in the entry
	Href      string // "/wissen/{id}" for document.* events with a TargetRef, else ""
	RelTime   string // German relative time, e.g. "vor 3 Min"
}

// BuildActivityRows converts domain.ActivityEntry slices to ActivityRowVM slices.
// `now` is used for RelTime formatting only.
func BuildActivityRows(entries []domain.ActivityEntry, now time.Time) []ActivityRowVM {
	rows := make([]ActivityRowVM, 0, len(entries))
	for _, e := range entries {
		var label string
		if e.Label != nil {
			label = *e.Label
		}
		var href string
		if strings.HasPrefix(e.Kind, "document.") && e.TargetRef != nil {
			href = "/wissen/" + *e.TargetRef
		}
		rows = append(rows, ActivityRowVM{
			ActorKind: e.ActorKind,
			ActorRef:  e.ActorRef,
			VerbKey:   "activity.verb." + e.Kind,
			Label:     label,
			Href:      href,
			RelTime:   fmtRelTime(e.At, now),
		})
	}
	return rows
}

// DistinctActors returns deduplicated actor refs from entries, preserving first-seen order.
func DistinctActors(entries []domain.ActivityEntry) []string {
	seen := make(map[string]struct{}, len(entries))
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if _, ok := seen[e.ActorRef]; !ok {
			seen[e.ActorRef] = struct{}{}
			out = append(out, e.ActorRef)
		}
	}
	return out
}

// fmtRelTime formats a timestamp relative to now in German.
// < 60 min  → "vor N Min"
// < 24 h    → "vor N Std"
// older     → date "02.01.2006"
func fmtRelTime(at, now time.Time) string {
	diff := now.Sub(at)
	if diff < 0 {
		diff = 0
	}
	switch {
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins < 1 {
			mins = 1
		}
		return fmt.Sprintf("vor %d Min", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours < 1 {
			hours = 1
		}
		return fmt.Sprintf("vor %d Std", hours)
	default:
		return at.Local().Format("02.01.2006")
	}
}
