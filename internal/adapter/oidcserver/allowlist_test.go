package oidcserver

import (
	"testing"

	"github.com/serverkraken/flow/internal/ports"
)

func TestUnit_Allowlist_AllowsListedSub(t *testing.T) {
	t.Parallel()
	al := NewSubAllowlist([]string{"sub-1", "sub-2"})
	if !al.Allow(ports.Identity{Sub: "sub-1"}) {
		t.Error("sub-1 should be allowed")
	}
	if !al.Allow(ports.Identity{Sub: "sub-2"}) {
		t.Error("sub-2 should be allowed")
	}
}

func TestUnit_Allowlist_RejectsUnlistedSub(t *testing.T) {
	t.Parallel()
	al := NewSubAllowlist([]string{"sub-1"})
	if al.Allow(ports.Identity{Sub: "sub-other"}) {
		t.Error("sub-other should be rejected")
	}
}

func TestUnit_Allowlist_EmptyList_RejectsEverything(t *testing.T) {
	t.Parallel()
	al := NewSubAllowlist(nil)
	if al.Allow(ports.Identity{Sub: "anyone"}) {
		t.Error("empty allowlist must reject all")
	}
}

func TestUnit_Allowlist_EmptyStringInList_Ignored(t *testing.T) {
	t.Parallel()
	al := NewSubAllowlist([]string{"", "valid"})
	if al.Allow(ports.Identity{Sub: ""}) {
		t.Error("empty sub must never be allowed")
	}
	if !al.Allow(ports.Identity{Sub: "valid"}) {
		t.Error("valid sub should still pass")
	}
}
