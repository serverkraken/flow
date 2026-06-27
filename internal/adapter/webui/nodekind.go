package webui

import "github.com/serverkraken/flow/internal/domain"

// NodeKindBadge is the WebUI presentation of a domain.NodeKind: an i18n label
// key, a whitelisted monospace glyph and a tone name that maps to the shared
// kindToneClass utility (accent|highlight|success|muted). This is the single
// source of truth for node-kind coloring in the WebUI (the TUI kindcolor pkg
// only maps DocumentType).
type NodeKindBadge struct {
	LabelKey string
	Glyph    string
	Tone     string
}

// NodeKindStyle maps a node kind to its badge treatment.
func NodeKindStyle(k domain.NodeKind) NodeKindBadge {
	switch k {
	case domain.KindEngagement:
		return NodeKindBadge{LabelKey: "node.kind.engagement", Glyph: "◆", Tone: "accent"}
	case domain.KindVorhaben:
		return NodeKindBadge{LabelKey: "node.kind.vorhaben", Glyph: "▲", Tone: "highlight"}
	case domain.KindRepo:
		return NodeKindBadge{LabelKey: "node.kind.repo", Glyph: "●", Tone: "success"}
	case domain.KindBranch:
		return NodeKindBadge{LabelKey: "node.kind.branch", Glyph: "·", Tone: "muted"}
	default:
		return NodeKindBadge{LabelKey: "node.kind.repo", Glyph: "·", Tone: "muted"}
	}
}
