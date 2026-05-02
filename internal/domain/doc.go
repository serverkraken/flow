// Package domain holds pure value types, parsers, and aggregations used by
// flow's worktime, palette, projects, and cheatsheet features.
//
// Dependency rule: stdlib only — no os, os/exec, net, database/*, no time.Now().
// Every dynamic value is passed in. Populated by phase F1 of the hexagonal
// refactor; see CLAUDE-hexagonal-plan.md for the migration map.
package domain
