package markdown

import (
	"reflect"
	"testing"
)

// TestValidWikilinkTargets_SkipsCodeAndUnresolved proves the enumeration is
// byte-for-byte parity with the renderer's validWikilinkIdx counting: wikilinks
// inside fenced code blocks and inline code spans are literal (goldmark does not
// parse them as wikilinks), and unresolvable targets are excluded — exactly as
// the renderer excludes them. Only the resolvable, non-code links survive, in
// render order.
func TestValidWikilinkTargets_SkipsCodeAndUnresolved(t *testing.T) {
	t.Parallel()
	res := stubWikiResolver{known: map[string]string{
		"alpha": "flow://docs/alpha",
		"beta":  "flow://docs/beta",
	}}
	src := "see [[alpha]]\n```\n[[incode]]\n```\nand `[[inspan]]` then [[beta]] and [[broken]]"

	got := ValidWikilinkTargets(src, res)
	want := []string{"alpha", "beta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ValidWikilinkTargets = %#v, want %#v", got, want)
	}
}

// TestValidWikilinkTargets_NilResolver returns no targets when no resolver can
// validate anything (matches the renderer treating every link as broken).
func TestValidWikilinkTargets_NilResolver(t *testing.T) {
	t.Parallel()
	if got := ValidWikilinkTargets("[[a]] [[b]]", nil); got != nil {
		t.Fatalf("nil resolver should yield no valid targets, got %#v", got)
	}
}
