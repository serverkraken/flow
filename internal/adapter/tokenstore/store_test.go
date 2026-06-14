package tokenstore

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/zalando/go-keyring"
)

func TestOpenReturnsWorkingStore(t *testing.T) {
	keyring.MockInit()
	s := Open()
	if s == nil {
		t.Fatal("Open returned nil")
	}
	tok := ports.Token{AccessToken: "x", Expiry: time.Unix(1, 0).UTC()}
	if err := s.Save(tok); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := s.Load()
	if err != nil || !ok || got.AccessToken != "x" {
		t.Fatalf("load: got=%+v ok=%v err=%v", got, ok, err)
	}
	_ = s.Clear()
}

func TestDefaultFilePath(t *testing.T) {
	p, err := defaultFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if p == "" {
		t.Fatal("empty default path")
	}
}
