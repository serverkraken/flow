package usecase_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestAllowList(t *testing.T) {
	subs := map[string]bool{"alice": true}
	groups := map[string]bool{"admins": true}
	allow := usecase.AllowList(subs, groups)

	tests := []struct {
		name    string
		id      ports.Identity
		allowed bool
	}{
		{
			name:    "sub match by Username",
			id:      ports.Identity{Username: "alice", Subject: "sub-1"},
			allowed: true,
		},
		{
			name:    "sub match by Subject",
			id:      ports.Identity{Username: "unknown", Subject: "alice"},
			allowed: true,
		},
		{
			name:    "group match",
			id:      ports.Identity{Username: "bob", Subject: "sub-2", Groups: []string{"users", "admins"}},
			allowed: true,
		},
		{
			name:    "sub and group both match",
			id:      ports.Identity{Username: "alice", Subject: "sub-3", Groups: []string{"admins"}},
			allowed: true,
		},
		{
			name:    "no match denied",
			id:      ports.Identity{Username: "eve", Subject: "sub-4", Groups: []string{"guests"}},
			allowed: false,
		},
		{
			name:    "no groups field denied",
			id:      ports.Identity{Username: "nobody", Subject: "sub-5"},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allow(tt.id); got != tt.allowed {
				t.Errorf("AllowList(%v) = %v, want %v", tt.id, got, tt.allowed)
			}
		})
	}
}

func TestAllowListEmptySets(t *testing.T) {
	allow := usecase.AllowList(map[string]bool{}, map[string]bool{})
	id := ports.Identity{Username: "alice", Subject: "sub-1", Groups: []string{"admins"}}
	if allow(id) {
		t.Fatal("AllowList with empty subs and groups should deny everyone")
	}
}
