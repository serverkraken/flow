package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func TestLinkWriter_Add_HappyPath(t *testing.T) {
	store := &testutil.FakeLinkStore{}
	w := &usecase.LinkWriter{Store: store}
	d := time.Date(2026, 4, 30, 0, 0, 0, 0, time.Local)
	if err := w.Add(d, "daily/2026-04-30"); err != nil {
		t.Fatal(err)
	}
	if got := store.ByDate[d.Format("2006-01-02")]; len(got) != 1 {
		t.Errorf("expected 1 link, got %v", got)
	}
}

func TestLinkWriter_Add_TrimsAndValidates(t *testing.T) {
	w := &usecase.LinkWriter{Store: &testutil.FakeLinkStore{}}
	d := time.Now()

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"empty", "", true},
		{"whitespace only", "   ", true},
		{"contains tab", "daily/x\ty", true},
		{"contains newline", "daily/x\ny", true},
		{"contains carriage return", "daily/x\ry", true},
		{"valid with trim", "  daily/x  ", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := w.Add(d, tc.id)
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestLinkWriter_Add_StoreErr(t *testing.T) {
	w := &usecase.LinkWriter{Store: &testutil.FakeLinkStore{Err: errors.New("boom")}}
	if err := w.Add(time.Now(), "daily/x"); err == nil {
		t.Error("expected error")
	}
}

func TestLinkWriter_Remove(t *testing.T) {
	d := time.Date(2026, 4, 30, 0, 0, 0, 0, time.Local)
	store := &testutil.FakeLinkStore{ByDate: map[string][]string{
		d.Format("2006-01-02"): {"daily/2026-04-30"},
	}}
	w := &usecase.LinkWriter{Store: store}
	if err := w.Remove(d, "daily/2026-04-30"); err != nil {
		t.Fatal(err)
	}
	if got := store.ByDate[d.Format("2006-01-02")]; len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}
