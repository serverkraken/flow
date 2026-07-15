package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// AuditDocuments inspects all active and archived documents for one owner. It
// deliberately exposes no write port and therefore cannot repair data.
type AuditDocuments struct {
	Docs  ports.DocumentStore
	Nodes ports.NodeStore
}

func (uc AuditDocuments) Execute(ctx context.Context, ownerID string) (domain.DocumentAuditReport, error) {
	active, err := uc.Docs.List(ctx, ownerID, nil)
	if err != nil {
		return domain.DocumentAuditReport{}, err
	}
	archived, err := uc.Docs.ListArchived(ctx, ownerID)
	if err != nil {
		return domain.DocumentAuditReport{}, err
	}
	nodes, err := uc.Nodes.List(ctx, ownerID)
	if err != nil {
		return domain.DocumentAuditReport{}, err
	}
	return BuildDocumentAuditReport(active, archived, nodes), nil
}

// BuildDocumentAuditReport applies the pure audit rules to an already
// owner-scoped snapshot. The CLI uses it as a compatibility fallback when an
// older server does not yet expose the maintenance endpoint.
func BuildDocumentAuditReport(active, archived []domain.Document, nodes []domain.Node) domain.DocumentAuditReport {
	validTypes := make(map[domain.DocumentType]struct{}, len(domain.DocumentTypes()))
	for _, typ := range domain.DocumentTypes() {
		validTypes[typ] = struct{}{}
	}
	ownedNodes := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		ownedNodes[node.ID] = struct{}{}
	}

	report := domain.DocumentAuditReport{
		Scanned: len(active) + len(archived), Active: len(active), Archived: len(archived),
		Issues: []domain.DocumentAuditIssue{},
	}
	for _, doc := range append(active, archived...) {
		report.Issues = append(report.Issues, auditDocument(doc, validTypes, ownedNodes)...)
	}
	sort.Slice(report.Issues, func(i, j int) bool {
		a, b := report.Issues[i], report.Issues[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.DocumentID != b.DocumentID {
			return a.DocumentID < b.DocumentID
		}
		return a.Code < b.Code
	})
	return report
}

func auditDocument(doc domain.Document, validTypes map[domain.DocumentType]struct{}, ownedNodes map[string]struct{}) []domain.DocumentAuditIssue {
	issues := []domain.DocumentAuditIssue{}
	add := func(code string, severity domain.AuditSeverity, detail string) {
		issues = append(issues, domain.DocumentAuditIssue{
			DocumentID: doc.ID, Type: doc.Type, Path: doc.Path,
			Code: code, Severity: severity, Detail: detail,
		})
	}

	if _, ok := validTypes[doc.Type]; !ok {
		add("invalid_document_type", domain.AuditError, fmt.Sprintf("unknown document type %q", doc.Type))
	}
	if !domain.SlugOK(doc.Path) {
		add("invalid_path", domain.AuditError, "path is not a valid hierarchical slug")
	}
	if doc.Type == domain.DocProject && (doc.NodeID == nil || *doc.NodeID == "") {
		add("project_missing_node", domain.AuditError, "project document has no projectId")
	}
	if doc.NodeID != nil && *doc.NodeID != "" {
		if _, ok := ownedNodes[*doc.NodeID]; !ok {
			add("node_not_owned_or_missing", domain.AuditError, "projectId does not resolve to an owner-scoped node")
		}
	}
	if doc.Type == domain.DocDaily {
		if doc.Date == nil {
			add("daily_missing_date", domain.AuditError, "daily document has no date")
		} else if want := domain.DailyPath(*doc.Date); doc.Path != want {
			add("daily_path_mismatch", domain.AuditError, fmt.Sprintf("path should be %q for the stored date", want))
		}
		if doc.NodeID != nil && *doc.NodeID != "" {
			add("daily_with_project", domain.AuditWarning, fmt.Sprintf("daily document carries projectId %q", *doc.NodeID))
		}
	} else if doc.Date != nil {
		add("date_on_non_daily", domain.AuditWarning, "non-daily document carries a date")
	}
	if doc.Type != domain.DocDaily && strings.HasPrefix(doc.Path, "daily/") {
		add("daily_path_on_non_daily", domain.AuditWarning, "daily path is assigned to a non-daily document type")
	}
	if doc.Type == domain.DocFree && doc.NodeID != nil && *doc.NodeID != "" {
		add("free_with_project", domain.AuditWarning, fmt.Sprintf("free document carries projectId %q; verify whether type should be project", *doc.NodeID))
	}
	if doc.Type == domain.DocAgent {
		add("deprecated_agent_type", domain.AuditWarning, "legacy agent type should be reclassified as spec or plan")
	}
	if doc.Archived && doc.Pinned {
		add("archived_and_pinned", domain.AuditError, "archived document is still pinned")
	}
	if doc.Archived && doc.ArchivedAt == nil {
		add("archived_missing_timestamp", domain.AuditError, "archived document has no archivedAt timestamp")
	}
	if doc.ContextMode != "" && !doc.ContextMode.Valid() {
		add("invalid_context_mode", domain.AuditError, fmt.Sprintf("unknown context mode %q", doc.ContextMode))
	}
	return issues
}
