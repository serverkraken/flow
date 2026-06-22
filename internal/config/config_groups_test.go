package config

import "testing"

func TestLoadAllowedGroups(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":            "postgres://x",
		"FLOW_OIDC_ISSUER":        "https://issuer",
		"FLOW_OIDC_CLIENT_ID":     "flow",
		"FLOW_OIDC_CLI_CLIENT_ID": "flow-cli",
		"FLOW_OIDC_CLIENT_SECRET": "s",
		"FLOW_PUBLIC_BASE_URL":    "https://flow.example.com",
		"FLOW_SESSION_SECRET":     "secret",
		"FLOW_ALLOWED_GROUPS":     "a, b",
	}
	c, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.AllowedGroups) != 2 || !c.AllowedGroups["a"] || !c.AllowedGroups["b"] {
		t.Fatalf("AllowedGroups parse: got %v, want {a:true, b:true}", c.AllowedGroups)
	}
}

func TestLoadAllowedGroupsUnset(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":            "postgres://x",
		"FLOW_OIDC_ISSUER":        "https://issuer",
		"FLOW_OIDC_CLIENT_ID":     "flow",
		"FLOW_OIDC_CLI_CLIENT_ID": "flow-cli",
		"FLOW_OIDC_CLIENT_SECRET": "s",
		"FLOW_PUBLIC_BASE_URL":    "https://flow.example.com",
		"FLOW_SESSION_SECRET":     "secret",
	}
	c, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("Load with no FLOW_ALLOWED_GROUPS must succeed: %v", err)
	}
	if len(c.AllowedGroups) != 0 {
		t.Fatalf("AllowedGroups should be empty map, got %v", c.AllowedGroups)
	}
}
