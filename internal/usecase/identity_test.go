package usecase_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

type fakeIdentityStore struct {
	bySub      map[string]domain.User
	counts     map[string]int
	relabels   [][2]string // (from,to)
	soleUser   *domain.User
	soleUserOK bool
}

func (f *fakeIdentityStore) EnsureBySub(sub, _, _ string) (domain.User, error) {
	if u, ok := f.bySub[sub]; ok {
		return u, nil
	}
	u := domain.User{ID: "id-" + sub, OIDCSub: sub}
	if f.bySub == nil {
		f.bySub = map[string]domain.User{}
	}
	f.bySub[sub] = u
	return u, nil
}

func (f *fakeIdentityStore) GetBySub(sub string) (domain.User, error) {
	if u, ok := f.bySub[sub]; ok {
		return u, nil
	}
	return domain.User{}, ports.ErrUserNotFound
}
func (f *fakeIdentityStore) CountOwnedRows(id string) (int, error) { return f.counts[id], nil }
func (f *fakeIdentityStore) RelabelBySub(from, to, _, _ string) error {
	f.relabels = append(f.relabels, [2]string{from, to})
	u := f.bySub[from]
	delete(f.bySub, from)
	u.OIDCSub = to
	f.bySub[to] = u
	return nil
}

func (f *fakeIdentityStore) SoleUser() (domain.User, bool, error) {
	if f.soleUserOK && f.soleUser != nil {
		return *f.soleUser, true, nil
	}
	return domain.User{}, false, nil
}

func TestUnit_Identity_ResolveActiveUser_FallsBackToLocalWhenNoSub(t *testing.T) {
	store := &fakeIdentityStore{}
	id := usecase.NewIdentity(store, "local")
	u, err := id.ResolveActiveUser("") // no token sub
	if err != nil {
		t.Fatal(err)
	}
	if u.OIDCSub != "local" {
		t.Errorf("sub = %q, want local", u.OIDCSub)
	}
}

func TestUnit_Identity_ResolveActiveUser_RunsAsLocalWhenOidcAbsentAndLocalHasData(t *testing.T) {
	// Footgun guard: logged in (token sub present) but no OIDC user row yet, and
	// the offline `local` profile still owns data → run as `local`, do NOT
	// eagerly create an empty OIDC user (which would defeat first-login adoption).
	store := &fakeIdentityStore{
		bySub:  map[string]domain.User{"local": {ID: "id-local", OIDCSub: "local"}},
		counts: map[string]int{"id-local": 3},
	}
	id := usecase.NewIdentity(store, "local")
	u, err := id.ResolveActiveUser("msoent")
	if err != nil {
		t.Fatal(err)
	}
	if u.OIDCSub != "local" {
		t.Errorf("sub = %q, want local (must not pre-create the OIDC user while local has unclaimed data)", u.OIDCSub)
	}
	if _, ok := store.bySub["msoent"]; ok {
		t.Error("must NOT have created an OIDC user — that would defeat adoption")
	}
}

func TestUnit_Identity_ResolveActiveUser_CreatesOidcWhenNoLocalData(t *testing.T) {
	store := &fakeIdentityStore{} // no local profile to adopt
	id := usecase.NewIdentity(store, "local")
	u, err := id.ResolveActiveUser("msoent")
	if err != nil {
		t.Fatal(err)
	}
	if u.OIDCSub != "msoent" {
		t.Errorf("sub = %q, want msoent (nothing to adopt → create the OIDC user)", u.OIDCSub)
	}
}

func TestUnit_Identity_Adopt_RelabelsLocalWhenFirstLoginWithData(t *testing.T) {
	store := &fakeIdentityStore{
		bySub:  map[string]domain.User{"local": {ID: "id-local", OIDCSub: "local"}},
		counts: map[string]int{"id-local": 3},
	}
	id := usecase.NewIdentity(store, "local")
	adopted, n, err := id.AdoptLocalDataIfFirstLogin("msoent", "m@x.de", "Soenne")
	if err != nil {
		t.Fatal(err)
	}
	if !adopted || n != 3 {
		t.Fatalf("adopted=%v n=%d, want true/3", adopted, n)
	}
	if len(store.relabels) != 1 || store.relabels[0] != [2]string{"local", "msoent"} {
		t.Errorf("relabels = %v", store.relabels)
	}
}

func TestUnit_Identity_Adopt_SkipsWhenOidcUserAlreadyExists(t *testing.T) {
	store := &fakeIdentityStore{
		bySub:  map[string]domain.User{"local": {ID: "id-local", OIDCSub: "local"}, "msoent": {ID: "id-msoent", OIDCSub: "msoent"}},
		counts: map[string]int{"id-local": 3},
	}
	id := usecase.NewIdentity(store, "local")
	adopted, _, err := id.AdoptLocalDataIfFirstLogin("msoent", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if adopted {
		t.Error("must not adopt when an OIDC user already exists (not first login)")
	}
}

// TestUnit_Identity_ResolveActiveUserEmptySubFallsBackToSoleUser checks that
// when the local sub is absent (already relabeled to OIDC) and tokenSub is ""
// (e.g. locked keychain), ResolveActiveUser returns the sole DB user instead of
// minting a fresh `local` row and forking the data.
func TestUnit_Identity_ResolveActiveUserEmptySubFallsBackToSoleUser(t *testing.T) {
	oidcUser := domain.User{ID: "id-msoent", OIDCSub: "msoent"}
	store := &fakeIdentityStore{
		// Only the adopted OIDC user exists — no "local" row.
		bySub:      map[string]domain.User{"msoent": oidcUser},
		soleUser:   &oidcUser,
		soleUserOK: true,
	}
	id := usecase.NewIdentity(store, "local")
	u, err := id.ResolveActiveUser("")
	if err != nil {
		t.Fatal(err)
	}
	if u.OIDCSub != "msoent" {
		t.Errorf("sub = %q, want msoent (sole-user fallback)", u.OIDCSub)
	}
	if _, localCreated := store.bySub["local"]; localCreated {
		t.Error("must NOT have created a fresh 'local' user — that would fork the data")
	}
}
