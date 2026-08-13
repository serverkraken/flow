package tokenstore

import (
	"context"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/ports"
	"github.com/zalando/go-keyring"
)

func TestOpenReturnsWorkingStore(t *testing.T) {
	oldUserConfigDir := userConfigDir
	userConfigDir = func() (string, error) { return t.TempDir(), nil }
	t.Cleanup(func() { userConfigDir = oldUserConfigDir })
	keyring.MockInit()
	s := Open()
	if s == nil {
		t.Fatal("Open returned nil")
	}
	tok := ports.Token{AccessToken: "x", Expiry: time.Unix(1, 0).UTC()}
	if err := s.WithLock(context.Background(), func(session ports.TokenStoreSession) error {
		if err := session.Save(tok); err != nil {
			return err
		}
		got, ok, err := session.Load()
		if err != nil || !ok || got.AccessToken != "x" {
			t.Fatalf("load: got=%+v ok=%v err=%v", got, ok, err)
		}
		return session.Clear()
	}); err != nil {
		t.Fatalf("transaction: %v", err)
	}
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

func TestDefaultLockPath(t *testing.T) {
	p, err := defaultLockPath()
	if err != nil {
		t.Fatal(err)
	}
	if p == "" {
		t.Fatal("empty default lock path")
	}
}
