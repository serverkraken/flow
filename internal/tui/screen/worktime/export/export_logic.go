package export

import (
	"os"
	"path/filepath"
	"time"
)

const dayFmt = "2006-01-02"

func presetRange(preset string, now time.Time) (string, string) {
	y, mo, d := now.Date()
	loc := now.Location()
	today := time.Date(y, mo, d, 0, 0, 0, 0, loc)
	switch preset {
	case "monat":
		from := time.Date(y, mo, 1, 0, 0, 0, 0, loc)
		return from.Format(dayFmt), today.Format(dayFmt)
	case "kw":
		off := (int(today.Weekday()) + 6) % 7
		return today.AddDate(0, 0, -off).Format(dayFmt), today.Format(dayFmt)
	case "letzter":
		firstThis := time.Date(y, mo, 1, 0, 0, 0, 0, loc)
		lastPrev := firstThis.AddDate(0, 0, -1)
		firstPrev := time.Date(lastPrev.Year(), lastPrev.Month(), 1, 0, 0, 0, 0, loc)
		return firstPrev.Format(dayFmt), lastPrev.Format(dayFmt)
	default:
		return today.Format(dayFmt), today.Format(dayFmt)
	}
}

func defaultPath(from, to, format string) string {
	return "~/Downloads/flow-export-" + from + "_" + to + "." + format
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if len(path) >= 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func cycleFormat(f string, dir int) string { return cycle([]string{"csv", "json", "md"}, f, dir) }
func cyclePreset(p string, dir int) string {
	return cycle([]string{"kw", "monat", "letzter", "custom"}, p, dir)
}

func cycle(order []string, cur string, dir int) string {
	idx := 0
	for i, v := range order {
		if v == cur {
			idx = i
			break
		}
	}
	return order[(idx+dir+len(order))%len(order)]
}
