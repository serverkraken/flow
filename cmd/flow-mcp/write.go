package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/domain"
)

// requireType validates a required `type` argument for create against the
// canonical document-type set. Unlike the read tools' optional checkType, an
// empty value is an error.
func requireType(typ string) (domain.DocumentType, error) {
	t := strings.TrimSpace(typ)
	if t == "" {
		return "", fmt.Errorf("type is required. Valid types: %s", typeList())
	}
	for _, v := range domain.DocumentTypes() {
		if domain.DocumentType(t) == v {
			return v, nil
		}
	}
	return "", fmt.Errorf("invalid type %q. Valid types: %s", t, typeList())
}

type documentWriteResult struct {
	Action    string `json:"action"`
	ID        string `json:"id"`
	Project   string `json:"project"`
	Type      string `json:"type,omitempty"`
	Path      string `json:"path,omitempty"`
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updatedAt"`
	Version   string `json:"version"`
	Hash      string `json:"hash"`
}

func (h *handlers) documentResult(ctx context.Context, action string, d domain.Document) *mcp.CallToolResult {
	project := "none"
	if d.NodeID != nil {
		project = *d.NodeID
		if nodes, err := h.nodeList(ctx, false); err == nil {
			for _, node := range nodes {
				if node.ID == *d.NodeID {
					project = node.Slug
					break
				}
			}
		}
	}
	version := d.UpdatedAt.UTC().Format(time.RFC3339Nano)
	out := makeWriteResult(action, d.ID, project, version, d.Body)
	out.Type, out.Path, out.Title = string(d.Type), d.Path, d.Title
	return writeResult(out)
}

func structuredWriteResult(action, id, project, updatedAt, body string) *mcp.CallToolResult {
	return writeResult(makeWriteResult(action, id, project, updatedAt, body))
}

func makeWriteResult(action, id, project, updatedAt, body string) documentWriteResult {
	sum := sha256.Sum256([]byte(body))
	return documentWriteResult{
		Action: action, ID: id, Project: project, UpdatedAt: updatedAt, Version: updatedAt,
		Hash: "sha256:" + hex.EncodeToString(sum[:]),
	}
}

func writeResult(out documentWriteResult) *mcp.CallToolResult {
	b, _ := json.Marshal(out)
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(b)}},
		StructuredContent: out,
	}
}

func expectedUpdatedAt(raw string, fallback time.Time) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if fallback.IsZero() {
			return nil, nil
		}
		t := fallback
		return &t, nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, fmt.Errorf("expectedUpdatedAt must be RFC3339 with optional fractional seconds")
	}
	return &t, nil
}

func patchMarkdown(body string, in patchDocIn) (string, error) {
	switch strings.TrimSpace(in.Operation) {
	case "replace_section":
		start, end, err := markdownSection(body, in.Section)
		if err != nil {
			return "", err
		}
		return replaceMarkdownSection(body, start, end, in.Body), nil
	case "append_section":
		section := normalizeSection(in.Section)
		if section == "" || strings.TrimSpace(in.Body) == "" {
			return "", fmt.Errorf("append_section requires section and body")
		}
		start, end, err := markdownSection(body, section)
		if err != nil {
			if !strings.Contains(err.Error(), "not found") {
				return "", err
			}
			base := strings.TrimRight(body, "\n")
			if base != "" {
				base += "\n\n"
			}
			return base + "## " + section + "\n\n" + strings.Trim(in.Body, "\n") + "\n", nil
		}
		lines := strings.Split(body, "\n")
		current := strings.Trim(strings.Join(lines[start+1:end], "\n"), "\n")
		if current != "" {
			current += "\n\n"
		}
		return replaceMarkdownSection(body, start, end, current+strings.Trim(in.Body, "\n")), nil
	case "set_checkbox":
		if strings.TrimSpace(in.Checkbox) == "" || in.Checked == nil {
			return "", fmt.Errorf("set_checkbox requires checkbox and checked")
		}
		if in.Label != nil && strings.TrimSpace(*in.Label) == "" {
			return "", fmt.Errorf("set_checkbox label must not be empty")
		}
		lines := strings.Split(body, "\n")
		match := -1
		for i, line := range lines {
			label, markAt, ok := markdownCheckbox(line)
			if !ok || strings.TrimSpace(label) != strings.TrimSpace(in.Checkbox) {
				continue
			}
			if match >= 0 {
				return "", fmt.Errorf("checkbox %q is ambiguous", in.Checkbox)
			}
			match = i
			if *in.Checked {
				lines[i] = line[:markAt] + "x" + line[markAt+1:]
			} else {
				lines[i] = line[:markAt] + " " + line[markAt+1:]
			}
			if in.Label != nil {
				labelAt := len(lines[i]) - len(label)
				lines[i] = lines[i][:labelAt] + " " + strings.TrimSpace(*in.Label)
			}
		}
		if match < 0 {
			return "", fmt.Errorf("checkbox %q not found", in.Checkbox)
		}
		return strings.Join(lines, "\n"), nil
	default:
		return "", fmt.Errorf("operation must be replace_section, append_section, or set_checkbox")
	}
}

func markdownSection(body, section string) (int, int, error) {
	want := normalizeSection(section)
	if want == "" {
		return 0, 0, fmt.Errorf("section is required")
	}
	lines := strings.Split(body, "\n")
	start, level := -1, 0
	for i, line := range lines {
		gotLevel, title, ok := markdownHeading(line)
		if !ok || title != want {
			continue
		}
		if start >= 0 {
			return 0, 0, fmt.Errorf("section %q is ambiguous", want)
		}
		start, level = i, gotLevel
	}
	if start < 0 {
		return 0, 0, fmt.Errorf("section %q not found", want)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		gotLevel, _, ok := markdownHeading(lines[i])
		if ok && gotLevel <= level {
			end = i
			break
		}
	}
	return start, end, nil
}

func replaceMarkdownSection(body string, start, end int, replacement string) string {
	lines := strings.Split(body, "\n")
	before := strings.TrimRight(strings.Join(lines[:start+1], "\n"), "\n")
	after := strings.Trim(strings.Join(lines[end:], "\n"), "\n")
	out := before + "\n\n" + strings.Trim(replacement, "\n")
	if after != "" {
		out += "\n\n" + after
	}
	return strings.TrimRight(out, "\n") + "\n"
}

func normalizeSection(section string) string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(section), "#"))
}

func markdownHeading(line string) (int, string, bool) {
	trimmed := strings.TrimSpace(line)
	level := 0
	for level < len(trimmed) && level < 6 && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level >= len(trimmed) || trimmed[level] != ' ' {
		return 0, "", false
	}
	return level, strings.TrimSpace(trimmed[level+1:]), true
}

func markdownCheckbox(line string) (label string, markAt int, ok bool) {
	trimmedAt := len(line) - len(strings.TrimLeft(line, " \t"))
	if trimmedAt >= len(line) || (line[trimmedAt] != '-' && line[trimmedAt] != '*') {
		return "", 0, false
	}
	rest := line[trimmedAt+1:]
	spaces := len(rest) - len(strings.TrimLeft(rest, " \t"))
	open := trimmedAt + 1 + spaces
	if spaces == 0 || open+3 >= len(line) || line[open] != '[' || line[open+2] != ']' || (line[open+1] != ' ' && line[open+1] != 'x' && line[open+1] != 'X') {
		return "", 0, false
	}
	return line[open+3:], open + 1, true
}

// guardMutation enforces the anti-clobber write guard: a human-owned document
// (daily / project / free) may only be modified or deleted with confirm=true.
func guardMutation(d domain.Document, confirm bool) error {
	if d.Type.HumanOwned() && !confirm {
		return fmt.Errorf("%s is a human-owned note (type=%s). Pass confirm=true to modify it", d.ID, d.Type)
	}
	return nil
}
