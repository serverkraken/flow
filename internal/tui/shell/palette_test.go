package shell_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
	"github.com/serverkraken/flow/internal/tui/theme"
)

func entries() []shell.PaletteEntry {
	return []shell.PaletteEntry{
		{Label: "Home", Action: func() tea.Msg { return nil }},
		{Label: "Worktime", Action: func() tea.Msg { return nil }},
		{Label: "Docs", Action: func() tea.Msg { return nil }},
	}
}

func TestPalette_filterContains(t *testing.T) {
	p := shell.NewPalette(entries()).SetQuery("work")
	f := p.Filtered()
	if len(f) != 1 || f[0].Label != "Worktime" {
		t.Fatalf("filtered = %v", f)
	}
}

func TestPalette_emptyShowsAll(t *testing.T) {
	if len(shell.NewPalette(entries()).Filtered()) != 3 {
		t.Fatal("empty query shows all")
	}
}

func TestPalette_typingFiltersAndEnterSelects(t *testing.T) {
	p := shell.NewPalette(entries())
	p, _ = p.Update(tea.KeyPressMsg{Text: "d"})
	p, _ = p.Update(tea.KeyPressMsg{Text: "o"})
	if len(p.Filtered()) != 1 || p.Filtered()[0].Label != "Docs" {
		t.Fatalf("after typing 'do': %v", p.Filtered())
	}
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should emit a command")
	}
	if _, ok := cmd().(shell.PaletteSelectedMsg); !ok {
		t.Fatal("enter should emit PaletteSelectedMsg")
	}
}

func TestPalette_escDismisses(t *testing.T) {
	p := shell.NewPalette(entries())
	_, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should emit a command")
	}
	if _, ok := cmd().(shell.PaletteDismissedMsg); !ok {
		t.Fatal("esc should emit PaletteDismissedMsg")
	}
}

func TestPalette_cursorClamp(t *testing.T) {
	p := shell.NewPalette(entries())
	for i := 0; i < 10; i++ {
		p, _ = p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if p.Cursor() >= len(p.Filtered()) {
		t.Fatalf("cursor %d OOB for %d", p.Cursor(), len(p.Filtered()))
	}
}

func TestPalette_viewShowsQuery(t *testing.T) {
	p := shell.NewPalette(entries()).SetQuery("ho")
	if !strings.Contains(p.View(60, theme.Default), "ho") {
		t.Fatal("view should echo query")
	}
}
