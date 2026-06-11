// internal/adapter/pgstore/store_test.go
package pgstore_test

import (
	"context"
	"testing"
)

func TestOpen_MigrationsCreateAllTables(t *testing.T) {
	want := []string{
		"users", "projects", "sessions", "active_sessions",
		"documents", "day_offs", "user_settings",
	}
	for _, name := range want {
		var got string
		err := testStore.Pool().QueryRow(context.Background(),
			`SELECT table_name FROM information_schema.tables
			 WHERE table_schema = 'public' AND table_name = $1`, name).Scan(&got)
		if err != nil {
			t.Errorf("table %q missing: %v", name, err)
		}
	}
	// Kein lamport mehr — bewusst gelöscht (Spec §6).
	var n int
	_ = testStore.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name = 'lamport'`).Scan(&n)
	if n != 0 {
		t.Errorf("lamport table must not exist, found %d", n)
	}
}
