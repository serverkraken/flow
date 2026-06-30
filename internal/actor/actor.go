// Package actor identifies who performed an action: a human or a named AI agent.
package actor

import "context"

type Kind string

const (
	Human Kind = "human"
	Agent Kind = "agent"
)

// Actor is the principal behind a mutation. Ref is the human's display name or
// the agent's MCP client name (e.g. "claude-code").
type Actor struct {
	Kind Kind
	Ref  string
}

type ctxKey int

const key ctxKey = 0

func WithContext(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, key, a)
}

// FromContext returns the actor stored in ctx, or a zero human if none.
func FromContext(ctx context.Context) Actor {
	if a, ok := ctx.Value(key).(Actor); ok {
		return a
	}
	return Actor{Kind: Human}
}

// FromHeader builds an Actor from the X-Flow-Actor header value. A non-empty
// value means an AI agent identified by that name; empty means the human user.
func FromHeader(headerVal, displayName string) Actor {
	if headerVal != "" {
		return Actor{Kind: Agent, Ref: headerVal}
	}
	return Actor{Kind: Human, Ref: displayName}
}
