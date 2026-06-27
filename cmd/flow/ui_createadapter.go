package main

import (
	"context"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// engagementCreateClient adapts the rich apiclient.CreateNode to the TUI screens'
// CreateNode(ctx, name) seam; inline worktime/project creates make an engagement
// (CHECK-valid root; worktime books to engagements). Slice E replaces this with a picker.
type engagementCreateClient struct{ *apiclient.Client }

func (a engagementCreateClient) CreateNode(ctx context.Context, name string) (domain.Node, error) {
	return a.Client.CreateNode(ctx, apiclient.CreateNodeFields{Name: name, Kind: string(domain.KindEngagement)})
}
