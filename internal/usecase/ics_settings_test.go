package usecase_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

type fakeFeedTokens struct {
	created []domain.FeedToken
	revoked []string
}

func (f *fakeFeedTokens) Create(_ context.Context, ft domain.FeedToken) error {
	f.created = append(f.created, ft)
	return nil
}
func (f *fakeFeedTokens) Resolve(_ context.Context, token string) (string, error) {
	for _, t := range f.created {
		if t.Token == token {
			return t.UserID, nil
		}
	}
	return "", nil
}
func (f *fakeFeedTokens) ListByUser(_ context.Context, _ string) ([]domain.FeedToken, error) {
	return f.created, nil
}
func (f *fakeFeedTokens) Revoke(_ context.Context, _, token string) error {
	f.revoked = append(f.revoked, token)
	return nil
}

// fixedClock is a ports.Clock returning a constant time.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestRegenerateIcsToken_RevokesOldMintsNew(t *testing.T) {
	ft := &fakeFeedTokens{created: []domain.FeedToken{{Token: "old", UserID: "u1", Kind: "ics"}}}
	uc := usecase.RegenerateIcsToken{Tokens: ft, Clock: fixedClock{time.Now()}}
	tok, err := uc.Execute(context.Background(), "u1")
	if err != nil || tok == "" || tok == "old" {
		t.Fatalf("token = %q err=%v", tok, err)
	}
	if len(ft.revoked) != 1 || ft.revoked[0] != "old" {
		t.Fatalf("old token not revoked: %+v", ft.revoked)
	}
}

func TestIcsFeed_WritesManualOnly(t *testing.T) {
	store := newFakeDayOffs()
	day := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local)
	_ = store.Add(context.Background(), "u1", domain.DayOff{Date: day, Kind: domain.KindVacation, Label: "Sommer"})
	ft := &fakeFeedTokens{created: []domain.FeedToken{{Token: "tok", UserID: "u1", Kind: "ics"}}}
	uc := usecase.IcsFeed{Tokens: ft, Store: store, Clock: fixedClock{day}}
	var buf bytes.Buffer
	if err := uc.Execute(context.Background(), "tok", &buf); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "BEGIN:VCALENDAR") || !strings.Contains(out, "Sommer") {
		t.Fatalf("ICS missing content:\n%s", out)
	}
	if strings.Contains(out, "Neujahr") {
		t.Fatalf("holiday leaked into feed:\n%s", out)
	}
}
