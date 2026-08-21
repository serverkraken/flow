package webui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
)

// Start (Screen 24) — was der Schreibtisch über „Jetzt / Weiterarbeiten /
// Wissen / Puls" hinaus sagt: was Aufmerksamkeit braucht, was gestern war,
// was angepinnt ist — und zuletzt die Zahlen (Bestand, Erträge). Die
// Reihenfolge ist Soennes: Wissen vor Zahlen.

// AttentionRow ist ein Punkt, der auf dem Schreibtisch liegt, weil er sich
// nicht von selbst erledigt.
type AttentionRow struct {
	Kind  string // context | idle | stale | unfiled
	Title string
	Note  string
	Href  string
}

// YesterdayNote ist der Auszug der gestrigen Tagesnotiz.
type YesterdayNote struct {
	ID      string
	Day     string // "So, 10.08."
	Excerpt string
}

// BestandVM ist der Bestand in drei Zellen.
type BestandVM struct {
	Engagements, Vorhaben, Repos int
	CardsTotal, CardsActive, CardsArchived int
}

// ErtraegeVM sind die Erträge des Monats je Engagement.
type ErtraegeVM struct {
	Month       string
	TotalStr    string // Summe mit Satz, "" wenn kein Satz irgendwo
	NoRateHours string // Stunden ohne Satz, "" wenn keine
	Rows        []ErtragRow
}

// ErtragRow ist ein Engagement im Monat.
type ErtragRow struct {
	ID, Name, Hours, Amount string
	dur                     time.Duration
}

// EngagementMonth ist die Eingabe je Engagement: Monatszeit und Satz.
type EngagementMonth struct {
	Node  domain.Node
	Month time.Duration
	Rate  *domain.Money
}

// Greeting grüßt nach Tageszeit mit dem Vornamen.
func Greeting(ctx context.Context, now time.Time, displayName string) string {
	var key string
	switch h := now.Hour(); {
	case h < 5:
		key = "home.greet.night"
	case h < 11:
		key = "home.greet.morning"
	case h < 18:
		key = "home.greet.day"
	default:
		key = "home.greet.evening"
	}
	name := strings.TrimSpace(displayName)
	if i := strings.IndexAny(name, " \t"); i > 0 {
		name = name[:i]
	}
	if name == "" {
		return components.T(ctx, key)
	}
	return components.T(ctx, key) + ", " + name
}

// TargetLine sagt, wie weit das Tagesziel ist: „Ziel 8:00 h · noch 1:48",
// „Ziel erreicht" — oder nichts, wenn es kein Ziel gibt.
func TargetLine(ctx context.Context, logged, target time.Duration) string {
	if target <= 0 {
		return ""
	}
	if logged >= target {
		return components.T(ctx, "home.targetDone")
	}
	return fmt.Sprintf(components.T(ctx, "home.target"), fmtDurHM(target), fmtDurHM(target-logged))
}

// AttentionInput ist alles, was „Braucht Aufmerksamkeit" braucht.
type AttentionInput struct {
	Engagements    []domain.Node
	ContextDropped map[string]int       // Engagement-ID → Karten, die aus dem Budget fallen
	LastBooking    map[string]time.Time // Engagement-ID → letzte Buchung (nur im Fenster)
	Docs           []domain.Document
	Now            time.Time
}

// attentionIdleDays: ein Engagement, das im Fenster gebucht wurde, aber so
// lange nicht mehr, liegt still.
const attentionIdleDays = 4

// attentionStaleDays: eine Weisung, die so lange unangetastet ist, hat
// vermutlich die Arbeit überlebt.
const attentionStaleDays = 30

// BuildAttention ordnet: Kontext über Budget zuerst (das betrifft jeden
// Agenten-Start), dann stille Register, dann alte Weisungen, dann der
// Eingang.
func BuildAttention(ctx context.Context, in AttentionInput) []AttentionRow {
	var rows []AttentionRow
	for _, e := range in.Engagements {
		if n := in.ContextDropped[e.ID]; n > 0 {
			rows = append(rows, AttentionRow{Kind: "context",
				Title: components.T(ctx, "home.attention.context"),
				Note:  ShortName(e.Name) + " · " + components.Tn(ctx, "home.attention.contextNote", n),
				Href:  "/kontext/" + e.ID})
		}
	}
	for _, e := range in.Engagements {
		last, ok := in.LastBooking[e.ID]
		if !ok {
			continue
		}
		days := int(in.Now.Sub(last).Hours() / 24)
		if days >= attentionIdleDays {
			rows = append(rows, AttentionRow{Kind: "idle",
				Title: fmt.Sprintf(components.T(ctx, "home.attention.idle"), ShortName(e.Name)),
				Note:  fmt.Sprintf(components.T(ctx, "home.attention.idleNote"), days),
				Href:  "/nodes/" + e.ID + "?tab=worktime"})
		}
	}
	var stale []domain.Document
	unfiled := 0
	for _, d := range in.Docs {
		if d.Archived {
			continue
		}
		if d.Type == domain.DocInstruction && in.Now.Sub(d.UpdatedAt) > attentionStaleDays*24*time.Hour {
			stale = append(stale, d)
		}
		if d.NodeID == nil && d.Type != domain.DocDaily {
			unfiled++
		}
	}
	sort.SliceStable(stale, func(i, j int) bool { return stale[i].UpdatedAt.Before(stale[j].UpdatedAt) })
	for i, d := range stale {
		if i == 3 {
			break
		}
		rows = append(rows, AttentionRow{Kind: "stale",
			Title: components.T(ctx, "home.attention.stale"),
			Note:  d.Title + " · " + fmt.Sprintf(components.T(ctx, "home.attention.staleNote"), int(in.Now.Sub(d.UpdatedAt).Hours()/24)),
			Href:  "/wissen/" + d.ID})
	}
	if unfiled > 0 {
		rows = append(rows, AttentionRow{Kind: "unfiled",
			Title: components.T(ctx, "home.attention.unfiled"),
			Note:  fmt.Sprintf(components.T(ctx, "home.attention.unfiledNote"), unfiled),
			Href:  "/wissen/typ?type=free"})
	}
	return rows
}

// FindYesterdayNote sucht die Tagesnotiz von gestern.
func FindYesterdayNote(ctx context.Context, docs []domain.Document, now time.Time) *YesterdayNote {
	y := now.AddDate(0, 0, -1)
	for _, d := range docs {
		if d.Type != domain.DocDaily || d.Archived || d.Date == nil {
			continue
		}
		if d.Date.Year() == y.Year() && d.Date.YearDay() == y.YearDay() {
			return &YesterdayNote{ID: d.ID, Day: FmtStaffel(ctx, *d.Date, now), Excerpt: docKastenExcerpt(d.Body)}
		}
	}
	return nil
}

// BuildBestand zählt Register und Karten.
func BuildBestand(nodes []domain.Node, docs []domain.Document, archived int) BestandVM {
	b := BestandVM{CardsArchived: archived}
	for _, n := range nodes {
		if n.Status == domain.NodeArchived {
			continue
		}
		switch n.Kind {
		case domain.KindEngagement:
			b.Engagements++
		case domain.KindVorhaben:
			b.Vorhaben++
		case domain.KindRepo:
			b.Repos++
		}
	}
	for _, d := range docs {
		if !d.Archived {
			b.CardsActive++
		}
	}
	b.CardsTotal = b.CardsActive + b.CardsArchived
	return b
}

// BuildErtraege summiert den Monat: mit Satz in Geld, ohne Satz in Stunden.
// nil, wenn in diesem Monat nichts gebucht wurde.
func BuildErtraege(ctx context.Context, month string, in []EngagementMonth) *ErtraegeVM {
	vm := &ErtraegeVM{Month: month}
	var total domain.Money
	var noRate time.Duration
	mixed := false
	for _, e := range in {
		if e.Month <= 0 {
			continue
		}
		row := ErtragRow{ID: e.Node.ID, Name: ShortName(e.Node.Name), Hours: fmtDurHM(e.Month), dur: e.Month}
		if e.Rate != nil {
			amt := e.Rate.Mul(e.Month)
			row.Amount = amt.String()
			if total.Currency == "" {
				total.Currency = amt.Currency
			}
			if total.Currency != amt.Currency {
				mixed = true
			}
			total.Amount += amt.Amount
		} else {
			row.Amount = components.T(ctx, "home.ertraege.kein")
			noRate += e.Month
		}
		vm.Rows = append(vm.Rows, row)
	}
	if len(vm.Rows) == 0 {
		return nil
	}
	sort.SliceStable(vm.Rows, func(i, j int) bool { return vm.Rows[i].dur > vm.Rows[j].dur })
	if total.Currency != "" && !mixed {
		vm.TotalStr = total.String()
	}
	if noRate > 0 {
		vm.NoRateHours = fmt.Sprintf(components.T(ctx, "home.ertraege.noRate"), fmtDurHM(noRate))
	}
	return vm
}
