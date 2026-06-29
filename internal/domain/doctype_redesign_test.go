package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestRedesignedDocType(t *testing.T) {
	cases := []struct {
		in       string
		wantType domain.DocumentType
		wantPath string
	}{
		{"plans/2026-06-25-foo", domain.DocPlan, "2026-06-25-foo"},
		{"specs/2026-06-25-foo-design", domain.DocSpec, "2026-06-25-foo-design"},
		{"loose-doc", domain.DocSpec, "loose-doc"}, // no prefix -> spec, path unchanged
	}
	for _, c := range cases {
		gotT, gotP := domain.RedesignedDocType(c.in)
		if gotT != c.wantType || gotP != c.wantPath {
			t.Errorf("RedesignedDocType(%q) = (%q,%q), want (%q,%q)", c.in, gotT, gotP, c.wantType, c.wantPath)
		}
	}
}
