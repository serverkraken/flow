package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/testutil"
	"github.com/serverkraken/flow/internal/kompendium/usecase"
)

func TestImportBundle_HappyPath(t *testing.T) {
	t.Parallel()
	store := testutil.NewFakeNoteStore()
	bundle := &testutil.FakeNotebookBundle{}

	u := usecase.NewImportBundle(store, bundle)
	out, err := u.Execute(context.Background(), usecase.ImportBundleInput{BundlePath: "/tmp/x.bundle"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Target != store.Root() {
		t.Errorf("Target got %q", out.Target)
	}
	if len(bundle.Imports) != 1 {
		t.Errorf("Imports got %+v", bundle.Imports)
	}
}

func TestImportBundle_AdapterError(t *testing.T) {
	t.Parallel()
	forced := errors.New("forced import bundle err")
	bundle := &testutil.FakeNotebookBundle{ImportErr: forced}

	u := usecase.NewImportBundle(testutil.NewFakeNoteStore(), bundle)
	_, err := u.Execute(context.Background(), usecase.ImportBundleInput{BundlePath: "/tmp/x"})
	if !errors.Is(err, forced) {
		t.Errorf("got %v", err)
	}
}
