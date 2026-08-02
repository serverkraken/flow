package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/config"
)

// captureWarnings runs fn with slog's default logger redirected to a buffer
// and returns everything logged, restoring the previous default afterward.
func captureWarnings(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)
	fn()
	return buf.String()
}

func TestWarnOnUnverifiableMachineOwners(t *testing.T) {
	tests := []struct {
		name      string
		cfg       config.Config
		wantWarn  bool
		wantLabel string
		wantOwner string
	}{
		{
			name: "no allowlist at all — cannot prove anything, must not warn",
			cfg: config.Config{
				MachineAccounts: map[string]config.MachineAccount{
					"machine-sub": {OwnerSub: "owner-sub", Label: "ci-runner"},
				},
			},
		},
		{
			name: "owner allowed by subject — fine",
			cfg: config.Config{
				AllowedSubs: map[string]bool{"owner-sub": true},
				MachineAccounts: map[string]config.MachineAccount{
					"machine-sub": {OwnerSub: "owner-sub", Label: "ci-runner"},
				},
			},
		},
		{
			name: "owner missing from sub allowlist, no groups — plausible revocation, warn",
			cfg: config.Config{
				AllowedSubs: map[string]bool{"someone-else": true},
				MachineAccounts: map[string]config.MachineAccount{
					"machine-sub": {OwnerSub: "owner-sub", Label: "ci-runner"},
				},
			},
			wantWarn:  true,
			wantLabel: "ci-runner",
			wantOwner: "owner-sub",
		},
		{
			name: "owner missing from sub allowlist but groups configured — cannot prove, must not warn",
			cfg: config.Config{
				AllowedSubs:   map[string]bool{"someone-else": true},
				AllowedGroups: map[string]bool{"flow-users": true},
				MachineAccounts: map[string]config.MachineAccount{
					"machine-sub": {OwnerSub: "owner-sub", Label: "ci-runner"},
				},
			},
		},
		{
			name: "no machine accounts configured — nothing to check",
			cfg: config.Config{
				AllowedSubs: map[string]bool{"someone-else": true},
			},
		},
		{
			// Regression case for the review finding: usecase.AllowList
			// (internal/usecase/allow.go) matches on Username OR Subject, and
			// username-keyed allowlisting is first-class (see
			// internal/config/config_test.go's "msoent, alice" case).
			// config.MachineAccount only carries the owner's OIDC subject, so
			// this shape is structurally unverifiable here: the owner's
			// subject is genuinely absent from AllowedSubs (which is keyed by
			// username instead), so the function still logs — it just must
			// never turn that into an error/abort, which the old
			// checkMachineRevocation did. The important behavioral change
			// this fix makes testable is "warns, does not fail"; the
			// warning's own wording carries the ambiguity ("revoked, or
			// keyed by username").
			name: "allowlist keyed by USERNAME for the owner — ambiguous, warns but never aborts startup",
			cfg: config.Config{
				AllowedSubs: map[string]bool{"msoent": true},
				MachineAccounts: map[string]config.MachineAccount{
					"svc-ci": {OwnerSub: "oidc|msoent-subject", Label: "ci-runner"},
				},
			},
			wantWarn:  true,
			wantLabel: "ci-runner",
			wantOwner: "oidc|msoent-subject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureWarnings(t, func() { warnOnUnverifiableMachineOwners(tt.cfg) })
			if !tt.wantWarn {
				if strings.Contains(out, "level=WARN") {
					t.Fatalf("warnOnUnverifiableMachineOwners() logged a warning, want none:\n%s", out)
				}
				return
			}
			if !strings.Contains(out, "level=WARN") {
				t.Fatalf("warnOnUnverifiableMachineOwners() logged nothing, want a warning")
			}
			if !strings.Contains(out, tt.wantLabel) {
				t.Fatalf("warning missing label %q:\n%s", tt.wantLabel, out)
			}
			if !strings.Contains(out, tt.wantOwner) {
				t.Fatalf("warning missing owner sub %q:\n%s", tt.wantOwner, out)
			}
		})
	}
}

// TestWarnOnUnverifiableMachineOwners_MessageStatesUncertainty pins the
// review's wording requirement: the log line must not claim the owner was
// definitely revoked (that would be false for a username-keyed allowlist),
// and it must spell out that fixing revocation also requires editing
// FLOW_MACHINE_ACCOUNTS, not just FLOW_ALLOWED_SUBS.
func TestWarnOnUnverifiableMachineOwners_MessageStatesUncertainty(t *testing.T) {
	cfg := config.Config{
		AllowedSubs: map[string]bool{"someone-else": true},
		MachineAccounts: map[string]config.MachineAccount{
			"machine-sub": {OwnerSub: "owner-sub", Label: "ci-runner"},
		},
	}
	out := captureWarnings(t, func() { warnOnUnverifiableMachineOwners(cfg) })
	for _, want := range []string{"username", "FLOW_MACHINE_ACCOUNTS", "FLOW_ALLOWED_SUBS"} {
		if !strings.Contains(out, want) {
			t.Fatalf("warning does not mention %q:\n%s", want, out)
		}
	}
}

// TestWarnOnUnverifiableMachineOwners_DeterministicOrder pins the fix for the
// "non-deterministic reporting" review finding: ranging a Go map directly
// would name a random offender on each run. warnOnUnverifiableMachineOwners
// sorts by machine subject (the map key) before logging, so repeated calls
// against identical config must log every offender in the same order.
func TestWarnOnUnverifiableMachineOwners_DeterministicOrder(t *testing.T) {
	cfg := config.Config{
		AllowedSubs: map[string]bool{"someone-else": true},
		MachineAccounts: map[string]config.MachineAccount{
			"zzz-machine": {OwnerSub: "owner-z", Label: "zzz-runner"},
			"aaa-machine": {OwnerSub: "owner-a", Label: "aaa-runner"},
			"mmm-machine": {OwnerSub: "owner-m", Label: "mmm-runner"},
		},
	}

	first := captureWarnings(t, func() { warnOnUnverifiableMachineOwners(cfg) })
	second := captureWarnings(t, func() { warnOnUnverifiableMachineOwners(cfg) })
	if first != second {
		t.Fatalf("warnOnUnverifiableMachineOwners() order not stable across runs:\n1: %s\n2: %s", first, second)
	}

	// Machine subjects sort as aaa- < mmm- < zzz-, so the three warnings must
	// appear in that order in the log output.
	ia := strings.Index(first, "aaa-runner")
	im := strings.Index(first, "mmm-runner")
	iz := strings.Index(first, "zzz-runner")
	if ia < 0 || im < 0 || iz < 0 {
		t.Fatalf("expected all three accounts to warn, got:\n%s", first)
	}
	if ia >= im || im >= iz {
		t.Fatalf("warnings not sorted by machine subject: aaa@%d mmm@%d zzz@%d\n%s", ia, im, iz, first)
	}
}
