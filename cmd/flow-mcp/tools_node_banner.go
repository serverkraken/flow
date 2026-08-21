package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/artifactfile"
	"github.com/serverkraken/flow/internal/usecase"
)

type setNodeBannerIn struct {
	Node   string  `json:"node,omitempty" jsonschema:"project slug, name, or id whose banner to set; omit to use the current directory's project"`
	Path   *string `json:"path,omitempty" jsonschema:"local image file path readable by the flow-mcp process; use exactly one of path or base64"`
	Base64 *string `json:"base64,omitempty" jsonschema:"inline base64-encoded image contents; use exactly one of base64 or path"`
}

// setNodeBanner is the MCP face of PUT /api/v1/nodes/{id}/banner — the twin of
// setNodeLogo. The banner is where a register's picture lives now: the WebUI
// renders identity as monogram + colour everywhere, and the uploaded image
// shows as the wide strip on the register's landing page. Source handling and
// the client-side size guard mirror the logo tool; the server sniffs the type
// and validates size/decodability itself (usecase.ValidateNodeBanner).
func (h *handlers) setNodeBanner(ctx context.Context, req *mcp.CallToolRequest, in setNodeBannerIn) (*mcp.CallToolResult, any, error) {
	if (in.Path == nil) == (in.Base64 == nil) {
		return errorResult("exactly one of path or base64 is required"), nil, nil
	}
	var data []byte
	if in.Path != nil {
		path := strings.TrimSpace(*in.Path)
		if path == "" {
			return errorResult("path must not be empty"), nil, nil
		}
		var err error
		data, err = artifactfile.Read(path)
		if err != nil {
			return errorResult(fmt.Sprintf("read %s: %v", path, err)), nil, nil
		}
	} else {
		var err error
		data, err = decodeArtifactBase64(*in.Base64)
		if err != nil {
			return errorResult("base64: " + err.Error()), nil, nil
		}
	}
	if len(data) > usecase.MaxNodeBannerBytes {
		return errorResult(usecase.ErrBannerTooLarge.Error()), nil, nil
	}
	var out string
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		nodeID, label, err := h.artifactNode(ctx, in.Node)
		if err != nil {
			return err
		}
		n, err := c.SetNodeBanner(ctx, nodeID, data)
		if err != nil {
			return err
		}
		out = fmt.Sprintf("Set banner %s (%d bytes, ref %s).", label, len(data), n.BannerRef)
		return nil
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return textResult(out), nil, nil
}
