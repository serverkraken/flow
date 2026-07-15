package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/spf13/cobra"
)

func runDocsMove(ctx context.Context, c *apiclient.Client, out io.Writer, id, typ, project, path, dateRaw string) error {
	docType, err := cliDocumentType(typ)
	if err != nil {
		return err
	}
	if docType != domain.DocDaily && strings.TrimSpace(path) == "" {
		return fmt.Errorf("--path is required for non-daily documents")
	}
	var date *time.Time
	if docType == domain.DocDaily {
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(dateRaw))
		if err != nil {
			return fmt.Errorf("--date is required for daily documents and must use YYYY-MM-DD")
		}
		date = &parsed
	} else if strings.TrimSpace(dateRaw) != "" {
		return fmt.Errorf("--date is only valid for daily documents")
	}

	nodeID, err := resolveDocsMoveProject(ctx, c, project)
	if err != nil {
		return err
	}
	doc, err := c.MoveDocument(ctx, id, apiclient.MoveDocumentInput{
		Type: string(docType), NodeID: nodeID, Path: path, Date: date,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "moved %s [%s] %s\n", doc.Type, doc.ID, doc.Path)
	return nil
}

func cliDocumentType(raw string) (domain.DocumentType, error) {
	for _, typ := range domain.DocumentTypes() {
		if raw == string(typ) {
			return typ, nil
		}
	}
	return "", fmt.Errorf("invalid --type %q", raw)
}

func resolveDocsMoveProject(ctx context.Context, c *apiclient.Client, ref string) (*string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || ref == "none" || ref == "global" {
		return nil, nil
	}
	nodes, err := c.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		if node.ID == ref {
			id := node.ID
			return &id, nil
		}
	}
	id, err := resolveNodeRef(nodes, ref)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func docsMoveCmd() *cobra.Command {
	var typ, project, path, date string
	cmd := &cobra.Command{
		Use:   "move <id>",
		Short: "Atomically change a document's type, project, path, and date",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := clientFromStore(cmd.Context())
			if err != nil {
				return err
			}
			return runDocsMove(cmd.Context(), c, cmd.OutOrStdout(), args[0], typ, project, path, date)
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "destination document type (required)")
	cmd.Flags().StringVar(&project, "project", "none", "destination node id/slug/path; none clears it")
	cmd.Flags().StringVar(&path, "path", "", "destination path (required except for daily)")
	cmd.Flags().StringVar(&date, "date", "", "daily date in YYYY-MM-DD")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}
