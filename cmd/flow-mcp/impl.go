package main

import (
	"errors"

	"github.com/serverkraken/flow/internal/adapter/mcpstdio"
	"github.com/serverkraken/flow/internal/usecase"
)

// impl bridges the usecase-level MCPTools dispatcher to the
// mcpstdio.Implementation interface. The conversion is a few field
// copies — keeping it in cmd/flow-mcp is intentional so the usecase
// layer stays free of any wire-protocol types.
type impl struct {
	tools *usecase.MCPTools
}

func newImpl(tools *usecase.MCPTools) *impl {
	return &impl{tools: tools}
}

func (i *impl) Tools() []mcpstdio.Tool {
	cat := i.tools.Catalog()
	out := make([]mcpstdio.Tool, 0, len(cat))
	for _, t := range cat {
		out = append(out, mcpstdio.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return out
}

func (i *impl) Resources() []mcpstdio.Resource {
	cat := i.tools.ResourceCatalog()
	out := make([]mcpstdio.Resource, 0, len(cat))
	for _, r := range cat {
		out = append(out, mcpstdio.Resource{
			URI:         r.URI,
			Name:        r.Name,
			Description: r.Description,
			MimeType:    r.MimeType,
		})
	}
	return out
}

func (i *impl) CallTool(name string, args map[string]any) mcpstdio.CallToolResult {
	r := i.tools.Call(name, args)
	return mcpstdio.CallToolResult{
		Content: []mcpstdio.Content{{Type: "text", Text: r.Text}},
		IsError: r.IsError,
	}
}

func (i *impl) ReadResource(uri string) (mcpstdio.ResourceContent, error) {
	c, err := i.tools.ReadResource(uri)
	if err != nil {
		if errors.Is(err, usecase.ErrResourceNotFound) {
			return mcpstdio.ResourceContent{}, err
		}
		return mcpstdio.ResourceContent{}, err
	}
	return mcpstdio.ResourceContent{
		URI:      c.URI,
		MimeType: c.MimeType,
		Text:     c.Text,
	}, nil
}
