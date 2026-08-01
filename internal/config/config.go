// Package config loads flow-server configuration from the environment.
package config

import (
	"fmt"
	"strings"
)

// MachineAccount delegates a machine credential to a human owner's tenant.
//
// The owner is addressed by OIDC subject, not by username: only users.oidc_sub
// carries a UNIQUE constraint (migration 0001), while username is a plain
// TEXT NOT NULL DEFAULT ''. Resolving an owner by a non-unique column would be
// ambiguous, and an ambiguous owner in a multi-tenant app means a machine token
// can end up writing into the wrong tenant.
type MachineAccount struct {
	Sub      string // the service account's OIDC subject
	OwnerSub string // the OIDC subject of the flow user whose tenant it writes into
	Label    string // audit name, surfaced as the actor Ref
}

type Config struct {
	DatabaseURL      string
	OIDCIssuer       string
	OIDCCliIssuer    string
	OIDCClientID     string
	OIDCCliClientID  string
	OIDCClientSecret string
	PublicBaseURL    string
	SessionSecret    string
	AllowedSubs      map[string]bool
	AllowedGroups    map[string]bool
	ListenAddr       string
	Dev              bool

	OIDCMachineIssuer   string
	OIDCMachineClientID string
	// MachineAccounts is keyed by the MACHINE subject.
	MachineAccounts map[string]MachineAccount
}

// Load reads config via getenv (injected for testability).
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		DatabaseURL:      getenv("DATABASE_URL"),
		OIDCIssuer:       getenv("FLOW_OIDC_ISSUER"),
		OIDCCliIssuer:    getenv("FLOW_OIDC_CLI_ISSUER"),
		OIDCClientID:     getenv("FLOW_OIDC_CLIENT_ID"),
		OIDCCliClientID:  getenv("FLOW_OIDC_CLI_CLIENT_ID"),
		OIDCClientSecret: getenv("FLOW_OIDC_CLIENT_SECRET"),
		PublicBaseURL:    getenv("FLOW_PUBLIC_BASE_URL"),
		SessionSecret:    getenv("FLOW_SESSION_SECRET"),
		ListenAddr:       getenv("FLOW_LISTEN_ADDR"),
		Dev:              getenv("FLOW_DEV") == "1",
		AllowedSubs:      map[string]bool{},
		AllowedGroups:    map[string]bool{},

		OIDCMachineIssuer:   getenv("FLOW_OIDC_MACHINE_ISSUER"),
		OIDCMachineClientID: getenv("FLOW_OIDC_MACHINE_CLIENT_ID"),
		MachineAccounts:     map[string]MachineAccount{},
	}
	for _, s := range strings.Split(getenv("FLOW_ALLOWED_SUBS"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			c.AllowedSubs[s] = true
		}
	}
	for _, g := range strings.Split(getenv("FLOW_ALLOWED_GROUPS"), ",") {
		if g = strings.TrimSpace(g); g != "" {
			c.AllowedGroups[g] = true
		}
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	for _, f := range []struct{ name, val string }{
		{"DATABASE_URL", c.DatabaseURL},
		{"FLOW_OIDC_ISSUER", c.OIDCIssuer},
		{"FLOW_OIDC_CLIENT_ID", c.OIDCClientID},
		{"FLOW_OIDC_CLI_CLIENT_ID", c.OIDCCliClientID},
		{"FLOW_OIDC_CLIENT_SECRET", c.OIDCClientSecret},
		{"FLOW_PUBLIC_BASE_URL", c.PublicBaseURL},
		{"FLOW_SESSION_SECRET", c.SessionSecret},
	} {
		if f.val == "" {
			return Config{}, fmt.Errorf("config: %s is required", f.name)
		}
	}

	// Machine auth is all-or-nothing. A half-configured server would verify
	// machine tokens and then reject them with no hint that a variable is
	// missing — a loud start-up failure costs less diagnosis time.
	rawMachineAccounts := getenv("FLOW_MACHINE_ACCOUNTS")
	var machineSet, machineMissing []string
	for _, f := range []struct{ name, val string }{
		{"FLOW_OIDC_MACHINE_ISSUER", c.OIDCMachineIssuer},
		{"FLOW_OIDC_MACHINE_CLIENT_ID", c.OIDCMachineClientID},
		{"FLOW_MACHINE_ACCOUNTS", rawMachineAccounts},
	} {
		if strings.TrimSpace(f.val) == "" {
			machineMissing = append(machineMissing, f.name)
		} else {
			machineSet = append(machineSet, f.name)
		}
	}
	if len(machineSet) > 0 && len(machineMissing) > 0 {
		return Config{}, fmt.Errorf(
			"config: machine auth is partially configured (%s set); also required: %s",
			strings.Join(machineSet, ", "), strings.Join(machineMissing, ", "))
	}
	if len(machineSet) == 3 {
		accounts, err := parseMachineAccounts(rawMachineAccounts)
		if err != nil {
			return Config{}, err
		}
		if len(accounts) == 0 {
			return Config{}, fmt.Errorf("config: FLOW_MACHINE_ACCOUNTS is set but contains no entries")
		}
		c.MachineAccounts = accounts
	}

	return c, nil
}

// parseMachineAccounts reads a comma-separated list of
// <machine-sub>=<owner-sub>:<label>. Every malformed entry is an error rather
// than a skip: a typo'd mapping that silently becomes "not mapped" would show
// up as a 403 at 04:30 with nothing pointing at the real cause.
func parseMachineAccounts(raw string) (map[string]MachineAccount, error) {
	out := map[string]MachineAccount{}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		sub, rest, hasEq := strings.Cut(entry, "=")
		ownerSub, label, hasColon := strings.Cut(rest, ":")
		if !hasEq || !hasColon {
			return nil, fmt.Errorf(
				"config: FLOW_MACHINE_ACCOUNTS entry %q: want <machine-sub>=<owner-sub>:<label>", entry)
		}
		sub = strings.TrimSpace(sub)
		ownerSub = strings.TrimSpace(ownerSub)
		label = strings.TrimSpace(label)
		if sub == "" || ownerSub == "" || label == "" {
			return nil, fmt.Errorf(
				"config: FLOW_MACHINE_ACCOUNTS entry %q: machine sub, owner sub and label must all be non-empty", entry)
		}
		if sub == ownerSub {
			return nil, fmt.Errorf(
				"config: FLOW_MACHINE_ACCOUNTS entry %q: machine sub and owner sub are identical — "+
					"this would demote a human token to machine rights", entry)
		}
		if _, dup := out[sub]; dup {
			return nil, fmt.Errorf("config: FLOW_MACHINE_ACCOUNTS: duplicate machine sub %q", sub)
		}
		out[sub] = MachineAccount{Sub: sub, OwnerSub: ownerSub, Label: label}
	}
	return out, nil
}

// RedirectURL is the OIDC auth-code callback, derived from the public base URL.
func (c Config) RedirectURL() string {
	return strings.TrimRight(c.PublicBaseURL, "/") + "/auth/callback"
}
