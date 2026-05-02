package domain

// KompendiumNote is one entry in the Kompendium notebook, returned by
// `kompendium ls --json` (or any equivalent gateway).
type KompendiumNote struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Date    string `json:"date,omitempty"`
	Project string `json:"project,omitempty"`
	MTime   string `json:"mtime,omitempty"`
}
