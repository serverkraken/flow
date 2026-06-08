package usecase

import "github.com/serverkraken/flow/internal/domain"

// IdentityStore is the subset of the user store the Identity use case needs.
type IdentityStore interface {
	EnsureBySub(sub, email, displayName string) (domain.User, error)
	GetBySub(sub string) (domain.User, error)
	CountOwnedRows(userID string) (int, error)
	RelabelBySub(fromSub, toSub, email, displayName string) error
}

// Identity resolves which local user a client runs as, and adopts the offline
// `local` profile into the OIDC identity on first login. See
// docs/superpowers/specs/2026-06-07-flow-client-oidc-identity-pull-remap-design.md.
type Identity struct {
	store    IdentityStore
	localSub string // FLOW_LOCAL_USER_SUB (default "local")
}

// NewIdentity constructs the Identity use case. localSub is the offline
// placeholder sub (FLOW_LOCAL_USER_SUB, default "local").
func NewIdentity(store IdentityStore, localSub string) *Identity {
	return &Identity{store: store, localSub: localSub}
}

// ResolveActiveUser returns the local user the client should run as. tokenSub is
// the sub decoded from the stored token (empty when logged out).
//
// When logged in but no OIDC user row exists yet AND an unclaimed `local`
// profile still owns data, it returns the `local` user instead of eagerly
// creating an empty OIDC user. That premature create would otherwise happen for
// EVERY command via buildDeps — including `flow logout`, which runs before it
// clears the token — and would make the first-login adoption see "OIDC user
// already exists" and silently skip the re-label. Once adoption re-labels
// `local`→sub, the GetBySub branch below returns that same row.
func (i *Identity) ResolveActiveUser(tokenSub string) (domain.User, error) {
	if tokenSub == "" {
		return i.store.EnsureBySub(i.localSub, "", "")
	}
	if u, err := i.store.GetBySub(tokenSub); err == nil {
		return u, nil // OIDC user already exists
	}
	// No OIDC user yet: if the offline `local` profile still owns data, run as it
	// so adoption can claim it on the next login rather than being defeated.
	if local, err := i.store.GetBySub(i.localSub); err == nil {
		if n, cerr := i.store.CountOwnedRows(local.ID); cerr == nil && n > 0 {
			return local, nil
		}
	}
	return i.store.EnsureBySub(tokenSub, "", "")
}

// AdoptLocalDataIfFirstLogin re-labels the `local` user into the OIDC identity
// when (a) no OIDC user for sub exists yet (first login) and (b) the `local`
// user owns data. Returns whether it adopted and how many rows it carried over.
// Caller (flow login) shows the prompt and only calls this on user consent.
func (i *Identity) AdoptLocalDataIfFirstLogin(sub, email, name string) (bool, int, error) {
	if _, err := i.store.GetBySub(sub); err == nil {
		return false, 0, nil // OIDC user already exists → not first login
	}
	localUser, err := i.store.GetBySub(i.localSub)
	if err != nil {
		return false, 0, nil // no local profile to adopt
	}
	n, err := i.store.CountOwnedRows(localUser.ID)
	if err != nil {
		return false, 0, err
	}
	if n == 0 {
		return false, 0, nil
	}
	if err := i.store.RelabelBySub(i.localSub, sub, email, name); err != nil {
		return false, 0, err
	}
	return true, n, nil
}
