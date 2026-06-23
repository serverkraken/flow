package clientcheckout_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/clientcheckout"
)

func TestRecordAndGet(t *testing.T) {
	dir := t.TempDir()
	if err := clientcheckout.RecordIn(dir, "flow", "/home/me/src/flow"); err != nil {
		t.Fatal(err)
	}
	// second slug, and an overwrite of the first
	if err := clientcheckout.RecordIn(dir, "dotfiles", "/home/me/dotfiles"); err != nil {
		t.Fatal(err)
	}
	if err := clientcheckout.RecordIn(dir, "flow", "/home/me/work/flow"); err != nil {
		t.Fatal(err)
	}

	c, err := clientcheckout.LoadFrom(dir)
	if err != nil {
		t.Fatal(err)
	}
	root, ok := c.Get("flow")
	if !ok || root != "/home/me/work/flow" {
		t.Errorf("flow → %q,%v; want /home/me/work/flow,true (overwrite)", root, ok)
	}
	if r, ok := c.Get("dotfiles"); !ok || r != "/home/me/dotfiles" {
		t.Errorf("dotfiles → %q,%v", r, ok)
	}
	if _, ok := c.Get("nope"); ok {
		t.Error("unknown slug must report ok=false")
	}
}

func TestLoadFromMissingFileIsEmpty(t *testing.T) {
	c, err := clientcheckout.LoadFrom(t.TempDir())
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if _, ok := c.Get("anything"); ok {
		t.Error("empty registry must have no entries")
	}
}
