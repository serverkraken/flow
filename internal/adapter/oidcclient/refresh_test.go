package oidcclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUnit_Refresh_RotatingIdP_ReturnsNewRefreshToken(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got := r.PostFormValue("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q", got)
		}
		if got := r.PostFormValue("refresh_token"); got != "old-r" {
			t.Errorf("refresh_token = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-a",
			"refresh_token": "new-r",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(srv.Close)

	got, err := Refresh(context.Background(), RefreshConfig{
		ClientID:     "flow-cli",
		TokenURL:     srv.URL,
		HTTPClient:   srv.Client(),
		RefreshToken: "old-r",
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got.AccessToken != "new-a" || got.RefreshToken != "new-r" {
		t.Fatalf("got %+v", got)
	}
	if time.Until(got.Expiry) < 30*time.Minute {
		t.Errorf("Expiry too soon: %v", got.Expiry)
	}
}

func TestUnit_Refresh_NonRotatingIdP_KeepsInputRefreshToken(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-a",
			// no refresh_token in response
			"expires_in": 1800,
		})
	}))
	t.Cleanup(srv.Close)

	got, _ := Refresh(context.Background(), RefreshConfig{
		ClientID: "flow-cli", TokenURL: srv.URL,
		HTTPClient: srv.Client(), RefreshToken: "keep-me",
	})
	if got.RefreshToken != "keep-me" {
		t.Errorf("RefreshToken = %q, want keep-me", got.RefreshToken)
	}
}

func TestUnit_Refresh_HTTPError_Returns(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	t.Cleanup(srv.Close)

	_, err := Refresh(context.Background(), RefreshConfig{
		ClientID: "flow-cli", TokenURL: srv.URL,
		HTTPClient: srv.Client(), RefreshToken: "bad",
	})
	if err == nil {
		t.Fatal("expected error on 401")
	}
}
