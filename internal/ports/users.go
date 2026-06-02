package ports

import "github.com/serverkraken/flow/internal/domain"

// UserStore persists the locally-cached User row. The client cache holds
// exactly one User (the logged-in account); the server holds multiple.
type UserStore interface {
	EnsureBySub(sub, email, displayName string) (domain.User, error)
	GetByID(id string) (domain.User, error)
	GetBySub(sub string) (domain.User, error)
}

// ErrUserNotFound is returned by UserStore when the requested user does not exist.
var ErrUserNotFound = errSentinel("flow: user not found")
