package oidcclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUnit_DeviceFlow_InitAndPoll_Happy(t *testing.T) {
	t.Parallel()
	step := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/device_authorization"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":               "dev-abc",
				"user_code":                 "ABC-123",
				"verification_uri":          "https://idp.example/activate",
				"verification_uri_complete": "https://idp.example/activate?code=ABC-123",
				"expires_in":                600,
				"interval":                  1,
			})
		case strings.HasSuffix(r.URL.Path, "/token"):
			step++
			if step == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "a-token",
				"refresh_token": "r-token",
				"id_token":      "id-token",
				"expires_in":    3600,
				"token_type":    "Bearer",
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	df := NewDeviceFlow(Config{
		ClientID:               "flow-cli",
		DeviceAuthorizationURL: srv.URL + "/device_authorization",
		TokenURL:               srv.URL + "/token",
		Scopes:                 []string{"openid", "profile", "email", "offline_access"},
		HTTPClient:             srv.Client(),
		PollIntervalOverride:   50 * time.Millisecond,
	})

	codes, err := df.Init(context.Background())
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if codes.UserCode != "ABC-123" {
		t.Errorf("UserCode = %q, want ABC-123", codes.UserCode)
	}

	tok, err := df.PollForToken(context.Background(), codes)
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if tok.AccessToken != "a-token" {
		t.Errorf("AccessToken = %q, want a-token", tok.AccessToken)
	}
	if tok.RefreshToken != "r-token" {
		t.Errorf("RefreshToken = %q", tok.RefreshToken)
	}
}

func TestUnit_DeviceFlow_PollForToken_HandlesSlowDown(t *testing.T) {
	t.Parallel()
	step := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/token") {
			http.NotFound(w, r)
			return
		}
		step++
		if step == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "slow_down"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok", "expires_in": 60,
		})
	}))
	t.Cleanup(srv.Close)

	df := NewDeviceFlow(Config{
		ClientID:               "x",
		DeviceAuthorizationURL: srv.URL + "/dev",
		TokenURL:               srv.URL + "/token",
		HTTPClient:             srv.Client(),
		PollIntervalOverride:   10 * time.Millisecond,
	})

	tok, err := df.PollForToken(context.Background(), Codes{
		DeviceCode: "x", ExpiresIn: 30, Interval: 1,
	})
	if err != nil {
		t.Fatalf("PollForToken: %v", err)
	}
	if tok.AccessToken != "tok" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}
}

func TestUnit_DeviceFlow_PollForToken_ContextCancelled_ReturnsCtxErr(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
	}))
	t.Cleanup(srv.Close)

	df := NewDeviceFlow(Config{
		ClientID:               "x",
		DeviceAuthorizationURL: srv.URL + "/dev",
		TokenURL:               srv.URL + "/token",
		HTTPClient:             srv.Client(),
		PollIntervalOverride:   200 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := df.PollForToken(ctx, Codes{DeviceCode: "x", ExpiresIn: 30, Interval: 1})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestUnit_DeviceFlow_Init_HTTPError_Returned(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server boom"))
	}))
	t.Cleanup(srv.Close)

	df := NewDeviceFlow(Config{
		ClientID:               "x",
		DeviceAuthorizationURL: srv.URL + "/dev",
		HTTPClient:             srv.Client(),
	})
	if _, err := df.Init(context.Background()); err == nil {
		t.Fatal("expected error on 5xx response")
	}
}
