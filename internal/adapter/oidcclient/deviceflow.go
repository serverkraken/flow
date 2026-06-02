package oidcclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

const grantTypeDeviceCode = "urn:ietf:params:oauth:grant-type:device_code"

// Config holds the runtime config of the device-flow client.
type Config struct {
	ClientID               string
	ClientSecret           string // optional — public clients leave empty
	DeviceAuthorizationURL string
	TokenURL               string
	Scopes                 []string
	HTTPClient             *http.Client
	PollIntervalOverride   time.Duration // 0 → use server's interval
}

// Codes is the response of the device-authorization request.
type Codes struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               int
	Interval                int
}

// DeviceFlow drives the RFC-8628 dance.
type DeviceFlow struct {
	cfg Config
}

func NewDeviceFlow(c Config) *DeviceFlow {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
	return &DeviceFlow{cfg: c}
}

// Init kicks off device authorization and returns the codes to display to the
// user.
func (d *DeviceFlow) Init(ctx context.Context) (Codes, error) {
	body := url.Values{}
	body.Set("client_id", d.cfg.ClientID)
	body.Set("scope", strings.Join(d.cfg.Scopes, " "))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cfg.DeviceAuthorizationURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return Codes{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if d.cfg.ClientSecret != "" {
		req.SetBasicAuth(d.cfg.ClientID, d.cfg.ClientSecret)
	}

	resp, err := d.cfg.HTTPClient.Do(req)
	if err != nil {
		return Codes{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return Codes{}, fmt.Errorf("device_authorization: status %d: %s", resp.StatusCode, string(b))
	}

	var raw struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Codes{}, err
	}
	if raw.Interval == 0 {
		raw.Interval = 5
	}
	return Codes(raw), nil
}

// PollForToken polls /token until the user approves or until expires_in is
// reached. ctx cancellation aborts immediately.
func (d *DeviceFlow) PollForToken(ctx context.Context, c Codes) (ports.Tokens, error) {
	interval := time.Duration(c.Interval) * time.Second
	if d.cfg.PollIntervalOverride > 0 {
		interval = d.cfg.PollIntervalOverride
	}
	deadline := time.Now().Add(time.Duration(c.ExpiresIn) * time.Second)

	for {
		if time.Now().After(deadline) {
			return ports.Tokens{}, errors.New("device authorization expired")
		}

		tok, err := d.exchange(ctx, c.DeviceCode)
		if err == nil {
			return tok, nil
		}
		if errors.Is(err, errAuthorizationPending) {
			select {
			case <-ctx.Done():
				return ports.Tokens{}, ctx.Err()
			case <-time.After(interval):
				continue
			}
		}
		if errors.Is(err, errSlowDown) {
			interval += 5 * time.Second
			select {
			case <-ctx.Done():
				return ports.Tokens{}, ctx.Err()
			case <-time.After(interval):
				continue
			}
		}
		return ports.Tokens{}, err
	}
}

var (
	errAuthorizationPending = errors.New("authorization_pending")
	errSlowDown             = errors.New("slow_down")
)

func (d *DeviceFlow) exchange(ctx context.Context, deviceCode string) (ports.Tokens, error) {
	body := url.Values{}
	body.Set("grant_type", grantTypeDeviceCode)
	body.Set("device_code", deviceCode)
	body.Set("client_id", d.cfg.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cfg.TokenURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return ports.Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if d.cfg.ClientSecret != "" {
		req.SetBasicAuth(d.cfg.ClientID, d.cfg.ClientSecret)
	}

	resp, err := d.cfg.HTTPClient.Do(req)
	if err != nil {
		return ports.Tokens{}, err
	}
	defer resp.Body.Close()

	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ports.Tokens{}, err
	}

	if raw.Error != "" {
		switch raw.Error {
		case "authorization_pending":
			return ports.Tokens{}, errAuthorizationPending
		case "slow_down":
			return ports.Tokens{}, errSlowDown
		default:
			return ports.Tokens{}, fmt.Errorf("token error: %s", raw.Error)
		}
	}
	return ports.Tokens{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		IDToken:      raw.IDToken,
		Expiry:       time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second),
	}, nil
}
