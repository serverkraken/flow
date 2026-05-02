package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestExportBundle_HappyPath(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	bundle := &testutil.FakeNotebookBundle{}

	u := usecase.NewExportBundle(store, bundle)
	out, err := u.Execute(context.Background(), usecase.ExportBundleInput{OutPath: "/tmp/snap.bundle"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.OutPath != "/tmp/snap.bundle" || out.Source != store.Root() {
		t.Errorf("output got %+v", out)
	}
	if len(bundle.Exports) != 1 {
		t.Errorf("Exports got %+v", bundle.Exports)
	}
}

func TestExportBundle_AdapterError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced bundle err")
	bundle := &testutil.FakeNotebookBundle{ExportErr: forced}

	u := usecase.NewExportBundle(testutil.NewFakeNoteStore(), bundle)
	_, err := u.Execute(context.Background(), usecase.ExportBundleInput{OutPath: "/tmp/x"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}
