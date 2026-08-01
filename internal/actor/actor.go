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

// AuthenticatedHuman builds audit provenance from the server-verified user.
// Agent is retained for historical entries and future trusted agent credentials;
// caller-controlled labels must never be promoted to it.
func AuthenticatedHuman(displayName string) Actor {
	return Actor{Kind: Human, Ref: displayName}
}

// TrustedMachine builds audit provenance for a verified machine credential: a
// token minted by the machine issuer/audience pair whose subject the server's
// own configuration maps to an owner. The label comes from that configuration,
// never from the request — which is the condition AuthenticatedHuman's comment
// sets for anything being promoted to Agent.
func TrustedMachine(label string) Actor {
	return Actor{Kind: Agent, Ref: label}
}
