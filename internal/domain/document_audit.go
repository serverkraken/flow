package domain

// AuditSeverity distinguishes broken invariants from suspicious but currently
// permitted document states.
type AuditSeverity string

const (
	AuditError   AuditSeverity = "error"
	AuditWarning AuditSeverity = "warning"
)

type DocumentAuditIssue struct {
	DocumentID string        `json:"documentId"`
	Type       DocumentType  `json:"type"`
	Path       string        `json:"path"`
	Code       string        `json:"code"`
	Severity   AuditSeverity `json:"severity"`
	Detail     string        `json:"detail"`
}

// DocumentAuditReport is a read-only inventory of document metadata issues.
type DocumentAuditReport struct {
	Scanned  int                  `json:"scanned"`
	Active   int                  `json:"active"`
	Archived int                  `json:"archived"`
	Issues   []DocumentAuditIssue `json:"issues"`
}
