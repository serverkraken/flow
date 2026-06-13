// internal/adapter/pgstore/settings_test.go
package pgstore_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
)

func TestSettings_GetSetRoundTrip(t *testing.T) {
	t.Parallel()
	s := pgstore.NewSettings(testStore)
	uid := mustUser(t, "settings-1")

	// fehlender Key → leer, kein Fehler
	v, err := s.Get(uid, "daily_target")
	if err != nil || v != "" {
		t.Fatalf("Get missing: v=%q err=%v", v, err)
	}

	if err := s.Set(uid, "daily_target", "8h"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set(uid, "daily_target", "7h"); err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}
	v, _ = s.Get(uid, "daily_target")
	if v != "7h" {
		t.Errorf("Get after Set: got %q want 7h", v)
	}

	all, err := s.All(uid)
	if err != nil || all["daily_target"] != "7h" {
		t.Errorf("All: %v %v", all, err)
	}
}

func TestSettings_Location_DefaultBerlin(t *testing.T) {
	t.Parallel()
	s := pgstore.NewSettings(testStore)
	uid := mustUser(t, "settings-2")

	loc := s.Location(uid)
	if loc.String() != "Europe/Berlin" {
		t.Errorf("default location: got %s want Europe/Berlin", loc)
	}

	_ = s.Set(uid, "timezone", "America/New_York")
	if got := s.Location(uid); got.String() != "America/New_York" {
		t.Errorf("custom location: got %s", got)
	}

	// kaputte Zeitzone → Fallback Berlin, kein Panic
	_ = s.Set(uid, "timezone", "Nicht/Existent")
	if got := s.Location(uid); got.String() != "Europe/Berlin" {
		t.Errorf("broken tz fallback: got %s", got)
	}
}
