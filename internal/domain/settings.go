package domain

import (
	"strings"
	"time"
)

// Settings holds per-user preferences. Bundesland drives the computed
// German-holiday set; DefaultTargetMin + WeekdayTargetMin drive the daily
// work target (M1d). WeekdayTargetMin keys are time.Weekday; a present
// entry (incl. 0) is an explicit override, absence means "use default".
type Settings struct {
	UserID           string               `json:"-"`
	Bundesland       string               `json:"bundesland"`
	DefaultTargetMin int                  `json:"defaultTargetMin"`
	WeekdayTargetMin map[time.Weekday]int `json:"-"`
}

// DefaultDailyTargetMin is the fallback daily target (8h) when a user never
// configured one.
const DefaultDailyTargetMin = 480

// FeedToken is a secret used to subscribe to a per-user calendar feed
// without interactive auth. Revoked tokens stop resolving.
type FeedToken struct {
	Token     string    `json:"token"`
	UserID    string    `json:"-"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"createdAt"`
}

// validLandCodes is the 16 Bundesländer plus "DE" (bundesweit only).
var validLandCodes = map[string]bool{
	"BW": true, "BY": true, "BE": true, "BB": true, "HB": true, "HH": true,
	"HE": true, "MV": true, "NI": true, "NW": true, "RP": true, "SL": true,
	"SN": true, "ST": true, "SH": true, "TH": true, "DE": true,
}

// ValidBundesland normalizes common aliases (NRW→NW, case) and reports
// whether the result is one of the 16 codes (or "DE"). Mirrors the aliasing
// that GermanHolidays applies internally so the API rejects garbage early.
func ValidBundesland(s string) (string, bool) {
	n := strings.ToUpper(strings.TrimSpace(s))
	switch n {
	case "NRW":
		n = "NW"
	case "BAYERN":
		n = "BY"
	case "BADEN-WÜRTTEMBERG", "BADEN-WUERTTEMBERG", "BAWÜ", "BAWUE":
		n = "BW"
	}
	return n, validLandCodes[n]
}
