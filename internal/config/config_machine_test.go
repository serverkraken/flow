package config

import "testing"

// baseEnv is the minimal valid environment; machine-auth cases layer on top.
func baseEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL":            "postgres://x",
		"FLOW_OIDC_ISSUER":        "https://issuer",
		"FLOW_OIDC_CLIENT_ID":     "flow",
		"FLOW_OIDC_CLI_CLIENT_ID": "flow-cli",
		"FLOW_OIDC_CLIENT_SECRET": "s",
		"FLOW_PUBLIC_BASE_URL":    "https://flow.example.com",
		"FLOW_SESSION_SECRET":     "secret",
	}
}

func withMachine(accounts string) map[string]string {
	env := baseEnv()
	env["FLOW_OIDC_MACHINE_ISSUER"] = "https://issuer/o/flow-machine/"
	env["FLOW_OIDC_MACHINE_CLIENT_ID"] = "flow-machine"
	env["FLOW_MACHINE_ACCOUNTS"] = accounts
	return env
}

func loadEnv(env map[string]string) (Config, error) {
	return Load(func(k string) string { return env[k] })
}

func TestLoadMachineAccounts(t *testing.T) {
	c, err := loadEnv(withMachine(" sa-1 = owner-1 : wartung-agent , sa-2=owner-1:backup-agent "))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.MachineAccounts) != 2 {
		t.Fatalf("want 2 accounts, got %v", c.MachineAccounts)
	}
	got, ok := c.MachineAccounts["sa-1"]
	if !ok {
		t.Fatalf("sa-1 missing from %v", c.MachineAccounts)
	}
	want := MachineAccount{Sub: "sa-1", OwnerSub: "owner-1", Label: "wartung-agent"}
	if got != want {
		t.Fatalf("sa-1 = %+v, want %+v", got, want)
	}
	if c.OIDCMachineClientID != "flow-machine" {
		t.Fatalf("OIDCMachineClientID = %q", c.OIDCMachineClientID)
	}
}

func TestLoadMachineAuthUnsetIsFine(t *testing.T) {
	c, err := loadEnv(baseEnv())
	if err != nil {
		t.Fatalf("Load without machine auth must succeed: %v", err)
	}
	if len(c.MachineAccounts) != 0 {
		t.Fatalf("MachineAccounts should be empty, got %v", c.MachineAccounts)
	}
	if c.OIDCMachineIssuer != "" || c.OIDCMachineClientID != "" {
		t.Fatal("machine issuer/client must stay empty when unset")
	}
}

func TestLoadMachineAuthRejectsPartialConfig(t *testing.T) {
	env := baseEnv()
	env["FLOW_OIDC_MACHINE_ISSUER"] = "https://issuer/o/flow-machine/"
	if _, err := loadEnv(env); err == nil {
		t.Fatal("partial machine config must fail loudly, not silently reject tokens later")
	}
}

func TestLoadMachineAccountsRejectsMalformed(t *testing.T) {
	cases := []struct {
		name, accounts string
	}{
		{"no equals", "sa-1:wartung-agent"},
		{"no colon", "sa-1=owner-1"},
		{"empty machine sub", "=owner-1:wartung-agent"},
		{"empty owner sub", "sa-1=:wartung-agent"},
		{"empty label", "sa-1=owner-1:"},
		{"duplicate machine sub", "sa-1=owner-1:a,sa-1=owner-2:b"},
		{"machine sub equals owner sub", "sa-1=sa-1:wartung-agent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := loadEnv(withMachine(tc.accounts)); err == nil {
				t.Fatalf("accounts %q must be rejected", tc.accounts)
			}
		})
	}
}
