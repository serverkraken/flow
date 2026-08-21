package webui

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// Der Strassenfuchs-Teilbaum im Kleinen: ein Vorhaben ohne eigene Karten,
// ein Repo mit vielen, ein Repo mit wenigen, ein Repo ohne.
func wissenEbeneFixture(now time.Time) WissenEbeneInput {
	vid, r1, r2, r3 := "vor", "sf", "dash", "leer"
	nodes := []domain.Node{
		{ID: vid, Name: "Strassenfuchs", Kind: domain.KindVorhaben},
		{ID: r1, Name: "strassenfuchs", Kind: domain.KindRepo, ParentID: &vid},
		{ID: r2, Name: "admin-dashboard", Kind: domain.KindRepo, ParentID: &vid},
		{ID: r3, Name: "leer", Kind: domain.KindRepo, ParentID: &vid},
	}
	var docs []domain.Document
	mk := func(id, node string, t domain.DocumentType, title string, age time.Duration) domain.Document {
		n := node
		return domain.Document{ID: id, NodeID: &n, Type: t, Path: strings.ToLower(title), Title: title, UpdatedAt: now.Add(-age), CreatedAt: now.Add(-age - time.Hour)}
	}
	for i := 0; i < 6; i++ {
		docs = append(docs, mk("s"+string(rune('a'+i)), r1, domain.DocSpec, "Spec "+string(rune('A'+i)), time.Duration(i+2)*time.Hour))
	}
	for i := 0; i < 4; i++ {
		docs = append(docs, mk("p"+string(rune('a'+i)), r1, domain.DocPlan, "Plan "+string(rune('A'+i)), time.Duration(i+10)*time.Hour))
	}
	mem := mk("m1", r1, domain.DocMemory, "Fallen der Linie", 30*time.Hour)
	mem.ContextMode = domain.ContextModeImmer
	mem.Tags = []string{"fallen", "htmx"}
	docs = append(docs, mem)
	docs = append(docs, mk("d1", r2, domain.DocProject, "Dashboard README", 1*time.Hour))
	docs = append(docs, mk("d2", r2, domain.DocSpec, "Dashboard Spec", 40*time.Hour))
	// Außerhalb des Teilbaums und archiviert — beides darf nicht erscheinen.
	other := "anderswo"
	docs = append(docs, domain.Document{ID: "x1", NodeID: &other, Type: domain.DocSpec, Title: "Fremd", UpdatedAt: now})
	arch := mk("ar", r1, domain.DocSpec, "Archiviert", time.Hour)
	arch.Archived = true
	docs = append(docs, arch)
	docs = append(docs, domain.Document{ID: "free", Type: domain.DocFree, Title: "Ohne Knoten", UpdatedAt: now})
	return WissenEbeneInput{N: nodes[0], Subtree: nodes, Docs: docs, Now: now}
}

func TestBuildWissenEbene_GroupsByOriginFreshestFirst(t *testing.T) {
	now := time.Date(2026, 8, 21, 18, 0, 0, 0, time.Local)
	vm := BuildWissenEbene(context.Background(), wissenEbeneFixture(now))

	if !vm.Grouped {
		t.Fatal("ein Vorhaben gliedert nach Herkunft")
	}
	if vm.Total != 13 {
		t.Errorf("Total = %d, want 13 (archiviert, fremd und knotenlos zählen nicht)", vm.Total)
	}
	names := make([]string, 0, len(vm.Groups))
	for _, g := range vm.Groups {
		names = append(names, g.Name)
	}
	// admin-dashboard hat die frischeste Karte (1h), strassenfuchs folgt (2h);
	// das Vorhaben selbst ist leer und steht als Hinweis am Ende; "leer" fehlt.
	if got := strings.Join(names, ","); got != "admin-dashboard,strassenfuchs,Strassenfuchs" {
		t.Errorf("Gruppenreihenfolge = %s", got)
	}
	self := vm.Groups[2]
	if !self.Self || self.Total != 0 {
		t.Errorf("die eigene Ebene muss als leere Gruppe stehen: %+v", self)
	}
	sf := vm.Groups[1]
	if sf.Total != 11 || sf.Hidden != 3 || len(sf.Rows) != wissenGruppeCap || sf.Expanded {
		t.Errorf("strassenfuchs: Total=%d Hidden=%d Rows=%d Expanded=%v", sf.Total, sf.Hidden, len(sf.Rows), sf.Expanded)
	}
	// Typ-Zähler: häufigster zuerst.
	if len(sf.Types) != 3 || sf.Types[0].Type != domain.DocSpec || sf.Types[0].Count != 6 || sf.Types[1].Type != domain.DocPlan || sf.Types[2].Type != domain.DocMemory {
		t.Errorf("Typ-Zähler = %+v", sf.Types)
	}
	if !strings.Contains(sf.Types[0].Href, "typ=spec") || !strings.Contains(sf.Types[0].Href, "in=sf") {
		t.Errorf("der Typ-Zähler filtert seine Gruppe: %s", sf.Types[0].Href)
	}
	if !strings.Contains(sf.MoreFragment, "open=sf") || !strings.HasPrefix(sf.MoreFragment, "/nodes/vor/wissen?") {
		t.Errorf("„alle ›\" klappt an Ort und Stelle auf: %s", sf.MoreFragment)
	}
	// Standard: zuletzt geändert zuerst.
	if sf.Rows[0].Title != "Spec A" {
		t.Errorf("erste Zeile = %s, want Spec A", sf.Rows[0].Title)
	}
}

func TestBuildWissenEbene_KuratierMarkeOnlyForContextTypes(t *testing.T) {
	now := time.Date(2026, 8, 21, 18, 0, 0, 0, time.Local)
	in := wissenEbeneFixture(now)
	in.Query = WissenEbeneQuery{Open: []string{"sf"}}
	vm := BuildWissenEbene(context.Background(), in)
	sf := vm.Groups[1]
	if sf.Hidden != 0 || len(sf.Rows) != 11 || !sf.Expanded || !sf.Collapsible {
		t.Fatalf("open=sf muss die Gruppe ganz zeigen: Hidden=%d Rows=%d", sf.Hidden, len(sf.Rows))
	}
	var marks int
	for _, r := range sf.Rows {
		if r.ID == "m1" && r.Mode != domain.ContextModeImmer {
			t.Errorf("die Memory trägt ihre Kuratier-Marke: %q", r.Mode)
		}
		if r.Mode != "" {
			marks++
		}
	}
	if marks != 1 {
		t.Errorf("nur kontextfähige Typen tragen eine Marke, got %d", marks)
	}
	if !strings.Contains(sf.LessFragment, "/nodes/vor/wissen") || strings.Contains(sf.LessFragment, "open=") {
		t.Errorf("„weniger\" nimmt open wieder heraus: %s", sf.LessFragment)
	}
}

func TestBuildWissenEbene_TypeFilterHitsOneGroup(t *testing.T) {
	now := time.Date(2026, 8, 21, 18, 0, 0, 0, time.Local)
	in := wissenEbeneFixture(now)
	in.Query = WissenEbeneQuery{Typ: domain.DocSpec, In: "sf"}
	vm := BuildWissenEbene(context.Background(), in)
	if !vm.Filtered {
		t.Error("ein Typ-Filter ist ein Filter")
	}
	sf, dash := vm.Groups[1], vm.Groups[0]
	if sf.Matching != 6 || len(sf.Rows) != 6 || sf.Hidden != 0 || !sf.Expanded {
		t.Errorf("gefilterte Gruppe zeigt alle Treffer ungedeckelt: Matching=%d Rows=%d Hidden=%d", sf.Matching, len(sf.Rows), sf.Hidden)
	}
	if dash.Matching != 2 {
		t.Errorf("der Filter wirkt nur auf seine Gruppe; admin-dashboard zeigt %d statt 2", dash.Matching)
	}
	if !sf.Types[0].Active || strings.Contains(sf.Types[0].Href, "typ=") {
		t.Errorf("der aktive Zähler ist markiert und sein Link hebt den Filter auf: %+v", sf.Types[0])
	}
	if vm.Matching != 8 {
		t.Errorf("Matching = %d, want 8", vm.Matching)
	}
}

func TestBuildWissenEbene_SearchCoversTitlePathTags(t *testing.T) {
	now := time.Date(2026, 8, 21, 18, 0, 0, 0, time.Local)
	in := wissenEbeneFixture(now)
	in.Query = WissenEbeneQuery{Q: "HTMX"}
	vm := BuildWissenEbene(context.Background(), in)
	if vm.Matching != 1 {
		t.Fatalf("Suche über Tags: Matching = %d, want 1", vm.Matching)
	}
	for _, g := range vm.Groups {
		if g.Name == "strassenfuchs" && (len(g.Rows) != 1 || g.Rows[0].ID != "m1") {
			t.Errorf("Treffer fehlt: %+v", g.Rows)
		}
		if g.Name == "admin-dashboard" && len(g.Rows) != 0 {
			t.Errorf("Gruppe ohne Treffer bleibt leer, behält aber ihren Kopf: %+v", g.Rows)
		}
	}
	if !strings.Contains(vm.FragmentHref, "q=HTMX") {
		t.Errorf("der Zustand steht im Neulade-Link: %s", vm.FragmentHref)
	}
}

func TestBuildWissenEbene_SortAppliesWithinGroups(t *testing.T) {
	now := time.Date(2026, 8, 21, 18, 0, 0, 0, time.Local)
	in := wissenEbeneFixture(now)
	in.Query = WissenEbeneQuery{Sort: SortTitle, Open: []string{wissenOpenAll}}
	vm := BuildWissenEbene(context.Background(), in)
	sf := vm.Groups[1]
	if sf.Rows[0].Title != "Fallen der Linie" || sf.Rows[1].Title != "Plan A" {
		t.Errorf("Titel · A→Z: %s, %s", sf.Rows[0].Title, sf.Rows[1].Title)
	}
	if !strings.Contains(vm.PageHref, "sort=titel") || strings.Contains(vm.SortBaseHref, "sort=") {
		t.Errorf("Sortierung in der URL, Basis ohne: %s / %s", vm.PageHref, vm.SortBaseHref)
	}
}

func TestBuildWissenEbene_RepoIsFlat(t *testing.T) {
	now := time.Date(2026, 8, 21, 18, 0, 0, 0, time.Local)
	in := wissenEbeneFixture(now)
	in.N = in.Subtree[1] // strassenfuchs
	vm := BuildWissenEbene(context.Background(), in)
	if vm.Grouped || len(vm.Groups) != 1 || !vm.Groups[0].Self {
		t.Fatalf("ein Repo hat eine Herkunft: Grouped=%v Groups=%d", vm.Grouped, len(vm.Groups))
	}
	if vm.Total != 11 {
		t.Errorf("Total = %d, want 11", vm.Total)
	}
	if len(vm.Types) != 3 || strings.Contains(vm.Types[0].Href, "in=") {
		t.Errorf("am Repo filtert der Zähler ohne Gruppe: %+v", vm.Types)
	}
	if !strings.HasSuffix(vm.ManagerHref, "scope=self") {
		t.Errorf("ManagerHref = %s", vm.ManagerHref)
	}
}

func TestParseWissenEbeneQuery(t *testing.T) {
	v, _ := url.ParseQuery("q=+foo+&sort=titel&typ=spec&in=sf&open=sf&open=dash&open=sf")
	q := ParseWissenEbeneQuery(v)
	if q.Q != "foo" || q.Sort != SortTitle || q.Typ != domain.DocSpec || q.In != "sf" || len(q.Open) != 2 {
		t.Errorf("geparst: %+v", q)
	}
	v, _ = url.ParseQuery("sort=kaputt&typ=gibtsnicht&in=sf")
	q = ParseWissenEbeneQuery(v)
	if q.Sort != SortChanged || q.Typ != "" || q.In != "" {
		t.Errorf("Unbekanntes fällt auf den Standard, in ohne typ entfällt: %+v", q)
	}
	if got := WissenEbenePageHref("n1", WissenEbeneQuery{}); got != "/nodes/n1?tab=wissen" {
		t.Errorf("Standard-Seite = %s", got)
	}
	if got := WissenEbeneFragmentHref("n1", WissenEbeneQuery{}); got != "/nodes/n1/wissen" {
		t.Errorf("Standard-Fragment = %s", got)
	}
}
