package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
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

type uploadArtifactIn struct {
	Node   string `json:"node,omitempty" jsonschema:"project slug, name, or id to attach the artifact to; omit to use the current directory's project"`
	Name   string `json:"name" jsonschema:"the artifact's file name"`
	Mime   string `json:"mime" jsonschema:"the artifact's MIME type, e.g. image/png or application/pdf"`
	Base64 string `json:"base64" jsonschema:"the file contents, base64-encoded"`
}

func (h *handlers) uploadArtifact(ctx context.Context, req *mcp.CallToolRequest, in uploadArtifactIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Mime) == "" {
		return errorResult("name and mime are required"), nil, nil
	}
	data, decErr := base64.StdEncoding.DecodeString(in.Base64)
	if decErr != nil {
		return errorResult("base64: invalid encoding: " + decErr.Error()), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
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
}

func (h *handlers) listArtifacts(ctx context.Context, req *mcp.CallToolRequest, in listArtifactsIn) (*mcp.CallToolResult, any, error) {
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		nodeID, label, err := h.artifactNode(ctx, in.Node)
		if err != nil {
			return err
		}
		as, err := c.ListArtifacts(ctx, nodeID)
		if err != nil {
			return err
		}
		if len(as) == 0 {
			out = "No artifacts " + label + "."
			return nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Artifacts %s:\n", label)
		for _, a := range as {
			fmt.Fprintf(&b, "- [%s] %s · %s · %d bytes\n", a.Slug, a.Name, a.Mime, a.SizeBytes)
		}
		out = b.String()
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}

type deleteArtifactIn struct {
	Node string `json:"node,omitempty" jsonschema:"project slug, name, or id the artifact is attached to; omit to use the current directory's project"`
	Slug string `json:"slug" jsonschema:"the artifact's slug"`
}

func (h *handlers) deleteArtifact(ctx context.Context, req *mcp.CallToolRequest, in deleteArtifactIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Slug) == "" {
		return errorResult("slug is required"), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
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
