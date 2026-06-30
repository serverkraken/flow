package webui

import "github.com/serverkraken/flow/internal/domain"

// BuildHomeNewest returns up to n DocRows sorted newest-first by UpdatedAt.
// If n <= 0, returns all rows uncapped.
// It mirrors the defensive sort used by sortedDocuments (wissen_vm.go) and
// maps each document via docRowFromDocument so the project color swatch is
// populated from the colors map (nodeID → hex).
func BuildHomeNewest(docs []domain.Document, colors map[string]string, n int) []DocRow {
	sorted := sortedDocuments(docs)
	if n > 0 && len(sorted) > n {
		sorted = sorted[:n]
	}
	rows := make([]DocRow, 0, len(sorted))
	for _, d := range sorted {
		rows = append(rows, docRowFromDocument(d, colors))
	}
	return rows
}
