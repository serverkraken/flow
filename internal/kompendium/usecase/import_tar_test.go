package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/ports"
	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestImportTar_HappyPath(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	tar := &testutil.FakeTarSnapshot{}

	u := usecase.NewImportTar(store, tar)
	out, err := u.Execute(context.Background(), usecase.ImportTarInput{
		Archive: "/tmp/snap.tar.gz",
		Mode:    ports.ConflictNewer,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Target != store.Root() {
		t.Errorf("Target got %q", out.Target)
	}
	if len(tar.Imports) != 1 {
		t.Fatalf("Import not recorded once, got %+v", tar.Imports)
	}
	if tar.Imports[0].Mode != ports.ConflictNewer {
		t.Errorf("Mode got %v", tar.Imports[0].Mode)
	}
}

func TestImportTar_AdapterError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced import err")
	tar := &testutil.FakeTarSnapshot{ImportErr: forced}

	u := usecase.NewImportTar(testutil.NewFakeNoteStore(), tar)
	_, err := u.Execute(context.Background(), usecase.ImportTarInput{Archive: "/tmp/x"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}
