package apiclient

import (
	"context"
	"net/http"
)

// NodeMRU mirrors one row of the server's /nodes/mru response (server-sorted,
// newest-first).
type NodeMRU struct {
	NodeID       string `json:"nodeId"`
	LastBookedAt string `json:"lastBookedAt"`
}

func (c *Client) NodeMRU(ctx context.Context) ([]NodeMRU, error) {
	var out []NodeMRU
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/mru", nil, &out)
	return out, err
}
