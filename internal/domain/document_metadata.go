package domain

import "time"

// DocumentMetadata is the complete mutable location/classification of a
// document. Callers submit the whole value so a reclassification cannot leave
// stale date or node metadata behind.
type DocumentMetadata struct {
	Type   DocumentType
	NodeID *string
	Path   string
	Date   *time.Time
}

// ReclassifyDocumentMetadata applies canonical metadata rules as one pure
// operation. Daily notes always derive their path from Date; daily and free
// notes are global; non-daily notes never retain a stale date.
func ReclassifyDocumentMetadata(d Document, m DocumentMetadata) (Document, error) {
	d.Type = m.Type
	d.NodeID = m.NodeID
	d.Path = m.Path
	d.Date = nil
	switch m.Type {
	case DocDaily:
		d.Date = m.Date
		d.NodeID = nil
		if m.Date != nil {
			d.Path = DailyPath(*m.Date)
		}
	case DocFree:
		d.NodeID = nil
	}
	if err := d.Validate(); err != nil {
		return Document{}, err
	}
	return d, nil
}
