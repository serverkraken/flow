package apiclient_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

// TestNewInsecure exercises NewInsecure and InsecureBase — both at 0% coverage.
// NewInsecure is a dev-only constructor that skips TLS verification.
// We just verify it returns a non-nil client (the transport is internal detail).
func TestNewInsecure(t *testing.T) {
	c := apiclient.NewInsecure("https://localhost:8443", "test-token")
	if c == nil {
		t.Fatal("NewInsecure should return a non-nil client")
	}
}

// TestInsecureBase exercises InsecureBase which returns an http.RoundTripper.
func TestInsecureBase(t *testing.T) {
	rt := apiclient.InsecureBase()
	if rt == nil {
		t.Fatal("InsecureBase should return a non-nil RoundTripper")
	}
	// Verify it implements http.RoundTripper (inferred, no explicit annotation needed).
	_ = rt
}
