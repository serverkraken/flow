package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/adapter/keyringadapter"
)

func TestUnit_RunLogin_HappyPath_PersistsTokens(t *testing.T) {
	t.Parallel()
	step := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/device_authorization"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":      "dev-x",
				"user_code":        "WXYZ-1234",
				"verification_uri": "http://idp.example/activate",
				"expires_in":       600,
				"interval":         1,
			})
		case strings.HasSuffix(r.URL.Path, "/token"):
			step++
			if step == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "a-tok",
				"refresh_token": "r-tok",
				"expires_in":    3600,
			})
		}
	}))
	t.Cleanup(srv.Close)

	store := keyringadapter.NewFake()
	out := &bytes.Buffer{}
	browserCalled := false

	err := runLogin(context.Background(), loginConfig{
		ClientID:               "flow-cli",
		DeviceAuthorizationURL: srv.URL + "/device_authorization",
		TokenURL:               srv.URL + "/token",
		HTTPClient:             srv.Client(),
		PollOverride:           50 * time.Millisecond,
		Store:                  store,
		SlotName:               "test-slot",
		Out:                    out,
		OpenBrowser:            func(string) error { browserCalled = true; return nil },
	})
	if err != nil {
		t.Fatalf("runLogin: %v", err)
	}

	got, _ := store.Get("test-slot")
	if got.AccessToken != "a-tok" {
		t.Errorf("AccessToken = %q", got.AccessToken)
	}
	if !strings.Contains(out.String(), "WXYZ-1234") {
		t.Errorf("output missing user code: %s", out.String())
	}
	if !browserCalled {
		t.Error("OpenBrowser should have been called")
	}
}

func TestUnit_RunLogin_DeviceInitFails_Returns(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	err := runLogin(context.Background(), loginConfig{
		ClientID:               "x",
		DeviceAuthorizationURL: srv.URL + "/dev",
		HTTPClient:             srv.Client(),
		Store:                  keyringadapter.NewFake(),
		Out:                    &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected error on 5xx from device_authorization")
	}
}

func TestUnit_SlotNameFor_StablePerServer(t *testing.T) {
	t.Parallel()
	a := slotNameFor("https://flow.example.com")
	b := slotNameFor("https://flow.example.com")
	c := slotNameFor("http://localhost:8080")
	if a != b {
		t.Error("slotNameFor must be deterministic")
	}
	if a == c {
		t.Error("different servers must produce different slots")
	}
}
