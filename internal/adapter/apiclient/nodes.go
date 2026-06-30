package apiclient

import (
	"context"
	"net/http"

	"github.com/serverkraken/flow/internal/domain"
)

// NodeTags returns the tags assigned to a node.
func (c *Client) NodeTags(ctx context.Context, id string) ([]domain.Tag, error) {
	var out []domain.Tag
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/"+id+"/tags", nil, &out)
	return out, err
}

// SetNodeTags replaces the tags on a node and returns the resulting set.
func (c *Client) SetNodeTags(ctx context.Context, id string, tags []string) ([]domain.Tag, error) {
	var out []domain.Tag
	err := c.do(ctx, http.MethodPut, "/api/v1/nodes/"+id+"/tags", map[string]any{"tags": tags}, &out)
	return out, err
}

// CreateNodeFields are the inputs for creating a node. JSON tags match the
// server's createNodeReq.
type CreateNodeFields struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Kind        string  `json:"kind"`
	ParentID    *string `json:"parentId"`
	Color       string  `json:"color"`
	Glyph       string  `json:"glyph"`
	Description string  `json:"description"`
	UpstreamGit string  `json:"upstreamGit"`
}

// CreateNode creates a node (engagement, vorhaben or repo).
func (c *Client) CreateNode(ctx context.Context, in CreateNodeFields) (domain.Node, error) {
	var n domain.Node
	err := c.do(ctx, http.MethodPost, "/api/v1/nodes", in, &n)
	return n, err
}

// MoveNode reparents a node. parentID nil → make it a root (engagement).
func (c *Client) MoveNode(ctx context.Context, id string, parentID *string) (domain.Node, error) {
	var n domain.Node
	err := c.do(ctx, http.MethodPost, "/api/v1/nodes/"+id+"/move",
		map[string]any{"parentId": parentID}, &n)
	return n, err
}

// Ancestors returns the node and its ancestors ordered leaf→root.
func (c *Client) Ancestors(ctx context.Context, id string) ([]domain.Node, error) {
	var out []domain.Node
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/"+id+"/ancestors", nil, &out)
	return out, err
}

// NodeRollup mirrors the server's nodeRollupDTO wire shape.
type NodeRollup struct {
	TotalMin int `json:"totalMin"`
	WeekMin  int `json:"weekMin"`
	MonthMin int `json:"monthMin"`
}

// NodeStats fetches a node's worktime rolled up over its subtree.
func (c *Client) NodeStats(ctx context.Context, id string) (NodeRollup, error) {
	var out NodeRollup
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/"+id+"/stats", nil, &out)
	return out, err
}
