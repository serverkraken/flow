// internal/adapter/pgstore/users.go
package pgstore

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Users implements ports.UserStore on PG.
type Users struct{ store *Store }

func NewUsers(s *Store) *Users { return &Users{store: s} }

var _ ports.UserStore = (*Users)(nil)

const userCols = `id, oidc_sub, email, display_name, created_at`

// EnsureBySub upserts by OIDC sub and returns the canonical row.
func (u *Users) EnsureBySub(sub, email, displayName string) (domain.User, error) {
	row := u.store.Pool().QueryRow(context.Background(), `
		INSERT INTO users (id, oidc_sub, email, display_name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (oidc_sub) DO UPDATE
			SET email = EXCLUDED.email, display_name = EXCLUDED.display_name
		RETURNING `+userCols,
		uuid.NewString(), sub, email, displayName)
	return scanUser(row)
}

func (u *Users) GetByID(id string) (domain.User, error) {
	row := u.store.Pool().QueryRow(context.Background(),
		`SELECT `+userCols+` FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (u *Users) GetBySub(sub string) (domain.User, error) {
	row := u.store.Pool().QueryRow(context.Background(),
		`SELECT `+userCols+` FROM users WHERE oidc_sub = $1`, sub)
	return scanUser(row)
}

// rowScanner abstracts pgx.Row and pgx.Rows for the scan helpers in this package.
type rowScanner interface{ Scan(dest ...any) error }

func scanUser(r rowScanner) (domain.User, error) {
	var out domain.User
	err := r.Scan(&out.ID, &out.OIDCSub, &out.Email, &out.DisplayName, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ports.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	return out, nil
}
