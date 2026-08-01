package httpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/serverkraken/flow/internal/actor"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// MachineAccount delegates a verified machine credential to a human owner's
// tenant. It mirrors config.MachineAccount on purpose: an adapter must not
// import the config package (adapter → usecase → ports ← domain), so
// cmd/flow-server translates between the two.
type MachineAccount struct {
	OwnerSub string
	Label    string
}

// errMachineUnmapped is returned for a machine token whose subject is absent
// from the server's configured accounts.
var errMachineUnmapped = errors.New("machine token not mapped to an owner")

// errMachineStore marks a resolveMachine failure that came from the user
// store itself rather than from a genuinely absent owner or unmapped subject —
// e.g. Postgres being unreachable. authWith checks this with errors.Is and
// answers 500, the same as the human path (middleware.go's authWith already
// does this for Ensure.Execute). Collapsing a transient store failure into
// "unknown owner" would send the operator to edit FLOW_MACHINE_ACCOUNTS for a
// mapping that was never wrong.
var errMachineStore = errors.New("machine account owner lookup failed")

// resolveMachine turns a verified machine identity into the delegated owner and
// its audit label.
//
// Machines deliberately never get a user record of their own: a second user row
// would be a second tenant with its own empty project tree, and the reports
// would then be invisible to the human they are written for.
func (s *Server) resolveMachine(ctx context.Context, id ports.Identity) (domain.User, string, error) {
	acct, ok := s.Machines[id.Subject]
	if !ok {
		return domain.User{}, "", errMachineUnmapped
	}
	u, err := s.Users.GetBySub(ctx, acct.OwnerSub)
	if err != nil {
		if !errors.Is(err, ports.ErrUserNotFound) {
			// Not a config problem — wrap for errors.Is at the call site. The
			// underlying store error never reaches the HTTP response; authWith
			// answers the same generic "server error" the human path uses.
			return domain.User{}, "", fmt.Errorf("%w: %v", errMachineStore, err)
		}
		// The owner SUBJECT is deliberately not echoed: the label already
		// identifies the entry to fix, and the subject is not the operator's to
		// read off a phone at 06:00.
		return domain.User{}, "", fmt.Errorf("machine account %q maps to an unknown owner", acct.Label)
	}
	return u, acct.Label, nil
}

// ctxWithMachine stores the delegated owner plus machine provenance. The owner
// is what every handler scopes its queries by; the actor is what the audit
// trail records.
func ctxWithMachine(ctx context.Context, u domain.User, label string) context.Context {
	ctx = context.WithValue(ctx, userKey, u)
	return actor.WithContext(ctx, actor.TrustedMachine(label))
}
