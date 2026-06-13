// internal/webui/handlers/main_pg_test.go
package handlers

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/pgstore"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/testutil/pgtest"
)

// pgWebUIStore backs every store-driven handler test in this package.
var pgWebUIStore *pgstore.Store

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
		pgWebUIStore = s
		return m.Run()
	}())
}

// pgStores is the per-test fixture: fresh user + the four store adapters.
type pgStores struct {
	User      domain.User
	Sessions  *pgstore.Sessions
	Active    *pgstore.ActiveSessions
	Projects  *pgstore.Projects
	Documents *pgstore.Documents
}

func newPGStores(t testing.TB, sub string) pgStores {
	t.Helper()
	u, err := pgstore.NewUsers(pgWebUIStore).EnsureBySub(sub, sub+"@test.de", sub)
	if err != nil {
		t.Fatalf("newPGStores user: %v", err)
	}
	sessions := pgstore.NewSessions(pgWebUIStore)
	settings := pgstore.NewSettings(pgWebUIStore)
	return pgStores{
		User:      u,
		Sessions:  sessions,
		Active:    pgstore.NewActiveSessions(pgWebUIStore, sessions, settings),
		Projects:  pgstore.NewProjects(pgWebUIStore),
		Documents: pgstore.NewDocuments(pgWebUIStore),
	}
}
