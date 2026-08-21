package apiclient

import (
	"context"
	"encoding/base64"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
)

type setNodeBannerBody struct {
	DataBase64 string `json:"dataBase64"`
}

// SetNodeBanner replaces nodeID's banner image (PUT /api/v1/nodes/{id}/banner,
// JSON with base64-encoded bytes) and returns the updated node with the
// server-stamped BannerRef. Mirrors SetNodeLogo.
func (c *Client) SetNodeBanner(ctx context.Context, nodeID string, data []byte) (domain.Node, error) {
	var out domain.Node
	err := c.do(ctx, http.MethodPut, "/api/v1/nodes/"+nodeID+"/banner", setNodeBannerBody{
		DataBase64: base64.StdEncoding.EncodeToString(data),
	}, &out)
	return out, err
}
