// internal/adapter/pgstore/main_test.go
package pgstore_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/testutil/pgtest"
)

// testStore is shared across the package's tests. Isolation comes from
// per-test users — every table is user-scoped.
var testStore *pgstore.Store

func TestMain(m *testing.M) {
	os.Exit(func() int {
		ctx := context.Background()
		dsn, terminate, err := pgtest.StartContainer(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer terminate()
		s, err := pgstore.Open(ctx, dsn)
		if err != nil {
			fmt.Fprintln(os.Stderr, "pgstore open:", err)
			return 1
		}
		defer s.Close()
		testStore = s
		return m.Run()
	}())
}
