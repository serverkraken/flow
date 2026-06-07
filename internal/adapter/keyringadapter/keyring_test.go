package keyringadapter

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/zalando/go-keyring"
)

func TestUnit_Fake_PutGetDelete_Roundtrip(t *testing.T) {
	t.Parallel()
	ks := NewFake()

	want := ports.Tokens{
		AccessToken:  "access",
		RefreshToken: "refresh",
		IDToken:      "id",
		Expiry:       time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC),
	}
	if err := ks.Put("slot-a", want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := ks.Get("slot-a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if err := ks.Delete("slot-a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := ks.Get("slot-a"); !errors.Is(err, ports.ErrTokenNotFound) {
		t.Fatalf("Get after Delete: err = %v, want ErrTokenNotFound", err)
	}
}

func TestUnit_Fake_Get_Missing_ReturnsErrTokenNotFound(t *testing.T) {
	t.Parallel()
	ks := NewFake()
	if _, err := ks.Get("nope"); !errors.Is(err, ports.ErrTokenNotFound) {
		t.Fatalf("err = %v, want ErrTokenNotFound", err)
	}
}

func TestUnit_Fake_OverwriteSameSlot(t *testing.T) {
	t.Parallel()
	ks := NewFake()
	_ = ks.Put("slot", ports.Tokens{AccessToken: "v1"})
	_ = ks.Put("slot", ports.Tokens{AccessToken: "v2"})
	got, err := ks.Get("slot")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AccessToken != "v2" {
		t.Fatalf("AccessToken = %q, want v2", got.AccessToken)
	}
}

func TestUnit_Fake_DeleteMissingSlot_NoError(t *testing.T) {
	t.Parallel()
	ks := NewFake()
	if err := ks.Delete("never-existed"); err != nil {
		t.Errorf("Delete missing: err = %v, want nil", err)
	}
}

// The real Keyring tests below use go-keyring's MockInit() backend so they
// run without touching the real OS keychain. MockInit is process-global,
// so these tests share a backend — they MUST NOT run in parallel with each
// other or with anything else that touches go-keyring.

func TestUnit_Keyring_RoundtripsLargeBundle(t *testing.T) {
	keyring.MockInit()
	ks := New()

	// A 6 KiB total payload — well above the historic 2 KiB per-item limit
	// that broke the legacy single-slot implementation. Splitting per
	// field keeps every individual entry small.
	want := ports.Tokens{
		AccessToken:  strings.Repeat("A", 1500),
		RefreshToken: strings.Repeat("R", 700),
		IDToken:      strings.Repeat("I", 3500),
		Expiry:       time.Date(2026, 6, 7, 19, 0, 0, 123_456_789, time.UTC),
	}
	if err := ks.Put("server-x", want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := ks.Get("server-x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken ||
		got.IDToken != want.IDToken || !got.Expiry.Equal(want.Expiry) {
		t.Fatalf("roundtrip mismatch\n got=%+v\nwant=%+v", got, want)
	}
}

func TestUnit_Keyring_DeleteRemovesAllSlots(t *testing.T) {
	keyring.MockInit()
	ks := New()

	_ = ks.Put("slot-d", ports.Tokens{
		AccessToken: "a", RefreshToken: "r", IDToken: "i",
		Expiry: time.Now().Add(time.Hour),
	})
	if err := ks.Delete("slot-d"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// All four sub-slots must be gone — Get returns ErrTokenNotFound when
	// the .access entry is missing.
	if _, err := ks.Get("slot-d"); !errors.Is(err, ports.ErrTokenNotFound) {
		t.Fatalf("Get after Delete: err = %v, want ErrTokenNotFound", err)
	}
	// Explicitly check the .refresh sub-slot too so a regression where
	// Delete only touches .access would surface here.
	if _, err := keyring.Get(service, "slot-d"+suffixRefresh); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf(".refresh sub-slot still present: err = %v", err)
	}
}

func TestUnit_Keyring_GetMissing_ReturnsErrTokenNotFound(t *testing.T) {
	keyring.MockInit()
	ks := New()
	if _, err := ks.Get("never-logged-in"); !errors.Is(err, ports.ErrTokenNotFound) {
		t.Fatalf("err = %v, want ErrTokenNotFound", err)
	}
}

func TestUnit_Keyring_GetWithOnlyAccessSlot_DegradesGracefully(t *testing.T) {
	keyring.MockInit()
	// Simulate a half-written or partially-deleted slot — only the access
	// token survives. Get must still succeed; the optional fields come
	// back zero-valued so the caller can detect a stale slot and re-login
	// rather than blow up with an unmarshal error.
	if err := keyring.Set(service, "half"+suffixAccess, "lone-access-token"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ks := New()
	got, err := ks.Get("half")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AccessToken != "lone-access-token" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "lone-access-token")
	}
	if got.RefreshToken != "" || got.IDToken != "" || !got.Expiry.IsZero() {
		t.Errorf("optional fields not zero-valued: %+v", got)
	}
}
