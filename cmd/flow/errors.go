package main

import (
	"errors"

	"github.com/serverkraken/flow/internal/adapter/httpapi"
)

// wrapRootErr translates httpapi sentinel errors into human-readable German
// messages before they are printed to stderr by the root Execute path.
// All non-sentinel errors pass through unchanged.
func wrapRootErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, httpapi.ErrUnavailable):
		return errors.New("flow-server nicht erreichbar — läuft er? (FLOW_SERVER_URL prüfen)")
	case errors.Is(err, httpapi.ErrLoggedOut):
		return errors.New("nicht angemeldet — bitte `flow login` ausführen")
	case errors.Is(err, httpapi.ErrNotConfigured):
		return errors.New("server nicht konfiguriert — $FLOW_SERVER_URL setzen")
	}
	return err
}
