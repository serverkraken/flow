package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/clientmachine"
	"github.com/serverkraken/flow/internal/domain"
)

// validateBindRef enforces exactly one of project / create_name.
func validateBindRef(in bindProjectIn) error {
	hasRef := strings.TrimSpace(in.Project) != ""
	hasCreate := strings.TrimSpace(in.CreateName) != ""
	if hasRef == hasCreate {
		return errGuard{errors.New(`give either "project" (an existing project id/slug/name) or "create_name" (to create one), not both or neither`)}
	}
	return nil
}

// decideBindKind picks the binding kind. An explicit override wins ("remote"
// requires a git origin); otherwise a git origin → remote, else path.
func decideBindKind(kindOverride string, originOK bool) (string, error) {
	switch strings.TrimSpace(kindOverride) {
	case "remote":
		if !originOK {
			return "", errGuard{errors.New(`kind "remote" needs a git origin in this directory; use "path" or omit kind`)}
		}
		return "remote", nil
	case "path":
		return "path", nil
	case "":
		if originOK {
			return "remote", nil
		}
		return "path", nil
	default:
		return "", errGuard{fmt.Errorf(`invalid kind %q; use "remote" or "path", or omit to auto-detect`, kindOverride)}
	}
}

// bindProjectCore validates the request, resolves or creates the target
// project, then binds the cwd to it (remote-slug or per-device path). It is a
// method so it can reuse the cached project-ref lookup; all IO that needs the
// environment (git origin, machine id, cwd) is passed in for testability.
func (h *handlers) bindProjectCore(ctx context.Context, c *apiclient.Client, in bindProjectIn, originSlug string, originOK bool, machine clientmachine.Machine, cwd string) (domain.Project, string, error) {
	if err := validateBindRef(in); err != nil {
		return domain.Project{}, "", err
	}
	kind, err := decideBindKind(in.Kind, originOK)
	if err != nil {
		return domain.Project{}, "", err
	}
	var proj domain.Project
	if name := strings.TrimSpace(in.CreateName); name != "" {
		proj, err = c.CreateProject(ctx, name)
	} else {
		proj, err = h.lookupProject(ctx, strings.TrimSpace(in.Project))
	}
	if err != nil {
		return domain.Project{}, "", err
	}
	switch kind {
	case "remote":
		if _, err := c.BindRemote(ctx, proj.ID, originSlug); err != nil {
			return domain.Project{}, "", err
		}
	case "path":
		if machine.ID == "" {
			return domain.Project{}, "", errGuard{errors.New("cannot determine this device's machine id for a path binding")}
		}
		if _, err := c.BindPath(ctx, proj.ID, machine.ID, machine.Label, filepath.Clean(cwd)); err != nil {
			return domain.Project{}, "", err
		}
	}
	return proj, kind, nil
}
