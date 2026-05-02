package usecase_test

import (
	"errors"
	"testing"

	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestNoteOpener_Open_DelegatesAfterValidation(t *testing.T) {
	launcher := &testutil.FakeNoteLauncher{}
	o := &usecase.NoteOpener{Launcher: launcher}
	if err := o.Open("daily/2026-04-30"); err != nil {
		t.Fatal(err)
	}
	if len(launcher.Calls) != 1 || launcher.Calls[0] != "open:daily/2026-04-30" {
		t.Errorf("got %v", launcher.Calls)
	}
}

func TestNoteOpener_View_DelegatesAfterValidation(t *testing.T) {
	launcher := &testutil.FakeNoteLauncher{}
	o := &usecase.NoteOpener{Launcher: launcher}
	if err := o.View("daily/2026-04-30"); err != nil {
		t.Fatal(err)
	}
	if launcher.Calls[0] != "view:daily/2026-04-30" {
		t.Errorf("got %v", launcher.Calls)
	}
}

func TestNoteOpener_Open_EmptyIdFails(t *testing.T) {
	o := &usecase.NoteOpener{Launcher: &testutil.FakeNoteLauncher{}}
	if err := o.Open(""); err == nil {
		t.Error("expected error")
	}
	if err := o.View(""); err == nil {
		t.Error("View(empty) should also fail")
	}
}

func TestNoteOpener_LauncherErrPropagates(t *testing.T) {
	o := &usecase.NoteOpener{Launcher: &testutil.FakeNoteLauncher{Err: errors.New("boom")}}
	if err := o.Open("x"); err == nil {
		t.Error("expected error")
	}
	if err := o.View("x"); err == nil {
		t.Error("expected error")
	}
}
