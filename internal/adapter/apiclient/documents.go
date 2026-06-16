package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/serverkraken/flow/internal/domain"
)

// CreateDocumentInput mirrors the server's create payload.
type CreateDocumentInput struct {
	Type      string  `json:"type"`
	ProjectID *string `json:"projectId,omitempty"`
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
	path := "/api/v1/documents"
	if len(tags) > 0 {
		q := url.Values{}
		for _, t := range tags {
			q.Add("tag", t)
		}
		path += "?" + q.Encode()
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

func (c *Client) Backlinks(ctx context.Context, id string) ([]domain.BacklinkRef, error) {
	var out []domain.BacklinkRef
	err := c.do(ctx, http.MethodGet, "/api/v1/documents/"+id+"/backlinks", nil, &out)
	return out, err
}
