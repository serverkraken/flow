package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

const docURIPrefix = "flow://doc/"

func docURI(id string) string { return docURIPrefix + id }

// inScope reports whether a document belongs to the resolved project — only such
// documents are exposed as resources.
func (h *handlers) inScope(d domain.Document) bool {
	proj, matched := h.resolved()
	return matched && d.ProjectID != nil && *d.ProjectID == proj.ID
}

// resourceFor builds the resource descriptor for a document.
func resourceFor(d domain.Document) *mcp.Resource {
	desc := fmt.Sprintf("%s · %s", d.Path, d.Type)
	if len(d.Tags) > 0 {
		desc += " · " + strings.Join(d.Tags, ", ")
	}
	return &mcp.Resource{
		URI:         docURI(d.ID),
		Name:        d.Title,
		Description: desc,
		MIMEType:    "text/markdown",
	}
}

// registerResources lists the resolved project's documents and registers a
// resource per document. No-op when no project is bound. Called from postAuthInit
// with the freshly-built client.
func (h *handlers) registerResources(ctx context.Context, c *apiclient.Client) error {
	proj, matched := h.resolved()
	if !matched {
		return nil
	}
	docs, err := c.ListDocumentsScoped(ctx, &proj.ID)
	if err != nil {
		return err
	}
	for _, d := range docs {
		h.addResource(ctx, d)
	}
	return nil
}

// addResource registers (or refreshes) a document's resource. The read handler
// fetches the body fresh via GetDocument so content never goes stale. The read
// closure fetches the current client from the manager so reads survive a token
// rebuild.
func (h *handlers) addResource(ctx context.Context, d domain.Document) {
	if h.srv == nil || !h.inScope(d) {
		return
	}
	id := d.ID
	h.srv.AddResource(resourceFor(d), func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		c, err := h.mgr.client(ctx)
		if err != nil {
			return nil, err
		}
		doc, err := c.GetDocument(ctx, id)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI: docURI(id), MIMEType: "text/markdown", Text: doc.Body,
		}}}, nil
	})
}

// removeResource unregisters a document's resource (safe if it was never added).
// RemoveResources is a no-op for URIs that were never registered, so this can be
// called unconditionally after a delete even for out-of-scope docs that addResource
// skipped.
func (h *handlers) removeResource(id string) {
	if h.srv == nil {
		return
	}
	h.srv.RemoveResources(docURI(id))
}
