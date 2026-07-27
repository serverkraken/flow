package apiclient

import (
	"context"
	"encoding/base64"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
)

type setNodeLogoBody struct {
	DataBase64 string `json:"dataBase64"`
}

// SetNodeLogo replaces nodeID's logo image (PUT /api/v1/nodes/{id}/logo,
// JSON with base64-encoded bytes) and returns the updated node with the
// server-stamped LogoRef.
func (c *Client) SetNodeLogo(ctx context.Context, nodeID string, data []byte) (domain.Node, error) {
	var out domain.Node
	err := c.do(ctx, http.MethodPut, "/api/v1/nodes/"+nodeID+"/logo", setNodeLogoBody{
		DataBase64: base64.StdEncoding.EncodeToString(data),
	}, &out)
	return out, err
}
