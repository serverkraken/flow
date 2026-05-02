// Package gitsnapshot manages the notebook's git lifecycle by shelling out
// to the system git binary. Init/Snapshot inject a kompendium identity via
// `git -c user.name=... -c user.email=...` only when the host has no
// identity configured; if user.name + user.email are set globally or
// locally, real authorship is preserved across machines.
package gitsnapshot

const (
	fallbackIdentityName  = "kompendium"
	fallbackIdentityEmail = "kompendium@local"
)
