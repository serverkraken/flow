package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestUnit_Session_HasNewFields(t *testing.T) {
	t.Parallel()
	rv := reflect.TypeOf(Session{})
	for _, want := range []string{"ID", "UserID", "ProjectID", "Version", "UpdatedAt"} {
		if _, ok := rv.FieldByName(want); !ok {
			t.Errorf("Session is missing field %q", want)
		}
	}
	for _, want := range []string{"Date", "Start", "Stop", "Elapsed", "Tag", "Note"} {
		if _, ok := rv.FieldByName(want); !ok {
			t.Errorf("Session lost legacy field %q", want)
		}
	}
	v, _ := rv.FieldByName("Version")
	if v.Type.Kind() != reflect.Int64 {
		t.Errorf("Version field kind = %s, want int64", v.Type.Kind())
	}
	u, _ := rv.FieldByName("UpdatedAt")
	if u.Type != reflect.TypeOf(time.Time{}) {
		t.Errorf("UpdatedAt type = %v, want time.Time", u.Type)
	}
}
