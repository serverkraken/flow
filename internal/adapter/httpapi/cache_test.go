package httpapi

import (
	"testing"
	"time"
)

func TestResourceCache_PutGet(t *testing.T) {
	var c resourceCache[string]
	_, ok := c.get()
	if ok {
		t.Fatal("empty cache should return ok=false")
	}
	c.put("hello")
	v, ok := c.get()
	if !ok {
		t.Fatal("after put, get should return ok=true")
	}
	if v != "hello" {
		t.Errorf("get = %q, want hello", v)
	}
}

func TestResourceCache_Invalidate_ForcesRefetch(t *testing.T) {
	var c resourceCache[string]
	c.put("data")
	c.invalidate()
	_, ok := c.get()
	if ok {
		t.Fatal("after invalidate, get should return ok=false")
	}
}

func TestResourceCache_Fallback_ReturnsStaleData(t *testing.T) {
	var c resourceCache[string]
	c.put("old")
	c.invalidate()
	v, ok := c.fallback()
	if !ok {
		t.Fatal("fallback should return ok=true even when stale")
	}
	if v != "old" {
		t.Errorf("fallback = %q, want old", v)
	}
}

func TestResourceCache_Fallback_EmptyReturnsNotOk(t *testing.T) {
	var c resourceCache[string]
	_, ok := c.fallback()
	if ok {
		t.Fatal("fallback on empty cache should return ok=false")
	}
}

func TestSnapshot_SaveLoad_Roundtrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	orig := Snapshot{
		FetchedAt: time.Now().Truncate(time.Second),
		Sessions: []sessionDTO{{
			ID:        "s1",
			ProjectID: "p1",
			Day:       "2026-06-01",
			Version:   3,
		}},
		Settings: map[string]string{"daily_target": "8h"},
	}
	if err := saveSnapshot(orig); err != nil {
		t.Fatalf("saveSnapshot: %v", err)
	}
	loaded, ok := loadSnapshot()
	if !ok {
		t.Fatal("loadSnapshot: not ok")
	}
	if len(loaded.Sessions) != 1 || loaded.Sessions[0].ID != "s1" {
		t.Errorf("sessions mismatch: %+v", loaded.Sessions)
	}
	if loaded.Settings["daily_target"] != "8h" {
		t.Errorf("settings mismatch: %+v", loaded.Settings)
	}
}

func TestSnapshot_Load_ReturnsFalseOnMissing(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	_, ok := loadSnapshot()
	if ok {
		t.Fatal("loadSnapshot should return ok=false when file missing")
	}
}
