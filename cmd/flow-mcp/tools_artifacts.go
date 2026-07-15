package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
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
	Name   string `json:"name" jsonschema:"the artifact's file name"`
	Mime   string `json:"mime" jsonschema:"the artifact's MIME type, e.g. image/png or application/pdf"`
	Base64 string `json:"base64" jsonschema:"the file contents, base64-encoded"`
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
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Mime) == "" {
		return errorResult("name and mime are required"), nil, nil
	}
	if in.Free && in.Node != "" {
		return errorResult(errFreeNodeExclusive), nil, nil
	}
	data, decErr := decodeArtifactBase64(in.Base64)
	if decErr != nil {
		return errorResult("base64: " + decErr.Error()), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		if in.Free {
			a, err := c.UploadFreeArtifact(ctx, in.Name, in.Mime, data)
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
		a, err := c.UploadArtifact(ctx, nodeID, in.Name, in.Mime, data)
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
