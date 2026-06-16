package domain

import (
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// TagCount is a tag and how many documents carry it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// ParseFrontmatter extracts tags from a leading YAML frontmatter block and
// reports the byte offset where the real body begins (for renderers to skip the
// block). A body that does not start with a "---\n" fence, has no closing
// "---"/"..." fence, or whose block is unparseable YAML yields (nil, 0) — the
// whole body is then treated as content. Tags are normalized: trimmed,
// lowercased, empties dropped, de-duplicated, first-seen order preserved.
func ParseFrontmatter(body string) (tags []string, bodyStart int) {
	const open = "---\n"
	if !strings.HasPrefix(body, open) {
		return nil, 0
	}
	rest := body[len(open):]

	end, after := -1, -1
	for off := 0; off <= len(rest); {
		nl := strings.IndexByte(rest[off:], '\n')
		var line string
		if nl < 0 {
			line = rest[off:]
		} else {
			line = rest[off : off+nl]
		}
		if line == "---" || line == "..." {
			end = off
			if nl < 0 {
				after = len(rest)
			} else {
				after = off + nl + 1
			}
			break
		}
		if nl < 0 {
			break
		}
		off += nl + 1
	}
	if end < 0 {
		return nil, 0
	}

	var fm struct {
		Tags []string `yaml:"tags"`
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return nil, 0
	}
	return normalizeTags(fm.Tags), len(open) + after
}

// normalizeTags trims, lowercases, drops empties, and de-duplicates while
// preserving first-seen order. Returns nil for an empty result.
func normalizeTags(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// CollectTags aggregates tag counts across a document set, sorted by count
// descending then tag ascending. Reads Document.Tags.
func CollectTags(docs []Document) []TagCount {
	counts := map[string]int{}
	for _, d := range docs {
		for _, t := range d.Tags {
			counts[t]++
		}
	}
	out := make([]TagCount, 0, len(counts))
	for t, c := range counts {
		out = append(out, TagCount{Tag: t, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}
