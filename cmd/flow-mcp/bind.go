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

// validateBindRef enforces exactly one of project / create_name, and that a new
// repo (create_name) is given a parent to nest under (create_parent) — a repo can
// never be a root node.
func validateBindRef(in bindNodeIn) error {
	hasRef := strings.TrimSpace(in.Project) != ""
	hasCreate := strings.TrimSpace(in.CreateName) != ""
	if hasRef == hasCreate {
		return errGuard{errors.New(`give either "project" (an existing project id/slug/name) or "create_name" (to create one), not both or neither`)}
	}
	if hasCreate && strings.TrimSpace(in.CreateParent) == "" {
		return errGuard{errors.New(`"create_name" needs "create_parent" — the engagement or vorhaben (id/slug/name) to nest the new repo under`)}
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

// bindNodeCore validates the request, resolves or creates the target
// project, then binds the cwd to it (remote-slug or per-device path). It is a
// method so it can reuse the cached project-ref lookup; all IO that needs the
// environment (git origin, machine id, cwd) is passed in for testability.
func (h *handlers) bindNodeCore(ctx context.Context, c *apiclient.Client, in bindNodeIn, originSlug string, originOK bool, machine clientmachine.Machine, cwd string) (domain.Node, string, error) {
	if err := validateBindRef(in); err != nil {
		return domain.Node{}, "", err
	}
	kind, err := decideBindKind(in.Kind, originOK)
	if err != nil {
		return domain.Node{}, "", err
	}
	if kind == "path" && machine.ID == "" {
		return domain.Node{}, "", errGuard{errors.New("cannot determine this device's machine id for a path binding")}
	}

	creating := strings.TrimSpace(in.CreateName) != ""
	var proj domain.Node
	if creating {
		parent, perr := h.lookupNode(ctx, strings.TrimSpace(in.CreateParent))
		if perr != nil {
			return domain.Node{}, "", fmt.Errorf("create_parent: %w", perr)
		}
		binding := apiclient.BindingFields{Kind: kind}
		switch kind {
		case "remote":
			binding.RemoteSlug = originSlug
		case "path":
			binding.MachineID = machine.ID
			binding.MachineLabel = machine.Label
			binding.Path = filepath.Clean(cwd)
		}
		result, createErr := c.CreateBoundNode(ctx, apiclient.CreateBoundNodeInput{
			Node: apiclient.CreateNodeFields{
				Name:     strings.TrimSpace(in.CreateName),
				Kind:     string(domain.KindRepo),
				ParentID: &parent.ID,
			},
			Binding: binding,
		})
		proj, err = result.Node, createErr
	} else {
		proj, err = h.lookupNode(ctx, strings.TrimSpace(in.Project))
	}
	if err != nil {
		return domain.Node{}, "", err
	}
	if !creating {
		switch kind {
		case "remote":
			if _, err := c.BindRemote(ctx, proj.ID, originSlug); err != nil {
				return domain.Node{}, "", err
			}
		case "path":
			if _, err := c.BindPath(ctx, proj.ID, machine.ID, machine.Label, filepath.Clean(cwd)); err != nil {
				return domain.Node{}, "", err
			}
		}
	}
	return proj, kind, nil
}
