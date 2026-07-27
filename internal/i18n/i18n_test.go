package i18n_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/i18n"
)

func TestT_GermanDefault(t *testing.T) {
	ctx := context.Background() // no locale → Default (de)
	if got := i18n.T(ctx, "nav.today"); got != "Heute" {
		t.Fatalf("T(nav.today) = %q, want Heute", got)
	}
}

func TestT_EnglishLocale(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.EN)
	if got := i18n.T(ctx, "nav.today"); got != "Today" {
		t.Fatalf("T(nav.today, en) = %q, want Today", got)
	}
}

func TestT_MissingKeyReturnsKey(t *testing.T) {
	ctx := context.Background()
	if got := i18n.T(ctx, "does.not.exist"); got != "does.not.exist" {
		t.Fatalf("missing key = %q, want the key itself", got)
	}
}

func TestT_FallsBackToDefaultLocale(t *testing.T) {
	// A key present in de but (intentionally) not in en still resolves via the
	// de fallback rather than echoing the key.
	ctx := i18n.WithLocale(context.Background(), i18n.EN)
	if got := i18n.T(ctx, "common.cancel"); got == "common.cancel" {
		t.Fatalf("expected fallback translation, got raw key")
	}
}

// TestT_ArtifactVerbKeys guards the L6 final-review finding: artifact
// mutations (upload/rename/delete) emit VerbKey "activity.verb.artifact.*"
// into the Home Puls feed (see activity_row.go BuildActivityRows). Before
// this fix those keys didn't exist in either catalog, so T() fell through to
// returning the raw key string — this pins that both locales now resolve to
// real copy.
func TestT_ArtifactVerbKeys(t *testing.T) {
	keys := []string{
		"activity.verb.artifact.created",
		"activity.verb.artifact.updated",
		"activity.verb.artifact.deleted",
	}
	for _, k := range keys {
		if got := i18n.T(context.Background(), k); got == k {
			t.Errorf("T(%q) = raw key, want a translated de string", k)
		}
		ctx := i18n.WithLocale(context.Background(), i18n.EN)
		if got := i18n.T(ctx, k); got == k {
			t.Errorf("T(%q, en) = raw key, want a translated en string", k)
		}
	}
}

func TestTn_Plural(t *testing.T) {
	ctx := context.Background()
	if got := i18n.Tn(ctx, "list.entries", 1); got != "1 Eintrag" {
		t.Fatalf("Tn(list.entries,1) = %q, want '1 Eintrag'", got)
	}
	if got := i18n.Tn(ctx, "list.entries", 3); got != "3 Einträge" {
		t.Fatalf("Tn(list.entries,3) = %q, want '3 Einträge'", got)
	}
}
