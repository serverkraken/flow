package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// entryDTO is the JSON shape of a NoteEntry. Domain types stay free of JSON
// concerns; the DTO lives in the frontend.
type entryDTO struct {
	ID      domain.ID       `json:"id"`
	Type    domain.NoteType `json:"type"`
	Title   string          `json:"title,omitempty"`
	Project string          `json:"project,omitempty"`
	Date    string          `json:"date,omitempty"`
	Mtime   time.Time       `json:"mtime"`
}

func entryToDTO(e ports.NoteEntry) entryDTO {
	return entryDTO{
		ID:      e.ID,
		Type:    e.Meta.Type,
		Title:   e.Meta.Title,
		Project: e.Meta.Project,
		Date:    e.Meta.Date,
		Mtime:   e.Mtime,
	}
}

// searchResultDTO is the JSON shape of a SearchResult.
type searchResultDTO struct {
	ID      domain.ID `json:"id"`
	Title   string    `json:"title,omitempty"`
	Snippet string    `json:"snippet,omitempty"`
	Score   float64   `json:"score"`
}

func resultToDTO(r domain.SearchResult) searchResultDTO {
	return searchResultDTO{ID: r.ID, Title: r.Title, Snippet: r.Snippet, Score: r.Score}
}

func printEntries(w io.Writer, entries []ports.NoteEntry, asJSON bool) error {
	if asJSON {
		out := make([]entryDTO, len(entries))
		for i, e := range entries {
			out[i] = entryToDTO(e)
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	for _, e := range entries {
		title := e.Meta.Title
		if title == "" {
			title = e.ID.String()
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", e.ID, e.Meta.Type, title); err != nil {
			return err
		}
	}
	return nil
}

func printSearchResults(w io.Writer, results []domain.SearchResult, asJSON bool) error {
	if asJSON {
		out := make([]searchResultDTO, len(results))
		for i, r := range results {
			out[i] = resultToDTO(r)
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	for _, r := range results {
		title := r.Title
		if title == "" {
			title = r.ID.String()
		}
		// FTS5 snippets carry the surrounding body verbatim, including
		// newlines. Collapse them so each result stays on one TSV row —
		// otherwise downstream consumers (column, awk, fzf) split a single
		// match into many records.
		snippet := flattenSnippet(r.Snippet)
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", r.ID, title, snippet); err != nil {
			return err
		}
	}
	return nil
}

func flattenSnippet(s string) string {
	if s == "" {
		return ""
	}
	r := strings.NewReplacer("\r\n", " ⏎ ", "\n", " ⏎ ", "\t", " ", "\r", " ⏎ ")
	return r.Replace(s)
}

func printCreateOutput(w io.Writer, id domain.ID, created bool, path string) error {
	verb := "Reused"
	if created {
		verb = "Created"
	}
	_, err := fmt.Fprintf(w, "%s %s\n%s\n", verb, id, path)
	return err
}
