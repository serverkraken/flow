package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	return matched && d.NodeID != nil && *d.NodeID == proj.ID
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
	_, err := h.reconcileResources(ctx, c)
	return err
}

type resourceReconcileResult struct {
	Project string `json:"project"`
	Total   int    `json:"total"`
	Added   int    `json:"added"`
	Updated int    `json:"updated"`
	Removed int    `json:"removed"`
}

func (h *handlers) reconcileResources(ctx context.Context, c *apiclient.Client) (resourceReconcileResult, error) {
	h.resourceRefreshMu.Lock()
	defer h.resourceRefreshMu.Unlock()
	return h.reconcileResourcesLocked(ctx, c)
}

func (h *handlers) reconcileResourcesLocked(ctx context.Context, c *apiclient.Client) (resourceReconcileResult, error) {
	proj, matched := h.resolved()
	result := resourceReconcileResult{Project: "none"}
	if matched {
		result.Project = proj.Slug
	}

	// Serialize the live snapshot with local resource mutations. A create that
	// commits while this list is in flight will add itself after reconciliation,
	// instead of being removed by a pre-commit snapshot.
	h.resourceMu.Lock()
	defer h.resourceMu.Unlock()
	var docs []domain.Document
	if matched {
		var err error
		docs, err = c.ListDocumentsScoped(ctx, &proj.ID)
		if err != nil {
			return resourceReconcileResult{}, err
		}
	}
	desired := make(map[string]domain.Document, len(docs))
	for _, d := range docs {
		desired[d.ID] = d
	}
	for id := range h.resources {
		if _, ok := desired[id]; ok {
			continue
		}
		h.srv.RemoveResources(docURI(id))
		delete(h.resources, id)
		result.Removed++
	}
	for id, d := range desired {
		fingerprint := resourceFingerprint(d)
		old, exists := h.resources[id]
		if exists && old == fingerprint {
			continue
		}
		h.installResource(d)
		h.resources[id] = fingerprint
		if exists {
			result.Updated++
		} else {
			result.Added++
		}
	}
	result.Total = len(h.resources)
	return result, nil
}

func resourceFingerprint(d domain.Document) string {
	return d.Title + "\x00" + d.Path + "\x00" + string(d.Type) + "\x00" + strings.Join(d.Tags, "\x00")
}

// addResource registers (or refreshes) a document's resource. The read handler
// fetches the body fresh via GetDocument so content never goes stale. The read
// closure fetches the current client from the manager so reads survive a token
// rebuild.
func (h *handlers) addResource(ctx context.Context, d domain.Document) {
	_ = ctx
	if h.srv == nil || !h.inScope(d) {
		return
	}
	h.resourceMu.Lock()
	defer h.resourceMu.Unlock()
	h.installResource(d)
	h.resources[d.ID] = resourceFingerprint(d)
}

func (h *handlers) installResource(d domain.Document) {
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
	h.resourceMu.Lock()
	defer h.resourceMu.Unlock()
	h.srv.RemoveResources(docURI(id))
	delete(h.resources, id)
}

func (h *handlers) refreshResourcesTool(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	var out resourceReconcileResult
	err := h.do(ctx, req, func(c *apiclient.Client) error {
		var err error
		out, err = h.reconcileResources(ctx, c)
		return err
	})
	if err != nil {
		return h.resultErr(err), nil, nil
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Reconciled %d resources for %s: +%d ~%d -%d.", out.Total, out.Project, out.Added, out.Updated, out.Removed)}},
		StructuredContent: out,
	}, nil, nil
}

func (h *handlers) runResourceReconciler(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := h.mgr.Do(ctx, func(c *apiclient.Client) error {
				_, err := h.reconcileResources(ctx, c)
				return err
			}); err != nil && !errors.Is(err, errLoginRequired) && !errors.Is(err, context.Canceled) {
				mcpLog().Warn("could not reconcile document resources", "err", err)
			}
		}
	}
}
