//go:build integration

package oidctest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/serverkraken/flow/internal/testutil/oidctest"
)

// TestIntegration_StartDex_DiscoveryReachable boots dex and verifies the
// OIDC discovery doc is reachable + has the expected issuer. Smoke test
// for the helper itself.
func TestIntegration_StartDex_DiscoveryReachable(t *testing.T) {
	d := oidctest.StartDex(t)

	url := d.Issuer + "/.well-known/openid-configuration"
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("discovery GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var doc struct {
		Issuer string `json:"issuer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.Issuer != d.Issuer {
		t.Errorf("issuer claim = %q, want %q", doc.Issuer, d.Issuer)
	}
}
