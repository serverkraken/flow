package oidcclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/oidcclient"
)

func TestResolveEndpoints_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/oidc/config" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"device_authorization_endpoint": "https://idp.example.com/device",
			"token_endpoint":                "https://idp.example.com/token",
		})
	}))
	defer srv.Close()

	deviceURL, tokenURL, err := oidcclient.ResolveEndpoints(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deviceURL != "https://idp.example.com/device" {
		t.Errorf("deviceURL: got %q, want %q", deviceURL, "https://idp.example.com/device")
	}
	if tokenURL != "https://idp.example.com/token" {
		t.Errorf("tokenURL: got %q, want %q", tokenURL, "https://idp.example.com/token")
	}
}

func TestResolveEndpoints_MissingTokenEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"device_authorization_endpoint": "https://idp.example.com/device",
		})
	}))
	defer srv.Close()

	_, _, err := oidcclient.ResolveEndpoints(context.Background(), srv.URL, nil)
	if err == nil {
		t.Fatal("expected error for missing token_endpoint, got nil")
	}
}

func TestResolveEndpoints_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, _, err := oidcclient.ResolveEndpoints(context.Background(), srv.URL, nil)
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}
