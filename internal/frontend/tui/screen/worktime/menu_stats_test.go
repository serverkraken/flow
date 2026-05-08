// White-box tests for the stats flow.

package worktime

import (
	"strings"
	"testing"
)

func TestStatsCmd_DefaultsToMonthWhenRangeEmpty(t *testing.T) {
	r := newBriefRig(t)
	cmd := statsCmd(r.deps, outputTargetClipboard, "")
	msg := cmd()
	done, ok := msg.(menuActionDoneMsg)
	if !ok {
		t.Fatalf("stats cmd returned %T, want menuActionDoneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("stats err = %v", done.err)
	}
	if len(r.out.Copies) != 1 {
		t.Fatalf("Copy must be called once, got %d", len(r.out.Copies))
	}
	body := r.out.Copies[0]
	// First row is "Range:    <expr>"; defaulted expr should be month.
	if !strings.HasPrefix(body, "Range:    month") {
		t.Errorf("default range should be month; first 40 chars: %q", body[:min(40, len(body))])
	}
}

func TestStatsCmd_PagerExtIsTxt(t *testing.T) {
	r := newBriefRig(t)
	cmd := statsCmd(r.deps, outputTargetSplit, "month")
	msg := cmd()
	done := msg.(menuActionDoneMsg)
	if done.err != nil {
		t.Fatalf("stats err = %v", done.err)
	}
	if len(r.out.Pagers) != 1 {
		t.Fatalf("Pager must be called once, got %d", len(r.out.Pagers))
	}
	if r.out.Pagers[0].Ext != "txt" {
		t.Errorf("ext = %q, want txt", r.out.Pagers[0].Ext)
	}
	if r.out.Pagers[0].Viewer != statsPager {
		t.Errorf("viewer = %q, want %q", r.out.Pagers[0].Viewer, statsPager)
	}
}

func TestStatsCmd_SaveFileBasenameSanitised(t *testing.T) {
	r := newBriefRig(t)
	cmd := statsCmd(r.deps, outputTargetFile, "2026-04-01..2026-04-30")
	msg := cmd()
	done := msg.(menuActionDoneMsg)
	if done.err != nil {
		t.Fatalf("stats err = %v", done.err)
	}
	if len(r.out.Saves) != 1 {
		t.Fatalf("SaveFile must be called once, got %d", len(r.out.Saves))
	}
	want := "worktime-stats-2026-04-01-to-2026-04-30"
	if r.out.Saves[0].Basename != want {
		t.Errorf("basename = %q, want %q", r.out.Saves[0].Basename, want)
	}
}

func TestStatsCmd_RejectsInvalidRange(t *testing.T) {
	r := newBriefRig(t)
	cmd := statsCmd(r.deps, outputTargetClipboard, "garbage-range")
	msg := cmd()
	if msg.(menuActionDoneMsg).err == nil {
		t.Error("stats must reject an invalid range")
	}
}

func TestStatsCmd_FailsWithoutDeps(t *testing.T) {
	r := newBriefRig(t)
	r.deps.Stats = nil
	cmd := statsCmd(r.deps, outputTargetClipboard, "month")
	if cmd().(menuActionDoneMsg).err == nil {
		t.Error("stats without Stats must fail cleanly")
	}

	r2 := newBriefRig(t)
	r2.deps.Output = nil
	cmd2 := statsCmd(r2.deps, outputTargetClipboard, "month")
	if cmd2().(menuActionDoneMsg).err == nil {
		t.Error("stats without Output must fail cleanly")
	}
}

// — menu integration: list → range → target → dispatch —

func TestMenu_StatsActionEntersRangeSubMode(t *testing.T) {
	r := newBriefRig(t)
	m := newMenuModel(pal(), r.deps).SetSize(120, 40).openMenu(tabHeute)
	for i, a := range m.filtered {
		if a.kind == menuActionStats {
			m.cursor = i
			break
		}
	}
	m, _ = m.handleKey(keyName("enter"))
	if m.subMode != menuSubModeRange {
		t.Errorf("after Enter on Stats, subMode = %v, want range", m.subMode)
	}
	if m.pending.kind != menuActionStats {
		t.Errorf("pending = %v, want Stats", m.pending.kind)
	}
	if m.rangeF.input.Value() != "month" {
		t.Errorf("range default = %q, want month", m.rangeF.input.Value())
	}
}

func TestMenu_StatsRangeEscReturnsToList(t *testing.T) {
	r := newBriefRig(t)
	m := newMenuModel(pal(), r.deps).SetSize(120, 40).openMenu(tabHeute)
	for i, a := range m.filtered {
		if a.kind == menuActionStats {
			m.cursor = i
			break
		}
	}
	m, _ = m.handleKey(keyName("enter"))
	m, _ = m.handleKey(keyName("esc"))
	if m.subMode != menuSubModeList {
		t.Errorf("Esc in range should return to list, got %v", m.subMode)
	}
	if m.pending.label != "" {
		t.Error("pending must be cleared on Esc from range")
	}
}

func TestMenu_StatsRangeSubmitTransitionsToTarget(t *testing.T) {
	r := newBriefRig(t)
	m := newMenuModel(pal(), r.deps).SetSize(120, 40).openMenu(tabHeute)
	for i, a := range m.filtered {
		if a.kind == menuActionStats {
			m.cursor = i
			break
		}
	}
	m, _ = m.handleKey(keyName("enter"))
	m, _ = m.handleKey(keyName("enter")) // submit default "month"
	if m.subMode != menuSubModeTarget {
		t.Errorf("after range submit, subMode = %v, want target", m.subMode)
	}
	if m.rangeExpr != "month" {
		t.Errorf("rangeExpr = %q, want month", m.rangeExpr)
	}
	out := m.View()
	if !strings.Contains(out, "Aktion · Stats") {
		t.Errorf("target view should keep parent label visible; got:\n%s", out)
	}
}

func TestMenu_StatsFullFlowEndsInToast(t *testing.T) {
	r := newBriefRig(t)
	m := newMenuModel(pal(), r.deps).SetSize(120, 40).openMenu(tabHeute)
	for i, a := range m.filtered {
		if a.kind == menuActionStats {
			m.cursor = i
			break
		}
	}
	m, _ = m.handleKey(keyName("enter")) // open range form
	m, _ = m.handleKey(keyName("enter")) // submit default "month"
	m, cmd := m.handleKey(runeKey('c'))  // pick clipboard target
	if cmd == nil {
		t.Fatal("pick should return a dispatch tea.Cmd")
	}
	msg := cmd()
	done := msg.(menuActionDoneMsg)
	if done.err != nil {
		t.Fatalf("dispatch err = %v", done.err)
	}
	m, _ = m.Update(done)
	if m.toast == nil {
		t.Error("dispatch should attach a toast")
	}
	if len(r.out.Copies) != 1 {
		t.Errorf("Copy should be called once; got %d", len(r.out.Copies))
	}
	if !strings.HasPrefix(r.out.Copies[0], "Range:    month") {
		t.Errorf("clipboard content should be the stats text; got first 40: %q",
			r.out.Copies[0][:min(40, len(r.out.Copies[0]))])
	}
}

func TestMenu_RangeFormInvalidInputStaysInRangeMode(t *testing.T) {
	r := newBriefRig(t)
	m := newMenuModel(pal(), r.deps).SetSize(120, 40).openMenu(tabHeute)
	for i, a := range m.filtered {
		if a.kind == menuActionStats {
			m.cursor = i
			break
		}
	}
	m, _ = m.handleKey(keyName("enter"))
	// Replace the default with garbage, then submit.
	for i := 0; i < len("month"); i++ {
		m, _ = m.handleKey(keyName("backspace"))
	}
	for _, ch := range "garbage" {
		m, _ = m.handleKey(runeKey(ch))
	}
	m, _ = m.handleKey(keyName("enter"))
	if m.subMode != menuSubModeRange {
		t.Errorf("invalid range should stay in range mode, got %v", m.subMode)
	}
	if m.rangeF.errMsg == "" {
		t.Error("range form must show errMsg after invalid submit")
	}
}
