package domain

import "testing"

func TestResolveBinding_Remote(t *testing.T) {
	bs := []ProjectBinding{
		{NodeID: "p1", Kind: BindingRemote, RemoteSlug: "github.com/a/flow"},
		{NodeID: "p2", Kind: BindingRemote, RemoteSlug: "github.com/a/other"},
	}
	got, ok := ResolveBinding(bs, "github.com/a/flow", "m1", "/whatever")
	if !ok || got.NodeID != "p1" {
		t.Fatalf("remote match = %+v,%v want p1", got, ok)
	}
	if _, ok := ResolveBinding(bs, "github.com/a/nope", "m1", "/x"); ok {
		t.Fatal("unknown remote must not match")
	}
	if _, ok := ResolveBinding(bs, "", "m1", "/x"); ok {
		t.Fatal("empty remote must not match a remote binding")
	}
}

func TestResolveBinding_Path(t *testing.T) {
	bs := []ProjectBinding{
		{NodeID: "pa", Kind: BindingPath, MachineID: "m1", Path: "/home/u/code"},
		{NodeID: "pb", Kind: BindingPath, MachineID: "m1", Path: "/home/u/code/flow"},
		{NodeID: "pc", Kind: BindingPath, MachineID: "m2", Path: "/home/u/code/flow"}, // other machine
	}
	// longest-prefix wins, machine-scoped
	if got, ok := ResolveBinding(bs, "", "m1", "/home/u/code/flow/sub"); !ok || got.NodeID != "pb" {
		t.Fatalf("longest prefix m1: %+v %v (want pb)", got, ok)
	}
	// shorter prefix when not under the longer one
	if got, ok := ResolveBinding(bs, "", "m1", "/home/u/code/other"); !ok || got.NodeID != "pa" {
		t.Fatalf("shorter prefix: %+v %v (want pa)", got, ok)
	}
	// exact match
	if got, ok := ResolveBinding(bs, "", "m1", "/home/u/code/flow"); !ok || got.NodeID != "pb" {
		t.Fatalf("exact: %+v %v (want pb)", got, ok)
	}
	// segment boundary: /home/u/codex must NOT match /home/u/code
	if _, ok := ResolveBinding(bs, "", "m1", "/home/u/codex"); ok {
		t.Fatal("/home/u/codex must not match /home/u/code")
	}
	// machine isolation: m2's binding not used for m1
	if got, ok := ResolveBinding(bs, "", "m1", "/home/u/code/flow"); ok && got.NodeID == "pc" {
		t.Fatal("must not cross machines")
	}
	// no match
	if _, ok := ResolveBinding(bs, "", "m1", "/elsewhere"); ok {
		t.Fatal("no path match expected")
	}
}

func TestResolveBinding_RemoteBeatsPath(t *testing.T) {
	bs := []ProjectBinding{
		{NodeID: "pp", Kind: BindingPath, MachineID: "m1", Path: "/home/u/code/flow"},
		{NodeID: "rr", Kind: BindingRemote, RemoteSlug: "github.com/a/flow"},
	}
	if got, ok := ResolveBinding(bs, "github.com/a/flow", "m1", "/home/u/code/flow"); !ok || got.NodeID != "rr" {
		t.Fatalf("remote must beat path: %+v (want rr)", got)
	}
}
