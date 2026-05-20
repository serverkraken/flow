package cheatsheet_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/frontend/tui/screen/cheatsheet"
)

func TestHelpSections_NonEmpty(t *testing.T) {
	sections := cheatsheet.Model{}.HelpSections()
	if len(sections) == 0 {
		t.Fatalf("HelpSections should not be empty")
	}
	for _, s := range sections {
		if s.Title == "" || len(s.Keys) == 0 {
			t.Errorf("section %+v missing title or keys", s)
		}
	}
}
