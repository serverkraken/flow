package main

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/config"
)

func TestCheckMachineRevocation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		wantErr string // substring expected in the error, "" means no error
	}{
		{
			name: "no allowlist at all — cannot prove revocation, must not block",
			cfg: config.Config{
				MachineAccounts: map[string]config.MachineAccount{
					"machine-sub": {OwnerSub: "owner-sub", Label: "ci-runner"},
				},
			},
		},
		{
			name: "owner allowed — fine",
			cfg: config.Config{
				AllowedSubs: map[string]bool{"owner-sub": true},
				MachineAccounts: map[string]config.MachineAccount{
					"machine-sub": {OwnerSub: "owner-sub", Label: "ci-runner"},
				},
			},
		},
		{
			name: "owner missing from sub allowlist, no groups — provably revoked",
			cfg: config.Config{
				AllowedSubs: map[string]bool{"someone-else": true},
				MachineAccounts: map[string]config.MachineAccount{
					"machine-sub": {OwnerSub: "owner-sub", Label: "ci-runner"},
				},
			},
			wantErr: `machine account "ci-runner" delegates to owner "owner-sub"`,
		},
		{
			name: "owner missing from sub allowlist but groups configured — cannot prove, must not block",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkMachineRevocation(tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("checkMachineRevocation() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("checkMachineRevocation() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("checkMachineRevocation() = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
