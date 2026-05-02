// Package fspaletteentries implements ports.PaletteEntryReader by
// aggregating menu.entries files from the user's enabled tmux plugins.
//
// Plugins to scan come from the enabled-plugins file (typically
// ~/.tmux/enabled-plugins, one plugin name per line, '#' comments
// allowed). When that file is absent, the reader falls back to listing
// every subdirectory of the plugins directory — useful in
// developer/test contexts where the enabled-plugins file is not yet
// in place.
//
// menu.entries schema: `icon<TAB>label<TAB>action[<TAB>section[<TAB>keybind]]`.
// Empty section defaults to "Misc"; rows missing the action column are
// dropped silently.
package fspaletteentries
