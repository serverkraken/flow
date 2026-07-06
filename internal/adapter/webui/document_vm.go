package webui

import (
	"html/template"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

// DocumentVM is the AppShell view model for one Wissen document.
type DocumentVM struct {
	User         string
	ID           string
	Type         string
	KindLabel    string
	KindGlyph    string
	KindTone     string
	Title        string
	NodeID    string
	NodeName  string
	ProjectColor string
	DateStr      string
	Tags         []TagLink
	HTML         template.HTML
	// HasMermaid mirrors DocMeta.HasMermaid from RenderDocument — the single
	// source of truth for whether this document needs mermaid-init.js
	// (Task 4/5 wire the conditional <script> load off this field).
	HasMermaid   bool
	Backlinks    []components.Backlink
	Embed        *EmbedView
	CategoryHref     string
	CategoryLabelKey string
}

func embedLabelKey(state string) string {
	switch state {
	case "ok":
		return "embed.ok"
	case "pending":
		return "embed.pending"
	case "retrying":
		return "embed.retrying"
	case "failed":
		return "embed.failed"
	default:
		return "embed.unknown"
	}
}

func embedToneClass(state string) string {
	switch state {
	case "ok":
		return "bg-success/10 text-success border-success/30"
	case "pending":
		return "bg-sunken text-muted border-line"
	case "retrying":
		return "bg-warning/10 text-warning border-warning/35"
	case "failed":
		return "bg-danger/10 text-danger border-danger/35"
	default:
		return "bg-sunken text-muted border-line"
	}
}

func projectSwatchStyle(color string) string {
	if color == "" {
		return ""
	}
	return "background-color: " + color
}
