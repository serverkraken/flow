package main

import (
	"fmt"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
)

// requireType validates a required `type` argument for create against the
// canonical document-type set. Unlike the read tools' optional checkType, an
// empty value is an error.
func requireType(typ string) (domain.DocumentType, error) {
	t := strings.TrimSpace(typ)
	if t == "" {
		return "", fmt.Errorf("type is required. Valid types: %s", typeList())
	}
	for _, v := range domain.DocumentTypes() {
		if domain.DocumentType(t) == v {
			return v, nil
		}
	}
	return "", fmt.Errorf("invalid type %q. Valid types: %s", t, typeList())
}

// mergeUpdate builds the apiclient update payload from the current document and
// the optionally-supplied fields, carrying over whatever the caller omitted.
// UpdateDocumentInput requires both Title and Body, so a partial MCP update is
// realized as fetch-current-then-merge. At least one field must be supplied.
func mergeUpdate(cur domain.Document, title, body *string) (apiclient.UpdateDocumentInput, error) {
	if title == nil && body == nil {
		return apiclient.UpdateDocumentInput{}, fmt.Errorf("nothing to update: pass title and/or body")
	}
	out := apiclient.UpdateDocumentInput{Title: cur.Title, Body: cur.Body}
	if title != nil {
		out.Title = *title
	}
	if body != nil {
		out.Body = *body
	}
	return out, nil
}

// guardMutation enforces the anti-clobber write guard: a human-owned document
// (daily / project / free) may only be modified or deleted with confirm=true.
func guardMutation(d domain.Document, confirm bool) error {
	if d.Type.HumanOwned() && !confirm {
		return fmt.Errorf("%s is a human-owned note (type=%s). Pass confirm=true to modify it", d.ID, d.Type)
	}
	return nil
}
