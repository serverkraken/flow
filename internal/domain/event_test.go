package domain_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

func TestEventNodeMovedValue(t *testing.T) {
	t.Parallel()
	if got := domain.EventNodeMoved; got != "node.moved" {
		t.Fatalf("EventNodeMoved = %q, want %q", got, "node.moved")
	}
}
