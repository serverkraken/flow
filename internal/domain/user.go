package domain

import "fmt"

// User is an authenticated flow user, keyed by the Authentik OIDC subject.
type User struct {
	ID          string `json:"id"`
	OIDCSub     string `json:"-"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

func NewUser(id, sub, username, email, displayName string) (User, error) {
	if id == "" {
		return User{}, fmt.Errorf("%w: id required", ErrInvalidUser)
	}
	if sub == "" {
		return User{}, fmt.Errorf("%w: oidc sub required", ErrInvalidUser)
	}
	return User{ID: id, OIDCSub: sub, Username: username, Email: email, DisplayName: displayName}, nil
}
