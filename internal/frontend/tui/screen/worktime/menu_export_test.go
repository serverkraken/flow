// White-box tests for the export flow (CSV / JSON via the Output port).

package worktime

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/testutil"
)

func TestExportFormat_ExtAndLabel(t *testing.T) {
	cases := []struct {
		f          exportFormat
		ext, label string
	}{
		{exportFormatCSV, "csv", "CSV"},
		{exportFormatJSON, "json", "JSON"},
	}
	for _, c := range cases {
		if got := c.f.ext(); got != c.ext {
			t.Errorf("ext(%v) = %q, want %q", c.f, got, c.ext)
		}
		if got := c.f.label(); got != c.label {
			t.Errorf("label(%v) = %q, want %q", c.f, got, c.label)
		}
	}
}

func TestExportBasename_ScopesAndFormats(t *testing.T) {
	cases := []struct {
		expr   string
		format exportFormat
		want   string
	}{
		{"month", exportFormatCSV, "worktime-export-csv-month"},
		{"month", exportFormatJSON, "worktime-export-json-month"},
		{"", exportFormatCSV, "worktime-export-csv-all"},
		{"2026-04-01..2026-04-30", exportFormatCSV, "worktime-export-csv-2026-04-01-to-2026-04-30"},
	}
	for _, c := range cases {
		if got := exportBasename(c.expr, c.format); got != c.want {
			t.Errorf("exportBasename(%q, %v) = %q, want %q", c.expr, c.format, got, c.want)
		}
	}
}

func TestSanitizeRangeForFilename_ReplacesUnsafeTokens(t *testing.T) {
	if got := sanitizeRangeForFilename("2026-04-01..2026-04-30"); got != "2026-04-01-to-2026-04-30" {
		t.Errorf("got %q, want 2026-04-01-to-2026-04-30", got)
	}
	if got := sanitizeRangeForFilename("a/b"); got != "a-b" {
		t.Errorf("slashes must be replaced; got %q", got)
	}
}

func TestExportCmd_RoutesCSVToClipboard(t *testing.T) {
	r := newBriefRig(t)
	cmd := exportCmd(r.deps, outputTargetClipboard, "month", exportFormatCSV)
	msg := cmd()
	done, ok := msg.(menuActionDoneMsg)
	if !ok {
		t.Fatalf("export cmd returned %T, want menuActionDoneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("export err = %v", done.err)
	}
	if len(r.out.Copies) != 1 {
		t.Fatalf("Copy must be called once, got %d", len(r.out.Copies))
	}
	body := r.out.Copies[0]
	// CSV header — sanity that we got the right format.
	if !strings.HasPrefix(body, "date,start,stop") {
		t.Errorf("CSV body should start with header row; got first 80 chars: %q",
			body[:min(80, len(body))])
	}
}

func TestExportCmd_JSONRoutesToFile(t *testing.T) {
	r := newBriefRig(t)
	cmd := exportCmd(r.deps, outputTargetFile, "month", exportFormatJSON)
	msg := cmd()
	done := msg.(menuActionDoneMsg)
	if done.err != nil {
		t.Fatalf("export err = %v", done.err)
	}
	if len(r.out.Saves) != 1 {
		t.Fatalf("SaveFile must be called once, got %d", len(r.out.Saves))
	}
	save := r.out.Saves[0]
	if save.Ext != "json" {
		t.Errorf("SaveFile ext = %q, want json", save.Ext)
	}
	if !strings.Contains(save.Basename, "json") || !strings.Contains(save.Basename, "month") {
		t.Errorf("SaveFile basename = %q, want json+month", save.Basename)
	}
}

func TestExportCmd_PagerCarriesCorrectViewer(t *testing.T) {
	r := newBriefRig(t)
	cmd := exportCmd(r.deps, outputTargetSplit, "month", exportFormatCSV)
	msg := cmd()
	done := msg.(menuActionDoneMsg)
	if done.err != nil {
		t.Fatalf("export err = %v", done.err)
	}
	if len(r.out.Pagers) != 1 {
		t.Fatalf("Pager must be called once, got %d", len(r.out.Pagers))
	}
	if r.out.Pagers[0].Viewer != exportPager {
		t.Errorf("viewer = %q, want %q", r.out.Pagers[0].Viewer, exportPager)
	}
	if r.out.Pagers[0].Ext != "csv" {
		t.Errorf("ext = %q, want csv", r.out.Pagers[0].Ext)
	}
}

func TestExportCmd_RejectsInvalidRange(t *testing.T) {
	r := newBriefRig(t)
	cmd := exportCmd(r.deps, outputTargetClipboard, "garbage-range", exportFormatCSV)
	msg := cmd()
	done := msg.(menuActionDoneMsg)
	if done.err == nil {
		t.Error("export must reject an invalid range expression")
	}
}

func TestExportCmd_FailsWithoutDeps(t *testing.T) {
	r := newBriefRig(t)
	r.deps.Reporter = nil
	cmd := exportCmd(r.deps, outputTargetClipboard, "month", exportFormatCSV)
	msg := cmd()
	if msg.(menuActionDoneMsg).err == nil {
		t.Error("export without Reporter must fail cleanly")
	}

	r2 := newBriefRig(t)
	r2.deps.Output = &testutil.FakeOutput{}
	r2.deps.Output = nil
	cmd2 := exportCmd(r2.deps, outputTargetClipboard, "month", exportFormatCSV)
	if cmd2().(menuActionDoneMsg).err == nil {
		t.Error("export without Output must fail cleanly")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
