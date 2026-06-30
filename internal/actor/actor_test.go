package actor_test

import (
	"context"
	"testing"

	"github.com/serverkraken/flow/internal/actor"
)

func TestFromHeader(t *testing.T) {
	got := actor.FromHeader("claude-code", "Soenne")
	want := actor.Actor{Kind: actor.Agent, Ref: "claude-code"}
	if got != want {
		t.Errorf("FromHeader(\"claude-code\", \"Soenne\") = %+v, want %+v", got, want)
	}

	got = actor.FromHeader("", "Soenne")
	want = actor.Actor{Kind: actor.Human, Ref: "Soenne"}
	if got != want {
		t.Errorf("FromHeader(\"\", \"Soenne\") = %+v, want %+v", got, want)
	}
}

func TestWithContextAndFromContext(t *testing.T) {
	a := actor.Actor{Kind: actor.Agent, Ref: "claude-code"}
	ctx := actor.WithContext(context.Background(), a)
	got := actor.FromContext(ctx)
	if got != a {
		t.Errorf("FromContext(WithContext(ctx, a)) = %+v, want %+v", got, a)
	}
}

func TestFromContextDefault(t *testing.T) {
	got := actor.FromContext(context.Background())
	want := actor.Actor{Kind: actor.Human}
	if got != want {
		t.Errorf("FromContext(Background()) = %+v, want %+v", got, want)
	}
}
