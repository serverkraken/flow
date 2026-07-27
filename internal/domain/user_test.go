package domain

import "testing"

func TestNewUserValidates(t *testing.T) {
	if _, err := NewUser("", "sub", "u", "e", "n"); err == nil {
		t.Fatal("expected error: empty id")
	}
	if _, err := NewUser("id", "", "u", "e", "n"); err == nil {
		t.Fatal("expected error: empty sub")
	}
	u, err := NewUser("id-1", "sub-1", "msoent", "m@x.de", "Martin")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if u.OIDCSub != "sub-1" || u.Username != "msoent" {
		t.Fatalf("fields not set: %+v", u)
	}
}
