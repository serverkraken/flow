package webui

import "github.com/serverkraken/flow/internal/domain"

// TimerWidgetVM drives the global shell timer widget (desktop card + mobile
// chip). Exactly ONE session can run per owner; the widget is that session's
// single global home (IA rule: eine globale Aktion, ein globales Zuhause).
type TimerWidgetVM struct {
	Running     bool
	Unbound     bool // running without a node → stop requires choosing one
	SessionID   string
	NodeID      string
	NodeName    string
	NodeColor   string
	NodeKind    domain.NodeKind
	BaseSeconds int64
	Bookable    []domain.Node
	Err         string // i18n-resolved message rendered inline (never a popup)
}
