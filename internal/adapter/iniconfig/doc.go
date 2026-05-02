// Package iniconfig implements ports.ConfigReader by parsing a simple
// key=value file (with `#` comments and blank lines allowed) — typically
// ~/.tmux/worktime.conf.
//
// Recognised keys:
//
//	target_hours      = 8       # default daily target
//	target_mon..sun   = 8       # per-weekday override
//	tag_target_NAME   = 4       # per-tag daily target
//
// The default-target value comes from (in order): the file's target_hours
// key, the WORKTIME_TARGET_HOURS env var, then 8h. A missing file is not
// an error — callers fall back to the env/default chain.
package iniconfig
