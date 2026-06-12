package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// ErrCaptureEmpty signals that CaptureDaily was called with no text.
var ErrCaptureEmpty = errors.New("capture text is required")

// CaptureDaily appends a timestamped bullet to today's daily note
// without opening the editor. The intended UX is a tmux-popup or shell
// alias for "quick thought, get back to work" — the friction-free
// equivalent of writing on a Post-it.
//
// If today's daily doesn't exist yet, it is created with the default
// daily frontmatter and the new bullet as the only body content.
type CaptureDaily struct {
	Store ports.NoteStore
	Clock ports.Clock
}

// NewCaptureDaily wires the use case with its required ports.
func NewCaptureDaily(store ports.NoteStore, clock ports.Clock) *CaptureDaily {
	return &CaptureDaily{Store: store, Clock: clock}
}

// CaptureDailyInput carries the bullet's text content.
type CaptureDailyInput struct {
	Text string
}

// CaptureDailyOutput reports the resolved daily ID and the line that
// was appended.
type CaptureDailyOutput struct {
	ID     domain.ID
	Bullet string
	// Created is true iff the daily note was newly created by this
	// capture (i.e. it didn't exist beforehand).
	Created bool
}

// Execute appends `- HH:MM — <text>` to today's daily, creating the
// note if missing.
func (u *CaptureDaily) Execute(ctx context.Context, in CaptureDailyInput) (CaptureDailyOutput, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return CaptureDailyOutput{}, ErrCaptureEmpty
	}
	// Local time — see the rationale in CreateDaily. The bullet's HH:MM
	// must match the user's wallclock for the daily-note format to read
	// naturally.
	now := u.Clock.Now()
	date := now.Format("2006-01-02")
	id := domain.ID("daily/" + date)
	bullet := fmt.Sprintf("- %s — %s\n", now.Format("15:04"), text)

	existing, err := u.Store.Get(ctx, id)
	created := false
	var note domain.Note
	switch {
	case errors.Is(err, ports.ErrNoteNotFound):
		note, err = domain.NewNote(id, domain.Frontmatter{
			ID:   id.String(),
			Type: domain.TypeDaily,
			Date: date,
		}, []byte(bullet))
		if err != nil {
			return CaptureDailyOutput{}, fmt.Errorf("build daily: %w", err)
		}
		created = true
	case err != nil:
		return CaptureDailyOutput{}, fmt.Errorf("get daily: %w", err)
	default:
		note = existing
		note.Body = appendBullet(existing.Body, bullet)
	}

	if err := u.Store.Put(ctx, note); err != nil {
		return CaptureDailyOutput{}, fmt.Errorf("put daily: %w", err)
	}
	return CaptureDailyOutput{ID: id, Bullet: strings.TrimRight(bullet, "\n"), Created: created}, nil
}

// appendBullet joins an existing body to a new bullet, ensuring the
// result has exactly one trailing newline and a separating newline
// between body and new bullet when the body is non-empty and didn't
// already end with one.
func appendBullet(body []byte, bullet string) []byte {
	if len(body) == 0 {
		return []byte(bullet)
	}
	out := make([]byte, 0, len(body)+len(bullet)+1)
	out = append(out, body...)
	if out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	out = append(out, bullet...)
	return out
}
