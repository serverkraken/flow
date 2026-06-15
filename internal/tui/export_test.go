package tui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestExportPresetRange(t *testing.T) {
	// Montag 2026-06-15 als "now".
	now := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)
	cases := []struct {
		preset, from, to string
	}{
		{"monat", "2026-06-01", "2026-06-15"},
		{"kw", "2026-06-15", "2026-06-15"},      // Montag → from==today
		{"letzter", "2026-05-01", "2026-05-31"}, // ganzer Vormonat
	}
	for _, c := range cases {
		from, to := exportPresetRange(c.preset, now)
		if from != c.from || to != c.to {
			t.Errorf("%s: got %s..%s want %s..%s", c.preset, from, to, c.from, c.to)
		}
	}
}

func TestExportPresetRange_KWMidweek(t *testing.T) {
	// Mittwoch 2026-06-17 → KW-Start Montag 2026-06-15.
	now := time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)
	from, to := exportPresetRange("kw", now)
	if from != "2026-06-15" || to != "2026-06-17" {
		t.Errorf("kw midweek: got %s..%s want 2026-06-15..2026-06-17", from, to)
	}
}

func TestDefaultExportPath(t *testing.T) {
	got := defaultExportPath("2026-06-01", "2026-06-30", "md")
	if got != "~/Downloads/flow-export-2026-06-01_2026-06-30.md" {
		t.Errorf("got %q", got)
	}
	if g := defaultExportPath("2026-06-01", "2026-06-30", "csv"); !strings.HasSuffix(g, ".csv") {
		t.Errorf("csv ext: got %q", g)
	}
}

func TestExpandHome(t *testing.T) {
	if got := expandHome("/tmp/x.csv"); got != "/tmp/x.csv" {
		t.Errorf("absolute path must pass through: %q", got)
	}
	got := expandHome("~/Downloads/x.csv")
	if strings.HasPrefix(got, "~") || !strings.HasSuffix(got, "/Downloads/x.csv") {
		t.Errorf("~ not expanded: %q", got)
	}
}

func TestCycleFormatAndPreset(t *testing.T) {
	if cycleFormat("csv", +1) != "json" || cycleFormat("json", +1) != "md" || cycleFormat("md", +1) != "csv" {
		t.Error("cycleFormat forward wrong")
	}
	if cycleFormat("csv", -1) != "md" {
		t.Error("cycleFormat backward wrong")
	}
	if cyclePreset("kw", +1) != "monat" || cyclePreset("monat", +1) != "letzter" || cyclePreset("letzter", +1) != "custom" || cyclePreset("custom", +1) != "kw" {
		t.Error("cyclePreset forward wrong")
	}
	if cyclePreset("kw", -1) != "custom" {
		t.Error("cyclePreset backward wrong")
	}
}

func TestExportPresetRange_LetzterJanuaryBoundary(t *testing.T) {
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	from, to := exportPresetRange("letzter", now)
	if from != "2025-12-01" || to != "2025-12-31" {
		t.Errorf("letzter in January: got %s..%s want 2025-12-01..2025-12-31", from, to)
	}
}

func TestExportOpenSetsDefaults(t *testing.T) {
	m := New(nil, "tester")
	m.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) // Montag
	next, _ := m.Update(tea.KeyPressMsg{Text: "e"})
	mm := next.(Model)
	if !mm.showExport {
		t.Fatal("e should open the export overlay")
	}
	if mm.expFormat != "md" {
		t.Errorf("default format md, got %q", mm.expFormat)
	}
	if mm.expPreset != "monat" {
		t.Errorf("default preset monat, got %q", mm.expPreset)
	}
	if mm.expFrom != "2026-06-01" || mm.expTo != "2026-06-15" {
		t.Errorf("default range got %s..%s", mm.expFrom, mm.expTo)
	}
	if mm.expPath != "~/Downloads/flow-export-2026-06-01_2026-06-15.md" {
		t.Errorf("default path got %q", mm.expPath)
	}
}

func TestExportEscCloses(t *testing.T) {
	m := New(nil, "tester")
	next, _ := m.Update(tea.KeyPressMsg{Text: "e"})
	m = next.(Model)
	next2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if next2.(Model).showExport {
		t.Fatal("esc should close the export overlay")
	}
}

func TestExportViewRenders(t *testing.T) {
	m := New(nil, "tester")
	m.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	next, _ := m.Update(tea.KeyPressMsg{Text: "e"})
	out := next.(Model).View().Content
	for _, want := range []string{"Export", "Format", "2026-06-01", "md"} {
		if !strings.Contains(out, want) {
			t.Errorf("export view missing %q:\n%s", want, out)
		}
	}
}

func TestMainViewFooterHasExportHint(t *testing.T) {
	m := New(nil, "tester")
	if !strings.Contains(m.View().Content, "e export") {
		t.Errorf("main footer missing 'e export':\n%s", m.View().Content)
	}
}

func openExportM(t *testing.T) Model {
	t.Helper()
	m := New(nil, "tester")
	m.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) // Montag
	next, _ := m.Update(tea.KeyPressMsg{Text: "e"})
	return next.(Model)
}

func TestExportTabMovesFocus(t *testing.T) {
	m := openExportM(t)
	if m.expFocus != 0 {
		t.Fatalf("start focus 0, got %d", m.expFocus)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if next.(Model).expFocus != 1 {
		t.Fatalf("tab → focus 1, got %d", next.(Model).expFocus)
	}
}

func TestExportPresetCycleUpdatesRange(t *testing.T) {
	m := openExportM(t) // focus 0 = preset, preset=monat
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	mm := next.(Model)
	if mm.expPreset != "letzter" {
		t.Fatalf("right on preset: monat → letzter, got %q", mm.expPreset)
	}
	if mm.expFrom != "2026-05-01" || mm.expTo != "2026-05-31" {
		t.Errorf("letzter range got %s..%s", mm.expFrom, mm.expTo)
	}
	if !strings.Contains(mm.expPath, "2026-05-01_2026-05-31") {
		t.Errorf("path should follow range, got %q", mm.expPath)
	}
}

func TestExportFormatCycleUpdatesPathExt(t *testing.T) {
	m := openExportM(t)
	for i := 0; i < 3; i++ {
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = next.(Model)
	}
	if m.expFocus != 3 {
		t.Fatalf("focus should be 3 (format), got %d", m.expFocus)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	mm := next.(Model)
	if mm.expFormat != "csv" { // md →(+1) csv
		t.Fatalf("md →+1 csv, got %q", mm.expFormat)
	}
	if !strings.HasSuffix(mm.expPath, ".csv") {
		t.Errorf("path ext should follow format, got %q", mm.expPath)
	}
}

func TestExportCustomDateEditSetsCustomAndPath(t *testing.T) {
	m := openExportM(t)
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // focus 1 = von
	m = next.(Model)
	for i := 0; i < 10; i++ {
		n, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		m = n.(Model)
	}
	for _, ch := range "2026-06-10" {
		n, _ := m.Update(tea.KeyPressMsg{Text: string(ch)})
		m = n.(Model)
	}
	if m.expFrom != "2026-06-10" {
		t.Fatalf("von edit got %q", m.expFrom)
	}
	if m.expPreset != "custom" {
		t.Errorf("editing date should set preset=custom, got %q", m.expPreset)
	}
	if !strings.Contains(m.expPath, "2026-06-10") {
		t.Errorf("path should follow edited date, got %q", m.expPath)
	}
}

func TestExportManualPathEditSticks(t *testing.T) {
	m := openExportM(t)
	for i := 0; i < 4; i++ { // focus 4 = pfad
		n, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = n.(Model)
	}
	n, _ := m.Update(tea.KeyPressMsg{Text: "X"})
	m = n.(Model)
	if !m.expPathEdited {
		t.Fatal("editing path should set expPathEdited")
	}
	editedPath := m.expPath
	// Tab back around to the Format field (index 3) and cycle it.
	for m.expFocus != 3 {
		n2, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = n2.(Model)
	}
	n3, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = n3.(Model)
	if m.expPath != editedPath {
		t.Errorf("manual path must stick after format change: got %q want %q", m.expPath, editedPath)
	}
}

func TestSubmitExportInvalidDate(t *testing.T) {
	m := openExportM(t)
	m.expPreset = "custom"
	m.expFrom = "not-a-date"
	m.expTo = "2026-06-30"
	next, cmd := m.submitExport()
	if cmd != nil {
		t.Fatal("invalid date should not dispatch a command")
	}
	if next.(Model).expStatus == "" {
		t.Fatal("invalid date should set an inline status")
	}
}

func TestSubmitExportToBeforeFrom(t *testing.T) {
	m := openExportM(t)
	m.expPreset = "custom"
	m.expFrom = "2026-06-30"
	m.expTo = "2026-06-01"
	_, cmd := m.submitExport()
	if cmd != nil {
		t.Fatal("to<from should not dispatch a command")
	}
}

func TestSubmitExportNilClientReportsStatus(t *testing.T) {
	// openExportM uses New(nil, …): valid default dates, no client.
	m := openExportM(t)
	next, cmd := m.submitExport()
	if cmd != nil {
		t.Fatal("nil client should not dispatch a command")
	}
	if !strings.Contains(next.(Model).expStatus, "kein Server") {
		t.Errorf("nil client should set a status, got %q", next.(Model).expStatus)
	}
}

func TestExportDoneMsgSetsStatus(t *testing.T) {
	m := openExportM(t)
	next, _ := m.Update(exportDoneMsg{path: "/tmp/flow-export.md"})
	if !strings.Contains(next.(Model).expStatus, "/tmp/flow-export.md") {
		t.Errorf("done status should contain path, got %q", next.(Model).expStatus)
	}
}

func TestExportErrMsgSetsStatus(t *testing.T) {
	m := openExportM(t)
	next, _ := m.Update(exportErrMsg{err: errExportTest})
	if !strings.Contains(next.(Model).expStatus, "boom") {
		t.Errorf("err status should contain error, got %q", next.(Model).expStatus)
	}
}

var errExportTest = &exportTestErr{}

type exportTestErr struct{}

func (*exportTestErr) Error() string { return "boom" }

func TestExportCmdWritesFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/export", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("# Worktime\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "out.md")
	m := New(apiclient.New(srv.URL, "tok"), "tester")
	m.expFrom, m.expTo, m.expFormat, m.expPath = "2026-06-01", "2026-06-30", "md", target

	msg := m.exportCmd()()
	done, ok := msg.(exportDoneMsg)
	if !ok {
		t.Fatalf("want exportDoneMsg, got %T (%v)", msg, msg)
	}
	if done.path != target {
		t.Errorf("done path %q want %q", done.path, target)
	}
	b, err := os.ReadFile(target)
	if err != nil || string(b) != "# Worktime\n" {
		t.Fatalf("file content %q err %v", b, err)
	}
}
