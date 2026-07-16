package main

import (
	"context"
	"encoding/base64"
	"fmt"
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
	Node   string  `json:"node,omitempty" jsonschema:"project slug, name, or id to attach the artifact to; omit to use the current directory's project"`
	Free   bool    `json:"free,omitempty" jsonschema:"upload/list/delete in the owner-global free (node-less) library instead of a node"`
	Path   *string `json:"path,omitempty" jsonschema:"local file path readable by the flow-mcp process; use exactly one of path or base64"`
	Name   string  `json:"name,omitempty" jsonschema:"artifact file name; defaults to the path basename in path mode and is required in base64 mode"`
	Mime   string  `json:"mime,omitempty" jsonschema:"artifact MIME type; guessed from the extension in path mode and required in base64 mode"`
	Base64 *string `json:"base64,omitempty" jsonschema:"inline base64-encoded file contents; use exactly one of base64 or path"`
}

func decodeArtifactBase64(raw string) ([]byte, error) {
	maxEncoded := base64.StdEncoding.EncodedLen(int(domain.MaxArtifactBytes))
	if len(raw) > maxEncoded {
		return nil, fmt.Errorf("artifact exceeds %d bytes", domain.MaxArtifactBytes)
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid encoding: %w", err)
	}
	if int64(len(data)) > domain.MaxArtifactBytes {
		return nil, fmt.Errorf("artifact exceeds %d bytes", domain.MaxArtifactBytes)
	}
	return data, nil
}

func (h *handlers) uploadArtifact(ctx context.Context, req *mcp.CallToolRequest, in uploadArtifactIn) (*mcp.CallToolResult, any, error) {
	if in.Free && in.Node != "" {
		return errorResult(errFreeNodeExclusive), nil, nil
	}
	if (in.Path == nil) == (in.Base64 == nil) {
		return errorResult("exactly one of path or base64 is required"), nil, nil
	}

	name := strings.TrimSpace(in.Name)
	mimeType := strings.TrimSpace(in.Mime)
	var data []byte
	if in.Path != nil {
		path := strings.TrimSpace(*in.Path)
		if path == "" {
			return errorResult("path must not be empty"), nil, nil
		}
		var readErr error
		data, readErr = artifactfile.Read(path)
		if readErr != nil {
			return errorResult(fmt.Sprintf("read %s: %v", path, readErr)), nil, nil
		}
		if name == "" {
			name = filepath.Base(path)
		}
		mimeType = artifactfile.GuessMime(path, mimeType)
	} else {
		if name == "" || mimeType == "" {
			return errorResult("name and mime are required in base64 mode"), nil, nil
		}
		var decErr error
		data, decErr = decodeArtifactBase64(*in.Base64)
		if decErr != nil {
			return errorResult("base64: " + decErr.Error()), nil, nil
		}
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
