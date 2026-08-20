package webui

import (
	"context"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/timefmt"
)

// ActivityRowVM is the view model for one activity-log row in the Home Puls feed.
type ActivityRowVM struct {
	ActorKind string // "human" | "agent"
	ActorRef  string // display name or agent client name
	VerbKey   string // i18n key, e.g. "activity.verb.document.updated" — resolved in templ
	Label     string // empty if nil in the entry
	Href      string // "/wissen/{id}" for document.* events with a TargetRef, else ""
	RelTime   string // Datumsstaffel (Katalog 3.10): "heute 14:32" · "Fr" · "11.08."
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

// FmtRelTime schreibt einen Zeitpunkt in der Datumsstaffel des Katalogs 3.10
// aus: "heute 14:32" · "Fr" · "11.08." · "08.11.25". Vorher stand hier eine
// eigene Staffel ("vor 3 Std", danach das volle Datum) — die Liste zeigte
// damit zwei verschiedene Schreibweisen für denselben Sinn und die Spalte
// war nicht mehr abtastbar.
//
// SPRACHE: die Wörter sind hier fest deutsch, weil keiner der sechs Aufrufer
// ein ctx führt. Das ist kein neuer Mangel — die Funktion war vorher schon
// deutsch fest verdrahtet ("vor %d Min"). Für neuen Code gibt es FmtStaffel
// mit ctx; das Nachziehen der Altaufrufer ist ein eigener Schritt.
func FmtRelTime(at, now time.Time) string {
	return timefmt.Staffel(at, now, staffelWordsDE)
}

// staffelWordsDE ist die deutsche Fassung der Staffel-Wörter.
var staffelWordsDE = timefmt.StaffelWords{
	Today:    "heute",
	Weekdays: [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"},
}

// FmtStaffel ist die sprachbewusste Fassung: sie zieht die Wörter aus dem
// Katalog des Betrachters. Neuer Code nutzt diese.
func FmtStaffel(ctx context.Context, at, now time.Time) string {
	return timefmt.Staffel(at, now, timefmt.StaffelWords{
		Today: components.T(ctx, "staffel.today"),
		Weekdays: [7]string{
			components.T(ctx, "staffel.wd.sun"),
			components.T(ctx, "staffel.wd.mon"),
			components.T(ctx, "staffel.wd.tue"),
			components.T(ctx, "staffel.wd.wed"),
			components.T(ctx, "staffel.wd.thu"),
			components.T(ctx, "staffel.wd.fri"),
			components.T(ctx, "staffel.wd.sat"),
		},
	})
}
