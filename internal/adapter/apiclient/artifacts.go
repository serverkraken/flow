package apiclient

import (
	"context"
	"encoding/base64"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
)

type uploadArtifactBody struct {
	Name       string `json:"name"`
	Mime       string `json:"mime"`
	DataBase64 string `json:"dataBase64"`
}

// UploadArtifact POSTs a new artifact (JSON, base64-encoded bytes) onto nodeID.
func (c *Client) UploadArtifact(ctx context.Context, nodeID, name, mime string, data []byte) (domain.Artifact, error) {
	var out domain.Artifact
	err := c.do(ctx, http.MethodPost, "/api/v1/nodes/"+nodeID+"/artifacts", uploadArtifactBody{
		Name: name, Mime: mime, DataBase64: base64.StdEncoding.EncodeToString(data),
	}, &out)
	return out, err
}

// ListArtifacts returns nodeID's reachable artifact meta (ancestor chain, not subtree).
func (c *Client) ListArtifacts(ctx context.Context, nodeID string) ([]domain.Artifact, error) {
	var out []domain.Artifact
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/"+nodeID+"/artifacts", nil, &out)
	return out, err
}

// DeleteArtifact removes one artifact by slug.
func (c *Client) DeleteArtifact(ctx context.Context, nodeID, slug string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/nodes/"+nodeID+"/artifacts/"+slug, nil, nil)
}
