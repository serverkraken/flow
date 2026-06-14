package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// IcsFeed resolves a feed token to its owner and writes a VCALENDAR of that
// owner's manual day-offs (vacation/sick) for [now-1y, now+1y]. Computed
// holidays are intentionally excluded — they are public and already present
// in the subscriber's calendar.
type IcsFeed struct {
	Tokens ports.FeedTokenStore
	Store  ports.DayOffStore
	Clock  ports.Clock
}

func (uc IcsFeed) Execute(ctx context.Context, token string, w io.Writer) error {
	owner, err := uc.Tokens.Resolve(ctx, token)
	if err != nil {
		return err // ports.ErrFeedTokenNotFound bubbles to a 404
	}
	now := uc.Clock.Now()
	from := now.AddDate(-1, 0, 0)
	to := now.AddDate(1, 0, 0)
	dayoffs, err := uc.Store.ListRange(ctx, owner, from, to)
	if err != nil {
		return err
	}
	return domain.WriteICS(w, dayoffs, now)
}

// RegenerateIcsToken revokes the user's existing active tokens and mints a
// fresh one. The new token is returned for display (it is the secret).
type RegenerateIcsToken struct {
	Tokens ports.FeedTokenStore
	Clock  ports.Clock
}

func (uc RegenerateIcsToken) Execute(ctx context.Context, ownerID string) (string, error) {
	existing, err := uc.Tokens.ListByUser(ctx, ownerID)
	if err != nil {
		return "", err
	}
	for _, t := range existing {
		if err := uc.Tokens.Revoke(ctx, ownerID, t.Token); err != nil {
			return "", err
		}
	}
	tok, err := newFeedToken()
	if err != nil {
		return "", err
	}
	ft := domain.FeedToken{Token: tok, UserID: ownerID, Kind: "ics", CreatedAt: uc.Clock.Now()}
	if err := uc.Tokens.Create(ctx, ft); err != nil {
		return "", err
	}
	return tok, nil
}

// newFeedToken returns a 32-byte crypto-random, base64url-encoded secret.
func newFeedToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
