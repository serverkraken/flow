package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
)

func TestWrapRootErr_Nil(t *testing.T) {
	t.Parallel()
	if got := wrapRootErr(nil); got != nil {
		t.Errorf("want nil, got %v", got)
	}
}

func TestWrapRootErr_ErrUnavailable(t *testing.T) {
	t.Parallel()
	// Simulate the real wrapping that httpapi does: fmt.Errorf("%w: dial tcp ...", ErrUnavailable)
	wrapped := fmt.Errorf("%w: dial tcp 127.0.0.1:8080: connect: connection refused", httpapi.ErrUnavailable)
	err := wrapRootErr(wrapped)
	if err == nil {
		t.Fatal("want non-nil error")
	}
	msg := err.Error()
	if strings.Contains(msg, "httpapi:") {
		t.Errorf("message must not contain 'httpapi:' prefix, got: %q", msg)
	}
	if strings.Contains(msg, "dial tcp") {
		t.Errorf("message must not contain raw 'dial tcp' chain, got: %q", msg)
	}
	if !strings.Contains(msg, "nicht erreichbar") {
		t.Errorf("message must contain 'nicht erreichbar', got: %q", msg)
	}
	if !strings.Contains(msg, "FLOW_SERVER_URL") {
		t.Errorf("message must mention 'FLOW_SERVER_URL', got: %q", msg)
	}
}

func TestWrapRootErr_ErrLoggedOut(t *testing.T) {
	t.Parallel()
	err := wrapRootErr(httpapi.ErrLoggedOut)
	if err == nil {
		t.Fatal("want non-nil error")
	}
	msg := err.Error()
	if strings.Contains(msg, "httpapi:") {
		t.Errorf("message must not contain 'httpapi:' prefix, got: %q", msg)
	}
	if !strings.Contains(msg, "flow login") {
		t.Errorf("message must mention 'flow login', got: %q", msg)
	}
}

func TestWrapRootErr_ErrNotConfigured(t *testing.T) {
	t.Parallel()
	err := wrapRootErr(httpapi.ErrNotConfigured)
	if err == nil {
		t.Fatal("want non-nil error")
	}
	msg := err.Error()
	if strings.Contains(msg, "httpapi:") {
		t.Errorf("message must not contain 'httpapi:' prefix, got: %q", msg)
	}
	if !strings.Contains(msg, "FLOW_SERVER_URL") {
		t.Errorf("message must mention 'FLOW_SERVER_URL', got: %q", msg)
	}
}

func TestWrapRootErr_OtherError_PassThrough(t *testing.T) {
	t.Parallel()
	orig := errors.New("some other error")
	got := wrapRootErr(orig)
	if got != orig {
		t.Errorf("want passthrough of original error, got: %v", got)
	}
}
