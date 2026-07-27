package httpserver

import "github.com/serverkraken/flow/internal/domain"

// sp / nsp build pointers for full-replace UpdateNode callers (WebUI forms,
// create-followup) that intend to set every field. The partial PATCH handler
// passes JSON pointers straight through and does not need these.
func sp(s string) *string { return &s }

func nsp(s domain.NodeStatus) *domain.NodeStatus { return &s }
