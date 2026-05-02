package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil"
	"github.com/serverkraken/flow/internal/usecase"
)

func mkTagger(sessions []domain.Session, err error) *usecase.Tagger {
	return &usecase.Tagger{
		Sessions: &testutil.FakeSessionStore{Sessions: sessions, Err: err},
	}
}

func TestTagger_Recent_DelegatesToDomain(t *testing.T) {
	sessions := []domain.Session{
		sessAt("2026-04-25", 9, 0, time.Hour),
		sessAt("2026-04-29", 9, 0, time.Hour),
	}
	sessions[0].Tag = "old"
	sessions[1].Tag = "new"
	got, err := mkTagger(sessions, nil).Recent(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "new" {
		t.Errorf("expected [new old], got %v", got)
	}
}

func TestTagger_TopUsage_DelegatesToDomain(t *testing.T) {
	sessions := []domain.Session{
		sessAt("2026-04-25", 9, 0, time.Hour),
		sessAt("2026-04-26", 9, 0, time.Hour),
	}
	sessions[0].Tag = "deep"
	sessions[1].Tag = "deep"
	got, err := mkTagger(sessions, nil).TopUsage(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "deep" {
		t.Errorf("expected [deep], got %v", got)
	}
}

func TestTagger_RecentTemplates_DelegatesToDomain(t *testing.T) {
	d := time.Date(2026, 4, 27, 9, 0, 0, 0, time.Local)
	sessions := []domain.Session{
		{Date: d, Start: d, Stop: d.Add(time.Hour), Elapsed: time.Hour, Tag: "a"},
		{Date: d.AddDate(0, 0, 1), Start: d.AddDate(0, 0, 1), Stop: d.AddDate(0, 0, 1).Add(time.Hour), Elapsed: time.Hour, Tag: "a"},
	}
	got, err := mkTagger(sessions, nil).RecentTemplates(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 template, got %d", len(got))
	}
}

func TestTagger_NLessOrEqualZeroReturnsNil(t *testing.T) {
	tg := mkTagger(nil, nil)

	if got, err := tg.Recent(0); err != nil || got != nil {
		t.Errorf("Recent(0): got %v err %v", got, err)
	}
	if got, err := tg.TopUsage(0); err != nil || got != nil {
		t.Errorf("TopUsage(0): got %v err %v", got, err)
	}
	if got, err := tg.RecentTemplates(0); err != nil || got != nil {
		t.Errorf("RecentTemplates(0): got %v err %v", got, err)
	}
}

func TestTagger_PropagatesErrors(t *testing.T) {
	tg := mkTagger(nil, errors.New("boom"))

	if _, err := tg.Recent(5); err == nil {
		t.Error("Recent: expected error")
	}
	if _, err := tg.TopUsage(5); err == nil {
		t.Error("TopUsage: expected error")
	}
	if _, err := tg.RecentTemplates(5); err == nil {
		t.Error("RecentTemplates: expected error")
	}
}
