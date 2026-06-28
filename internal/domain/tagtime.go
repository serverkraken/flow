package domain

// TagTime is the total tracked minutes carrying a given tag.
type TagTime struct {
	Tag     string `json:"tag"`
	Minutes int    `json:"minutes"`
}
