// Package nvimeditor implements ports.Editor by spawning $VISUAL or
// $EDITOR (or nvim when both are unset) on the given file path with full
// stdio passthrough.
package nvimeditor

const defaultEditor = "nvim"
