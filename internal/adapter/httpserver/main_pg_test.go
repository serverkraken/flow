// internal/adapter/httpserver/main_pg_test.go
package httpserver

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/testutil/pgtest"
)

// pgTestStore backs the new R1 API handler tests. One container per
// package; isolation via per-test users.
var pgTestStore *pgstore.Store

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
		pgTestStore = s
		return m.Run()
	}())
}
