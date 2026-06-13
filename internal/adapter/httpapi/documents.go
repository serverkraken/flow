package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// Documents implements ports.DocumentStore via the server bearer API.
// userID parameters are ignored — the server scopes to the bearer token.
type Documents struct {
	c *Client
}

// NewDocuments constructs a Documents adapter backed by c.
func NewDocuments(c *Client) *Documents {
	return &Documents{c: c}
}

var _ ports.DocumentStore = (*Documents)(nil)

// Get returns the document at path or ports.ErrDocumentNotFound.
func (d *Documents) Get(_, path string) (ports.Document, error) {
	var dto documentDTO
	err := d.c.doJSON(context.Background(), http.MethodGet,
		"/api/v1/documents/"+path, nil, -1, &dto)
	if err != nil {
		if statusCode(err) == http.StatusNotFound {
			return ports.Document{}, ports.ErrDocumentNotFound
		}
		return ports.Document{}, err
	}
	return documentFromDTO(dto)
}

// GetByRepoKey returns the repo-note alias for the given canonical key.
func (d *Documents) GetByRepoKey(_, repoKey string) (ports.Document, error) {
	var dto documentDTO
	err := d.c.doJSON(context.Background(), http.MethodGet,
		"/api/v1/repos/"+url.PathEscape(repoKey)+"/note", nil, -1, &dto)
	if err != nil {
		if statusCode(err) == http.StatusNotFound {
			return ports.Document{}, ports.ErrDocumentNotFound
		}
		return ports.Document{}, err
	}
	return documentFromDTO(dto)
}

// List returns entries matching prefix and/or query.
func (d *Documents) List(_, prefix, query string, limit int) ([]ports.DocumentEntry, error) {
	q := url.Values{}
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	if query != "" {
		q.Set("q", query)
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	path := "/api/v1/documents"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var env itemsEnvelope[entryDTO]
	if err := d.c.doJSON(context.Background(), http.MethodGet, path, nil, -1, &env); err != nil {
		return nil, err
	}
	out := make([]ports.DocumentEntry, 0, len(env.Items))
	for _, e := range env.Items {
		entry, err := entryFromDTO(e)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

// Put upserts with If-Match semantics: ifMatch 0 = create-only, N = update-only-if-version-N.
func (d *Documents) Put(_, path, body, repoKey string, ifMatch int64) (ports.Document, error) {
	reqBody := struct {
		Body string `json:"body"`
	}{Body: body}

	var targetPath string
	if repoKey != "" {
		targetPath = "/api/v1/repos/" + url.PathEscape(repoKey) + "/note"
	} else {
		targetPath = "/api/v1/documents/" + path
	}

	var dto documentDTO
	err := d.c.doJSON(context.Background(), http.MethodPut, targetPath, reqBody, ifMatch, &dto)
	if err != nil {
		if statusCode(err) == http.StatusPreconditionFailed {
			return ports.Document{}, ports.ErrDocumentVersionConflict
		}
		return ports.Document{}, err
	}
	return documentFromDTO(dto)
}

// Delete removes the document at path. Idempotent (missing = no error).
func (d *Documents) Delete(_, path string) error {
	err := d.c.doJSON(context.Background(), http.MethodDelete,
		"/api/v1/documents/"+path, nil, -1, nil)
	if err != nil {
		if statusCode(err) == http.StatusNotFound {
			return nil // idempotent
		}
		return err
	}
	return nil
}

// — helpers -------------------------------------------------------------------

func documentFromDTO(dto documentDTO) (ports.Document, error) {
	updatedAt, err := time.Parse(time.RFC3339, dto.UpdatedAt)
	if err != nil {
		return ports.Document{}, fmt.Errorf("httpapi: documents: parse updated_at %q: %w", dto.UpdatedAt, err)
	}
	return ports.Document{
		Path: dto.Path, Body: dto.Body, RepoKey: dto.RepoKey,
		Version: dto.Version, UpdatedAt: updatedAt,
	}, nil
}

func entryFromDTO(e entryDTO) (ports.DocumentEntry, error) {
	updatedAt, err := time.Parse(time.RFC3339, e.UpdatedAt)
	if err != nil {
		return ports.DocumentEntry{}, fmt.Errorf("httpapi: documents: parse entry updated_at %q: %w", e.UpdatedAt, err)
	}
	return ports.DocumentEntry{
		Path: e.Path, RepoKey: e.RepoKey, Version: e.Version,
		UpdatedAt: updatedAt, Snippet: e.Snippet,
	}, nil
}
