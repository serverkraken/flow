package worktime

import "strings"

// normalizeDurationArg lowercases the H/M suffixes of a "+1h30m" style
// stop argument so domain.parseHumanDuration (strict-lower) accepts
// macro-keymap'd or shifted-typing variants like "+8H02M". The leading
// "+" and the numeric digits pass through unchanged. Arguments that do
// not start with "+" (HH:MM clock times, empty) are returned as-is so
// the caller's downstream parser sees the literal user input.
//
// Both today_dialog.submitEdit and history_edit.parseDrillStop call
// this — the lowercase step was originally inlined in parseDrillStop
// only; the inline duplication is what caused the heute-Edit form to
// reject "+8H02M" while the history-drill form accepted it.
func normalizeDurationArg(arg string) string {
	if len(arg) > 1 && arg[0] == '+' {
		return strings.ToLower(arg)
	}
	return arg
}
