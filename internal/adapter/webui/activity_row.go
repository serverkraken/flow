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
	// Ziel-Pill (nur session.* mit NodeRef): live node name+kind, or the Label
	// snapshot when the node no longer exists (then kind=="" and no href).
	TargetName string
	TargetKind domain.NodeKind
	TargetHref string
}

// BuildActivityRows converts domain.ActivityEntry slices to ActivityRowVM slices.
// names/kinds are the owner's current node lookups (s.nodeMaps): a session row
// whose NodeRef still resolves renders the live target pill (linked); a deleted
// node falls back to the persisted Label snapshot (unlinked pill, no kind).
// `now` is used for RelTime formatting only.
func BuildActivityRows(entries []domain.ActivityEntry, names map[string]string, kinds map[string]domain.NodeKind, now time.Time) []ActivityRowVM {
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
		row := ActivityRowVM{
			ActorKind: e.ActorKind,
			ActorRef:  e.ActorRef,
			VerbKey:   "activity.verb." + e.Kind,
			Label:     label,
			Href:      href,
			RelTime:   FmtRelTime(e.At, now),
		}
		if strings.HasPrefix(e.Kind, "session.") && e.NodeRef != nil {
			if name, ok := names[*e.NodeRef]; ok {
				row.TargetName = name
				row.TargetKind = kinds[*e.NodeRef]
				row.TargetHref = "/nodes/" + *e.NodeRef
			} else {
				row.TargetName = label // snapshot of the deleted node's name
			}
			// the label for session rows IS the node name — the pill replaces it.
			row.Label = ""
		}
		rows = append(rows, row)
	}
	return rows
}

// FmtRelTime formats a timestamp relative to now in German.
// < 60 min  → "vor N Min"
// < 24 h    → "vor N Std"
// older     → date "02.01.2006"
//
// Exported so httpserver's document-page handler (Provenance row) can reuse
// the exact same relative-time convention as the cockpit/activity feed
// instead of inventing a second one. NOTE: this is hardcoded German
// regardless of locale, matching the pre-existing behaviour of every other
// caller (cockpit pulse, activity feed, Wissen "zuletzt aktualisiert") — a
// wider locale-aware fix is out of scope for this task.
func FmtRelTime(at, now time.Time) string {
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
