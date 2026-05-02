package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestExportTar_HappyPath(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	tar := &testutil.FakeTarSnapshot{}

	u := usecase.NewExportTar(store, tar)
	out, err := u.Execute(context.Background(), usecase.ExportTarInput{OutPath: "/tmp/snap.tar.gz"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Source != store.Root() || out.OutPath != "/tmp/snap.tar.gz" {
		t.Errorf("output got %+v", out)
	}
	if len(tar.Exports) != 1 {
		t.Errorf("Export not recorded once, got %+v", tar.Exports)
	}
}

func TestExportTar_AdapterError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced export err")
	tar := &testutil.FakeTarSnapshot{ExportErr: forced}

	u := usecase.NewExportTar(testutil.NewFakeNoteStore(), tar)
	_, err := u.Execute(context.Background(), usecase.ExportTarInput{OutPath: "/tmp/x"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v, want wrapped forced", err)
	}
}
