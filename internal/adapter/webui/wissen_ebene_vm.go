package webui

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// Der Wissens-Überblick je Ebene (Soenne, 21.08.: „Mir geht es hauptsächlich
// um das Wissen pro Engagement und Projekt. Wir bauen einen Kontextspeicher.")
//
// Auf einem Vorhaben oder Engagement ist das Wissen des ganzen Teilbaums
// nach HERKUNFT gegliedert — eine Gruppe je Register, das Karten trägt,
// Typen als Zähler im Gruppenkopf, die Gruppen nach Frische sortiert. Auf
// einem Repo gibt es nur eine Herkunft, also keine Gruppe; die Typ-Zähler
// stehen dann unter dem Kopf. Suche und Sortierung laufen über alles und sind
// ungedeckelt; nur die ungefilterte Gruppe zeigt anfangs acht Zeilen und
// klappt an Ort und Stelle auf — kein Sprung in die Bibliothek.

// wissenGruppeCap ist die Zeilenzahl, die eine ungefilterte Gruppe anfangs
// zeigt. Sie gilt je Gruppe, nicht je Seite: ein Teilbaum mit drei Repos
// zeigt bis zu 24 Zeilen, und „alle ›" hebt sie für genau eine Gruppe auf.
const wissenGruppeCap = 8

// wissenOpenAll ist der Wert, mit dem ?open= jede Gruppe aufklappt.
const wissenOpenAll = "alle"

// WissenEbeneQuery ist der Zustand der Fläche, wie er in der URL steht.
// Jeder Link der Fläche trägt ihn vollständig weiter — Suche, Sortierung,
// Typ-Filter und aufgeklappte Gruppen gehen beim Klick auf „alle ›" nicht
// verloren, und das SSE-Neuladen kennt ihn ebenso.
type WissenEbeneQuery struct {
	Q    string              // Suchtext über Titel, Pfad und Tags
	Sort DocSort             // Sortierwahl (Katalog 3.10)
	Typ  domain.DocumentType // Typ-Filter; "" = keiner
	In   string              // Gruppe, auf die der Typ-Filter wirkt; "" = überall (Repo)
	Open []string            // aufgeklappte Gruppen; wissenOpenAll klappt alle auf
}

// ParseWissenEbeneQuery liest den Zustand aus den Query-Parametern. Unbekannte
// Werte fallen auf den Standard — eine kaputte URL soll die Fläche nicht
// zerreißen.
func ParseWissenEbeneQuery(v url.Values) WissenEbeneQuery {
	q := WissenEbeneQuery{
		Q:    strings.TrimSpace(v.Get("q")),
		Sort: NormalizeDocSort(v.Get("sort")),
		In:   v.Get("in"),
	}
	if t := domain.DocumentType(v.Get("typ")); knownDocType(t) {
		q.Typ = t
	}
	if q.Typ == "" {
		q.In = ""
	}
	for _, o := range v["open"] {
		if o != "" && !q.opened(o) {
			q.Open = append(q.Open, o)
		}
	}
	return q
}

func knownDocType(t domain.DocumentType) bool {
	for _, k := range domain.DocumentTypes() {
		if k == t {
			return true
		}
	}
	return false
}

func (q WissenEbeneQuery) values() url.Values {
	v := url.Values{}
	if q.Q != "" {
		v.Set("q", q.Q)
	}
	if q.Sort != SortChanged {
		v.Set("sort", string(q.Sort))
	}
	if q.Typ != "" {
		v.Set("typ", string(q.Typ))
		if q.In != "" {
			v.Set("in", q.In)
		}
	}
	for _, o := range q.Open {
		v.Add("open", o)
	}
	return v
}

func (q WissenEbeneQuery) opened(id string) bool {
	for _, o := range q.Open {
		if o == wissenOpenAll || o == id {
			return true
		}
	}
	return false
}

func (q WissenEbeneQuery) withOpen(id string) WissenEbeneQuery {
	if q.opened(id) {
		return q
	}
	q.Open = append(append([]string(nil), q.Open...), id)
	return q
}

func (q WissenEbeneQuery) withoutOpen(id string) WissenEbeneQuery {
	out := make([]string, 0, len(q.Open))
	for _, o := range q.Open {
		if o != id && o != wissenOpenAll {
			out = append(out, o)
		}
	}
	q.Open = out
	return q
}

// toggleTyp setzt den Typ-Filter für eine Gruppe — oder hebt ihn auf, wenn
// genau dieser schon aktiv ist.
func (q WissenEbeneQuery) toggleTyp(t domain.DocumentType, in string) WissenEbeneQuery {
	if q.Typ == t && q.In == in {
		q.Typ, q.In = "", ""
		return q
	}
	q.Typ, q.In = t, in
	return q
}

// filterFor sagt, ob der Typ-Filter auf die Gruppe gid wirkt.
func (q WissenEbeneQuery) filterFor(gid string) bool {
	return q.Typ != "" && (q.In == "" || q.In == gid)
}

// WissenEbenePageHref ist die Seite mit diesem Zustand (Lesezeichen, Zurück).
func WissenEbenePageHref(nodeID string, q WissenEbeneQuery) string {
	v := q.values()
	v.Set("tab", "wissen")
	return "/nodes/" + nodeID + "?" + v.Encode()
}

// WissenEbeneFragmentHref ist das Fragment mit diesem Zustand (htmx, SSE).
func WissenEbeneFragmentHref(nodeID string, q WissenEbeneQuery) string {
	href := "/nodes/" + nodeID + "/wissen"
	if s := q.values().Encode(); s != "" {
		href += "?" + s
	}
	return href
}

// WissenEbeneInput ist alles, was die Fläche braucht — die Domäne liefert,
// der Builder ordnet.
type WissenEbeneInput struct {
	N             domain.Node
	Subtree       []domain.Node     // samt N; leer → nur N
	Docs          []domain.Document // aktive Karten des Besitzers; gefiltert wird hier
	Now           time.Time
	Query         WissenEbeneQuery
	ArchivedTotal int
}

// WissenTypZaehler ist ein Typ mit Anzahl, als Filter klickbar.
type WissenTypZaehler struct {
	Type     domain.DocumentType
	Label    string
	Count    int
	Href     string // Seite mit umgeschaltetem Filter
	Fragment string // Fragment mit umgeschaltetem Filter
	Active   bool
}

// WissenEbeneRow ist eine Karte in der Liste.
type WissenEbeneRow struct {
	ID, Title, Path, Actor string
	ChipClass, ChipLabel   string
	When                   string // Datumsstaffel
	ReadTime               string
	// Mode ist die Kuratier-Marke (auto/immer/nie) — nur bei kontextfähigen
	// Typen gesetzt. Sichtbar hier, bedienbar auf der Kuratieren-Fläche.
	Mode domain.ContextMode
}

// WissenGruppe ist eine Herkunft: ein Register mit seinen Karten.
type WissenGruppe struct {
	NodeID, Name string
	Kind         domain.NodeKind
	Self         bool // die Ebene selbst
	Total        int  // Karten der Gruppe vor Filter und Suche
	Matching     int  // Karten nach Filter und Suche, vor der Kappe
	Types        []WissenTypZaehler
	Rows         []WissenEbeneRow
	Hidden       int  // Zeilen hinter der Kappe
	Expanded     bool // aufgeklappt (open, Filter oder Suche)
	Collapsible  bool // per ?open aufgeklappt und groß genug, um wieder zuzuklappen
	MoreHref     string
	MoreFragment string
	LessHref     string
	LessFragment string
	NewHref      string
	Newest       time.Time
}

// WissenEbeneVM treibt die Fläche und ihr Fragment.
type WissenEbeneVM struct {
	NodeID, NodeName string
	Kind             domain.NodeKind
	Grouped          bool // false am Repo: eine Herkunft, keine Gruppenköpfe
	Total            int  // Karten in Sicht vor Filter und Suche
	Matching         int  // Karten nach Filter und Suche
	Filtered         bool // Suche oder Typ-Filter aktiv
	ArchivedTotal    int
	Query            WissenEbeneQuery
	Sort             DocSort
	Types            []WissenTypZaehler // über alles — am Repo die Filterleiste
	Groups           []WissenGruppe
	PageHref         string // die Seite mit dem vollen Zustand
	FragmentHref     string // das Fragment mit dem vollen Zustand (SSE-Neuladen)
	SortBaseHref     string // die Seite ohne Sortierung — Basis des Sortierkopfs
	ResetHref        string
	ManagerHref      string
	CurateHref       string
	NewHref          string
}

// BuildWissenEbene ordnet die Karten eines Teilbaums nach Herkunft. Rein:
// keine Stores, keine Uhr außer in.Now.
func BuildWissenEbene(ctx context.Context, in WissenEbeneInput) WissenEbeneVM {
	q := in.Query
	vm := WissenEbeneVM{
		NodeID:        in.N.ID,
		NodeName:      in.N.Name,
		Kind:          in.N.Kind,
		Grouped:       in.N.Kind != domain.KindRepo,
		ArchivedTotal: in.ArchivedTotal,
		Query:         q,
		Sort:          q.Sort,
		PageHref:      WissenEbenePageHref(in.N.ID, q),
		FragmentHref:  WissenEbeneFragmentHref(in.N.ID, q),
		ManagerHref:   "/wissen?node=" + url.QueryEscape(in.N.ID) + "&scope=" + wissenEbeneScope(in.N.Kind),
		CurateHref:    "/kontext/" + in.N.ID,
		NewHref:       "/wissen/neu?node=" + in.N.ID + "&type=project",
	}
	noSort := q
	noSort.Sort = SortChanged
	vm.SortBaseHref = WissenEbenePageHref(in.N.ID, noSort)
	vm.ResetHref = WissenEbenePageHref(in.N.ID, WissenEbeneQuery{Sort: q.Sort})
	vm.Filtered = q.Q != "" || q.Typ != ""

	// Herkünfte: am Repo nur das Repo selbst — ein Repo hat keine Kinder,
	// und selbst wenn der Aufrufer einen Teilbaum mitgibt, bleibt die Sicht
	// flach.
	nodes := []domain.Node{in.N}
	if vm.Grouped && len(in.Subtree) > 0 {
		nodes = in.Subtree
	}
	byNode := make(map[string][]domain.Document, len(nodes))
	order := make([]domain.Node, 0, len(nodes))
	seen := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		if seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		order = append(order, n)
		byNode[n.ID] = nil
	}
	if !seen[in.N.ID] {
		order = append([]domain.Node{in.N}, order...)
		byNode[in.N.ID] = nil
	}
	var inScope []domain.Document
	for _, d := range in.Docs {
		if d.Archived || d.NodeID == nil {
			continue
		}
		if _, ok := byNode[*d.NodeID]; !ok {
			continue
		}
		byNode[*d.NodeID] = append(byNode[*d.NodeID], d)
		inScope = append(inScope, d)
	}
	vm.Total = len(inScope)
	vm.Types = buildTypZaehler(in.N.ID, inScope, q, "")

	groups := make([]WissenGruppe, 0, len(order))
	for _, n := range order {
		docs := byNode[n.ID]
		self := n.ID == in.N.ID
		if len(docs) == 0 && !self {
			continue
		}
		groups = append(groups, buildWissenGruppe(ctx, in, n, self, docs))
	}
	// Nach Frische sortiert; die leere eigene Ebene bleibt als Hinweis am Ende.
	sort.SliceStable(groups, func(i, j int) bool {
		if (groups[i].Total == 0) != (groups[j].Total == 0) {
			return groups[i].Total > 0
		}
		return groups[j].Newest.Before(groups[i].Newest)
	})
	for _, g := range groups {
		vm.Matching += g.Matching
	}
	vm.Groups = groups
	return vm
}

func wissenEbeneScope(k domain.NodeKind) string {
	if k == domain.KindRepo {
		return "self"
	}
	return "subtree"
}

func buildWissenGruppe(ctx context.Context, in WissenEbeneInput, n domain.Node, self bool, docs []domain.Document) WissenGruppe {
	q := in.Query
	g := WissenGruppe{
		NodeID:  n.ID,
		Name:    ShortName(n.Name),
		Kind:    n.Kind,
		Self:    self,
		Total:   len(docs),
		NewHref: "/wissen/neu?node=" + n.ID + "&type=project",
	}
	for _, d := range docs {
		if d.UpdatedAt.After(g.Newest) {
			g.Newest = d.UpdatedAt
		}
	}
	// Am Repo wirkt der Typ-Filter ohne Gruppe (in=""), sonst auf genau
	// diese Herkunft.
	filterIn := n.ID
	if in.N.Kind == domain.KindRepo {
		filterIn = ""
	}
	g.Types = buildTypZaehler(in.N.ID, docs, q, filterIn)

	filterHere := q.filterFor(n.ID)
	matching := make([]domain.Document, 0, len(docs))
	needle := strings.ToLower(q.Q)
	for _, d := range docs {
		if filterHere && d.Type != q.Typ {
			continue
		}
		if needle != "" && !docMatches(d, needle) {
			continue
		}
		matching = append(matching, d)
	}
	g.Matching = len(matching)
	sorted := SortDocuments(matching, q.Sort)

	opened := q.opened(n.ID)
	g.Expanded = opened || filterHere || needle != ""
	if !g.Expanded && len(sorted) > wissenGruppeCap {
		g.Hidden = len(sorted) - wissenGruppeCap
		sorted = sorted[:wissenGruppeCap]
	}
	g.Collapsible = opened && !filterHere && needle == "" && len(sorted) > wissenGruppeCap
	more := q.withOpen(n.ID)
	less := q.withoutOpen(n.ID)
	g.MoreHref = WissenEbenePageHref(in.N.ID, more)
	g.MoreFragment = WissenEbeneFragmentHref(in.N.ID, more)
	g.LessHref = WissenEbenePageHref(in.N.ID, less)
	g.LessFragment = WissenEbeneFragmentHref(in.N.ID, less)

	g.Rows = make([]WissenEbeneRow, 0, len(sorted))
	for _, d := range sorted {
		g.Rows = append(g.Rows, buildWissenEbeneRow(ctx, d, in.Now))
	}
	return g
}

func buildWissenEbeneRow(ctx context.Context, d domain.Document, now time.Time) WissenEbeneRow {
	r := WissenEbeneRow{
		ID:        d.ID,
		Title:     d.Title,
		Path:      d.Path,
		Actor:     d.UpdatedByRef,
		ChipClass: DocTypeChipClass(d.Type),
		ChipLabel: DocTypeLabel(d.Type),
		When:      FmtStaffel(ctx, d.UpdatedAt, now),
		ReadTime:  readTimeLabel(d.Body),
	}
	if d.Type.ContextEligible() {
		r.Mode = d.ContextMode.OrAuto()
	}
	return r
}

// docMatches prüft Titel, Pfad und Tags — das, was ein Mensch von einer
// Karte im Kopf hat. Der Volltext gehört der Bibliothekssuche.
func docMatches(d domain.Document, needle string) bool {
	if strings.Contains(strings.ToLower(d.Title), needle) || strings.Contains(strings.ToLower(d.Path), needle) {
		return true
	}
	for _, t := range d.Tags {
		if strings.Contains(strings.ToLower(t), needle) {
			return true
		}
	}
	return false
}

// buildTypZaehler zählt die Typen einer Kartenmenge, häufigster zuerst,
// gleich häufige in kanonischer Reihenfolge. Jeder Zähler trägt den Link,
// der den Filter für die Gruppe `in` umschaltet.
func buildTypZaehler(nodeID string, docs []domain.Document, q WissenEbeneQuery, in string) []WissenTypZaehler {
	counts := make(map[domain.DocumentType]int)
	for _, d := range docs {
		counts[d.Type]++
	}
	out := make([]WissenTypZaehler, 0, len(counts))
	for _, t := range domain.DocumentTypes() {
		c := counts[t]
		if c == 0 {
			continue
		}
		toggled := q.toggleTyp(t, in)
		out = append(out, WissenTypZaehler{
			Type:     t,
			Label:    DocTypeLabel(t),
			Count:    c,
			Href:     WissenEbenePageHref(nodeID, toggled),
			Fragment: WissenEbeneFragmentHref(nodeID, toggled),
			Active:   q.Typ == t && q.In == in,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}
