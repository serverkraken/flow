package usecase_test

import (
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

type fakeIdentityStore struct {
	bySub    map[string]domain.User
	counts   map[string]int
	relabels [][2]string // (from,to)
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
