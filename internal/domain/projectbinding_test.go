package domain

import "testing"

func TestResolveBinding_Remote(t *testing.T) {
	bs := []ProjectBinding{
		{ProjectID: "p1", Kind: BindingRemote, RemoteSlug: "github.com/a/flow"},
		{ProjectID: "p2", Kind: BindingRemote, RemoteSlug: "github.com/a/other"},
	}
	got, ok := ResolveBinding(bs, "github.com/a/flow", "m1", "/whatever")
	if !ok || got.ProjectID != "p1" {
		t.Fatalf("remote match = %+v,%v want p1", got, ok)
	}
	if _, ok := ResolveBinding(bs, "github.com/a/nope", "m1", "/x"); ok {
		t.Fatal("unknown remote must not match")
	}
	if _, ok := ResolveBinding(bs, "", "m1", "/x"); ok {
		t.Fatal("empty remote must not match a remote binding")
	}
}
