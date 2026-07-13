package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/artifactfile"
	"github.com/serverkraken/flow/internal/domain"
)

// artifactNode resolves a tool's optional `node` argument to a concrete node ID
// via resolveScope, rejecting the "global"/"none" scopes that make no sense for
// a single-node artifact library — an artifact must live on exactly one node.
func (h *handlers) artifactNode(ctx context.Context, node string) (string, string, error) {
	sc, err := h.resolveScope(ctx, node)
	if err != nil {
		return "", "", err
	}
	if sc.nodeID == nil || *sc.nodeID == "none" {
		return "", "", errGuard{fmt.Errorf("a node is required (pass node=<slug/name/id>, or bind a project to this directory with flow_bind_project)")}
	}
	return *sc.nodeID, sc.label, nil
}

// errFreeNodeExclusive is the MCP-side mutual-exclusion error for
// free:true + a non-empty node — the free (owner-global, node-less) library
// and a node-scoped library are mutually exclusive targets (OE #1).
const errFreeNodeExclusive = "free and node are mutually exclusive"

type uploadArtifactIn struct {
	Node   string `json:"node,omitempty" jsonschema:"project slug, name, or id to attach the artifact to; omit to use the current directory's project"`
	Free   bool   `json:"free,omitempty" jsonschema:"upload/list/delete in the owner-global free (node-less) library instead of a node"`
	Path   string `json:"path,omitempty" jsonschema:"absolute or relative filesystem path the local MCP process reads directly (relative resolves against the MCP process's working directory); preferred for files on disk. Mutually exclusive with base64."`
	Name   string `json:"name,omitempty" jsonschema:"the artifact's file name; optional with path (defaults to the file's basename), required with base64"`
	Mime   string `json:"mime,omitempty" jsonschema:"the artifact's MIME type, e.g. image/png; optional with path (guessed from the extension), required with base64"`
	Base64 string `json:"base64,omitempty" jsonschema:"the file contents, base64-encoded; use for small generated content. Mutually exclusive with path."`
}

func (h *handlers) uploadArtifact(ctx context.Context, req *mcp.CallToolRequest, in uploadArtifactIn) (*mcp.CallToolResult, any, error) {
	if in.Free && in.Node != "" {
		return errorResult(errFreeNodeExclusive), nil, nil
	}
	hasPath := strings.TrimSpace(in.Path) != ""
	hasB64 := strings.TrimSpace(in.Base64) != ""
	switch {
	case hasPath && hasB64:
		return errorResult("provide either path or base64, not both"), nil, nil
	case !hasPath && !hasB64:
		return errorResult("provide either path or base64"), nil, nil
	}

	var data []byte
	name, mimeType := in.Name, in.Mime
	if hasPath {
		b, readErr := os.ReadFile(in.Path)
		if readErr != nil {
			return errorResult(fmt.Sprintf("read %s: %v", in.Path, readErr)), nil, nil
		}
		data = b
		if strings.TrimSpace(name) == "" {
			name = filepath.Base(in.Path)
		}
		mimeType = artifactfile.GuessMime(in.Path, in.Mime)
	} else {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(mimeType) == "" {
			return errorResult("name and mime are required"), nil, nil
		}
		b, decErr := base64.StdEncoding.DecodeString(in.Base64)
		if decErr != nil {
			return errorResult("base64: invalid encoding: " + decErr.Error()), nil, nil
		}
		data = b
	}

	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		if in.Free {
			a, err := c.UploadFreeArtifact(ctx, name, mimeType, data)
			if err != nil {
				return err
			}
			out = fmt.Sprintf("Uploaded artifact [%s] %s (%s, %d bytes) %s.", a.Slug, a.Name, a.Mime, a.SizeBytes, "(free library)")
			return nil
		}
		nodeID, label, err := h.artifactNode(ctx, in.Node)
		if err != nil {
			return err
		}
		a, err := c.UploadArtifact(ctx, nodeID, name, mimeType, data)
		if err != nil {
			return err
		}
		out = fmt.Sprintf("Uploaded artifact [%s] %s (%s, %d bytes) %s.", a.Slug, a.Name, a.Mime, a.SizeBytes, label)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

type listArtifactsIn struct {
	Node string `json:"node,omitempty" jsonschema:"project slug, name, or id whose artifact library (incl. ancestors) to list; omit to use the current directory's project"`
	Free bool   `json:"free,omitempty" jsonschema:"upload/list/delete in the owner-global free (node-less) library instead of a node"`
}

func (h *handlers) listArtifacts(ctx context.Context, req *mcp.CallToolRequest, in listArtifactsIn) (*mcp.CallToolResult, any, error) {
	if in.Free && in.Node != "" {
		return errorResult(errFreeNodeExclusive), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		if in.Free {
			as, err := c.ListFreeArtifacts(ctx)
			if err != nil {
				return err
			}
			out = formatArtifactList(as, "(free library)")
			return nil
		}
		nodeID, label, err := h.artifactNode(ctx, in.Node)
		if err != nil {
			return err
		}
		as, err := c.ListArtifacts(ctx, nodeID)
		if err != nil {
			return err
		}
		out = formatArtifactList(as, label)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

func formatArtifactList(as []domain.Artifact, label string) string {
	if len(as) == 0 {
		return "No artifacts " + label + "."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Artifacts %s:\n", label)
	for _, a := range as {
		fmt.Fprintf(&b, "- [%s] %s · %s · %d bytes\n", a.Slug, a.Name, a.Mime, a.SizeBytes)
	}
	return b.String()
}

type deleteArtifactIn struct {
	Node string `json:"node,omitempty" jsonschema:"project slug, name, or id the artifact is attached to; omit to use the current directory's project"`
	Free bool   `json:"free,omitempty" jsonschema:"upload/list/delete in the owner-global free (node-less) library instead of a node"`
	Slug string `json:"slug" jsonschema:"the artifact's slug"`
}

func (h *handlers) deleteArtifact(ctx context.Context, req *mcp.CallToolRequest, in deleteArtifactIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Slug) == "" {
		return errorResult("slug is required"), nil, nil
	}
	if in.Free && in.Node != "" {
		return errorResult(errFreeNodeExclusive), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		if in.Free {
			if err := c.DeleteFreeArtifact(ctx, in.Slug); err != nil {
				return err
			}
			out = fmt.Sprintf("Deleted artifact [%s] %s.", in.Slug, "(free library)")
			return nil
		}
		nodeID, label, err := h.artifactNode(ctx, in.Node)
		if err != nil {
			return err
		}
		if err := c.DeleteArtifact(ctx, nodeID, in.Slug); err != nil {
			return err
		}
		out = fmt.Sprintf("Deleted artifact [%s] %s.", in.Slug, label)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
