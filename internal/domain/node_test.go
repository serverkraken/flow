package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestValidParentKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		child, parent domain.NodeKind
		want         bool
	}{
		{"engagement never has a parent", domain.KindEngagement, domain.KindEngagement, false},
		{"engagement not under repo", domain.KindEngagement, domain.KindRepo, false},
		{"vorhaben under engagement", domain.KindVorhaben, domain.KindEngagement, true},
		{"vorhaben under vorhaben", domain.KindVorhaben, domain.KindVorhaben, true},
		{"vorhaben not under repo", domain.KindVorhaben, domain.KindRepo, false},
		{"repo under engagement", domain.KindRepo, domain.KindEngagement, true},
		{"repo under vorhaben", domain.KindRepo, domain.KindVorhaben, true},
		{"repo not under repo", domain.KindRepo, domain.KindRepo, false},
		{"branch under repo", domain.KindBranch, domain.KindRepo, true},
		{"branch not under engagement", domain.KindBranch, domain.KindEngagement, false},
		{"branch not under vorhaben", domain.KindBranch, domain.KindVorhaben, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.ValidParentKind(c.child, c.parent); got != c.want {
				t.Errorf("ValidParentKind(%s,%s)=%v want %v", c.child, c.parent, got, c.want)
			}
			if got := domain.AllowedChildKind(c.parent, c.child); got != c.want {
				t.Errorf("AllowedChildKind(%s,%s)=%v want %v", c.parent, c.child, got, c.want)
			}
		})
	}
}

func TestIsBookable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind domain.NodeKind
		want bool
	}{
		{domain.KindEngagement, true},
		{domain.KindVorhaben, true},
		{domain.KindRepo, true},
		{domain.KindBranch, false},
		{domain.NodeKind("unknown"), false},
	}
	for _, c := range cases {
		if got := domain.IsBookable(c.kind); got != c.want {
			t.Errorf("IsBookable(%s)=%v want %v", c.kind, got, c.want)
		}
	}
}

func TestResolveEngagement(t *testing.T) {
	t.Parallel()
	p := "p"
	chain := []domain.Node{
		{ID: "repo", Kind: domain.KindRepo, ParentID: &p},
		{ID: "p", Kind: domain.KindEngagement, Name: "Privat"},
	}
	eng, ok := domain.ResolveEngagement(chain)
	if !ok || eng.ID != "p" {
		t.Fatalf("want engagement p, got %+v ok=%v", eng, ok)
	}
	if _, ok := domain.ResolveEngagement(nil); ok {
		t.Error("empty chain must be ok=false")
	}
	noEng := []domain.Node{{ID: "repo", Kind: domain.KindRepo}}
	if _, ok := domain.ResolveEngagement(noEng); ok {
		t.Error("chain whose root is not an engagement must be ok=false")
	}
}
