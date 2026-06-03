package sqliteserver

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Users implements ports.UserStore against the server SQLite users table.
// Users are created on first OIDC login; there is no Lamport version on users —
// they are owned by the server and never pushed from a client.
type Users struct{ store *Store }

// compile-time interface assertion
var _ ports.UserStore = (*Users)(nil)

// NewUsers constructs a Users sub-adapter backed by store.
func NewUsers(s *Store) *Users { return &Users{store: s} }

// EnsureBySub returns the existing user with the given OIDC sub or inserts a
// new row. Email and displayName are only applied on creation; subsequent
// calls do not overwrite them.
func (u *Users) EnsureBySub(sub, email, displayName string) (domain.User, error) {
	existing, err := u.GetBySub(sub)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ports.ErrUserNotFound) {
		return domain.User{}, fmt.Errorf("sqliteserver.Users.EnsureBySub: lookup: %w", err)
	}

	id := uuid.NewString()
	createdAt := time.Now().UTC().Format(time.RFC3339)
	_, err = u.store.DB().Exec(
		`INSERT INTO users (id, oidc_sub, email, display_name, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, sub, email, displayName, createdAt,
	)
	if err != nil {
		return domain.User{}, fmt.Errorf("sqliteserver.Users.EnsureBySub: insert: %w", err)
	}
	return u.GetByID(id)
}

// GetByID fetches the user with the given UUID. Returns ports.ErrUserNotFound
// when no such user exists.
func (u *Users) GetByID(id string) (domain.User, error) {
	row := u.store.DB().QueryRow(
		`SELECT id, oidc_sub, email, display_name, created_at FROM users WHERE id = ?`, id,
	)
	return scanServerUser(row)
}

// GetBySub fetches the user with the given OIDC sub. Returns ports.ErrUserNotFound
// when no such user exists.
func (u *Users) GetBySub(sub string) (domain.User, error) {
	row := u.store.DB().QueryRow(
		`SELECT id, oidc_sub, email, display_name, created_at FROM users WHERE oidc_sub = ?`, sub,
	)
	return scanServerUser(row)
}

func scanServerUser(row *sql.Row) (domain.User, error) {
	var user domain.User
	var createdAt string
	err := row.Scan(&user.ID, &user.OIDCSub, &user.Email, &user.DisplayName, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, ports.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("sqliteserver.Users: scan: %w", err)
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return domain.User{}, fmt.Errorf("sqliteserver.Users: parse created_at %q: %w", createdAt, err)
	}
	user.CreatedAt = t
	return user, nil
}
