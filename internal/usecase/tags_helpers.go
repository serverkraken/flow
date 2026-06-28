package usecase

import "github.com/serverkraken/flow/internal/domain"

// slugsOf converts a slice of domain.Tag to a slice of slug strings.
func slugsOf(tags []domain.Tag) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.Slug
	}
	return out
}
