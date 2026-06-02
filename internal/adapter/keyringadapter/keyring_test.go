package keyringadapter

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
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
