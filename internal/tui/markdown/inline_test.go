package markdown

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/yuin/goldmark/ast"
)

// TestRenderString_DirectCall covers renderString (0% coverage).
// renderString is registered to handle *ast.String nodes produced by
// linkify-style extensions. We call it directly to exercise both the
// entering=false early-return and the entering=true write path.
func TestRenderString_DirectCall(t *testing.T) {
	t.Parallel()

	r := &nodeRenderer{}

	// entering=false: should return WalkContinue with no write.
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	status, err := r.renderString(bw, nil, nil, false)
	if err != nil {
		t.Fatalf("renderString(entering=false) error: %v", err)
	}
	if status != ast.WalkContinue {
		t.Errorf("status = %v, want WalkContinue", status)
	}

	// entering=true with a non-String node (type guard → no write).
	status2, err2 := r.renderString(bw, nil, ast.NewDocument(), true)
	if err2 != nil {
		t.Fatalf("renderString(non-String node) error: %v", err2)
	}
	if status2 != ast.WalkContinue {
		t.Errorf("status = %v, want WalkContinue", status2)
	}

	// entering=true with an *ast.String node: should write the Value.
	s := ast.NewString([]byte("hello"))
	status3, err3 := r.renderString(bw, nil, s, true)
	if err3 != nil {
		t.Fatalf("renderString(*ast.String) error: %v", err3)
	}
	if status3 != ast.WalkContinue {
		t.Errorf("status = %v, want WalkContinue", status3)
	}
	_ = bw.Flush()
	if buf.String() != "hello" {
		t.Errorf("buffer = %q, want %q", buf.String(), "hello")
	}
}
