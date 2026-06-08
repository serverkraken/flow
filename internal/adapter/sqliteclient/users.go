package sqliteclient

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// Users implements ports.UserStore against the SQLite users table.
type Users struct {
	store *Store
}

// compile-time interface assertion
var _ ports.UserStore = (*Users)(nil)

// NewUsers constructs a Users sub-adapter backed by store.
func NewUsers(store *Store) *Users { return &Users{store: store} }

// EnsureBySub returns the existing user with the given OIDC sub or inserts a
// new row. The email and displayName are only applied on creation; subsequent
// calls do not overwrite them (callers must use Upsert for profile updates).
func (u *Users) EnsureBySub(sub, email, displayName string) (domain.User, error) {
	existing, err := u.GetBySub(sub)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ports.ErrUserNotFound) {
		return domain.User{}, fmt.Errorf("sqliteclient.Users.EnsureBySub: lookup: %w", err)
	}

	id := uuid.NewString()
	createdAt := time.Now().UTC().Format(time.RFC3339)
	_, err = u.store.DB().Exec(
		`INSERT INTO users (id, oidc_sub, email, display_name, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, sub, email, displayName, createdAt,
	)
	if err != nil {
		return domain.User{}, fmt.Errorf("sqliteclient.Users.EnsureBySub: insert: %w", err)
	}
	return u.GetByID(id)
}

// GetByID fetches the user with the given UUID. Returns ports.ErrUserNotFound
// when no such user exists.
func (u *Users) GetByID(id string) (domain.User, error) {
	row := u.store.DB().QueryRow(
		`SELECT id, oidc_sub, email, display_name, created_at FROM users WHERE id = ?`, id,
	)
	return scanUser(row)
}

// GetBySub fetches the user with the given OIDC sub. Returns ports.ErrUserNotFound
// when no such user exists.
func (u *Users) GetBySub(sub string) (domain.User, error) {
	row := u.store.DB().QueryRow(
		`SELECT id, oidc_sub, email, display_name, created_at FROM users WHERE oidc_sub = ?`, sub,
	)
	return scanUser(row)
}

// RelabelBySub re-points the user row identified by fromSub to a new identity,
// keeping the same primary-key id so all rows that reference it stay owned.
// Used for first-login adoption of the offline `local` profile. Caller must
// ensure toSub is not already present (oidc_sub is UNIQUE).
func (u *Users) RelabelBySub(fromSub, toSub, email, displayName string) error {
	res, err := u.store.DB().Exec(
		`UPDATE users SET oidc_sub = ?, email = ?, display_name = ? WHERE oidc_sub = ?`,
		toSub, email, displayName, fromSub,
	)
	if err != nil {
		return fmt.Errorf("sqliteclient.Users.RelabelBySub: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sqliteclient.Users.RelabelBySub: no user with sub %q", fromSub)
	}
	return nil
}

// CountOwnedRows returns how many projects + sessions reference the given user.
// Used to decide whether the first-login adoption prompt is worth showing.
func (u *Users) CountOwnedRows(userID string) (int, error) {
	var n int
	err := u.store.DB().QueryRow(
		`SELECT (SELECT COUNT(*) FROM projects WHERE user_id = ?)
		      + (SELECT COUNT(*) FROM sessions WHERE user_id = ?)`,
		userID, userID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("sqliteclient.Users.CountOwnedRows: %w", err)
	}
	return n, nil
}

func scanUser(row *sql.Row) (domain.User, error) {
	var user domain.User
	var createdAt string
	err := row.Scan(&user.ID, &user.OIDCSub, &user.Email, &user.DisplayName, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, ports.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("sqliteclient.Users: scan: %w", err)
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return domain.User{}, fmt.Errorf("sqliteclient.Users: parse created_at %q: %w", createdAt, err)
	}
	user.CreatedAt = t
	return user, nil
}
