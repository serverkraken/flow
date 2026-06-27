package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// CreateDocumentInput mirrors the server's create payload.
type CreateDocumentInput struct {
	Type      string  `json:"type"`
	NodeID *string `json:"projectId,omitempty"`
	Path      string  `json:"path"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
}

func (c *Client) CreateDocument(ctx context.Context, in CreateDocumentInput) (domain.Document, error) {
	var out domain.Document
	err := c.do(ctx, http.MethodPost, "/api/v1/documents", in, &out)
	return out, err
}

func (c *Client) ListDocuments(ctx context.Context, tags ...string) ([]domain.Document, error) {
	return c.ListDocumentsScoped(ctx, nil, tags...)
}

// ListDocumentsScoped lists documents, optionally scoped to a project.
// nodeID: nil → all; "none" → unassigned; else a project ID.
func (c *Client) ListDocumentsScoped(ctx context.Context, nodeID *string, tags ...string) ([]domain.Document, error) {
	q := url.Values{}
	if nodeID != nil {
		q.Set("projectId", *nodeID)
	}
	for _, t := range tags {
		q.Add("tag", t)
	}
	path := "/api/v1/documents"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out []domain.Document
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) Tags(ctx context.Context) ([]domain.TagCount, error) {
	var out []domain.TagCount
	err := c.do(ctx, http.MethodGet, "/api/v1/documents/tags", nil, &out)
	return out, err
}

func (c *Client) GetDocument(ctx context.Context, id string) (domain.Document, error) {
	var out domain.Document
	err := c.do(ctx, http.MethodGet, "/api/v1/documents/"+id, nil, &out)
	return out, err
}

// UpdateDocumentInput mirrors the server's update payload.
type UpdateDocumentInput struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func (c *Client) UpdateDocument(ctx context.Context, id string, in UpdateDocumentInput) (domain.Document, error) {
	var out domain.Document
	err := c.do(ctx, http.MethodPut, "/api/v1/documents/"+id, in, &out)
	return out, err
}

func (c *Client) DeleteDocument(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/documents/"+id, nil, nil)
}

// Search runs a server-side ranked search; tags AND-filter the results.
func (c *Client) Search(ctx context.Context, q string, tags ...string) ([]domain.SearchHit, error) {
	return c.SearchScoped(ctx, q, nil, tags...)
}

// SearchScoped is Search, optionally scoped to a project (see ListDocumentsScoped).
func (c *Client) SearchScoped(ctx context.Context, q string, nodeID *string, tags ...string) ([]domain.SearchHit, error) {
	v := url.Values{}
	v.Set("q", q)
	if nodeID != nil {
		v.Set("projectId", *nodeID)
	}
	for _, t := range tags {
		v.Add("tag", t)
	}
	var out []domain.SearchHit
	err := c.do(ctx, http.MethodGet, "/api/v1/documents?"+v.Encode(), nil, &out)
	return out, err
}

func (c *Client) Backlinks(ctx context.Context, id string) ([]domain.BacklinkRef, error) {
	var out []domain.BacklinkRef
	err := c.do(ctx, http.MethodGet, "/api/v1/documents/"+id+"/backlinks", nil, &out)
	return out, err
}

// ImportDocumentInput mirrors the server's import payload (verbatim persist).
type ImportDocumentInput struct {
	Type      string     `json:"type"`
	Path      string     `json:"path"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Date      *time.Time `json:"date,omitempty"`
	NodeID *string    `json:"projectId,omitempty"`
}

func (c *Client) ImportDocument(ctx context.Context, in ImportDocumentInput) (domain.Document, error) {
	var out domain.Document
	err := c.do(ctx, http.MethodPost, "/api/v1/documents/import", in, &out)
	return out, err
}
