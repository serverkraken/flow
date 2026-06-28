package domain

// StripReport summarises the result of a StripFrontmatter maintenance run.
type StripReport struct {
	Scanned  int `json:"scanned"`
	Stripped int `json:"stripped"`
}
