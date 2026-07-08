package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/statuscache"
	"github.com/serverkraken/flow/internal/statusline"
	"github.com/serverkraken/flow/internal/tmuxopts"
	"github.com/spf13/cobra"
)

func worktimeStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Render the tmux status-right worktime segment (cached; never interactive)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := tmuxopts.Read()
			ro := statusRenderOpts{Palette: tmuxopts.Palette(opts), MaxStreakMin: tmuxopts.MaxStreak(opts)}
			fetch := func(ctx context.Context) (apiclient.WorktimeStatus, error) {
				c, err := clientFromStore(ctx) // NEVER triggers device flow on a plain read
				if err != nil {
					return apiclient.WorktimeStatus{}, err
				}
				return c.GetWorktimeStatus(ctx)
			}
			seg := renderStatus(cmd.Context(), time.Now(), statusCachePath(), fetch, ro)
			if seg != "" {
				fmt.Fprintln(cmd.OutOrStdout(), seg)
			}
			return nil // ALWAYS exit 0, never stderr (Spec §2)
		},
	}
}

type statusRenderOpts struct {
	Palette      statusline.StatusPalette
	MaxStreakMin int
}

// renderStatus is the pure tick: fresh cache → render; else fetch (2s, derived
// from the command's context so a signal still cancels) → renew + render; on
// fetch error → stale (dim) render, or empty when >30min old / no cache.
func renderStatus(parent context.Context, now time.Time, cachePath string, fetch func(context.Context) (apiclient.WorktimeStatus, error), ro statusRenderOpts) string {
	entry, ok := statuscache.Read(cachePath)
	if ok && entry.Fresh(now) {
		return render(entry.Status, now, ro, false)
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	if st, err := fetch(ctx); err == nil {
		_ = statuscache.Write(cachePath, statuscache.Entry{FetchedAt: now, Status: st})
		return render(st, now, ro, false)
	}
	if !ok || entry.Expired(now) {
		return "" // no usable cache → suppress the segment entirely
	}
	return render(entry.Status, now, ro, true) // stale: dim
}

func render(st apiclient.WorktimeStatus, now time.Time, ro statusRenderOpts, dim bool) string {
	pal := ro.Palette
	if dim {
		pal = pal.Dimmed()
	}
	return statusline.BuildStatusSegment(toSnapshot(st, now, pal, ro.MaxStreakMin))
}

// statusCachePath honours XDG_CACHE_HOME, else ~/.cache, landing at
// flow/worktime-status.json (Spec §2).
func statusCachePath() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "flow", "worktime-status.json")
}

// toSnapshot maps the wire DTO onto the pure renderer's Snapshot. LoggedMin is
// the COMPLETED time; the running session is extrapolated locally from
// ActiveStart (RFC3339), so banner + client stay in step without double-count.
func toSnapshot(st apiclient.WorktimeStatus, now time.Time, pal statusline.StatusPalette, maxStreak int) statusline.Snapshot {
	snap := statusline.Snapshot{
		Now: now, LoggedMin: st.LoggedMin, TargetMin: st.TargetMin, Running: st.Running,
		Streak: st.Streak, SaldoMin: st.Burndown.SaldoMin, SaldoTarget: st.Burndown.TargetMin,
		Palette: pal, MaxStreakMin: maxStreak,
	}
	if st.Running && st.ActiveStart != "" {
		if t, err := time.Parse(time.RFC3339, st.ActiveStart); err == nil {
			snap.ActiveStart = t
		}
	}
	if st.DayOff != nil {
		snap.DayOff = &statusline.DayOffInfo{Kind: domain.Kind(st.DayOff.Kind), Label: st.DayOff.Label}
	}
	for _, d := range st.Week {
		wd := statusline.WeekDay{LoggedMin: d.LoggedMin, TargetMin: d.TargetMin, IsToday: d.IsToday}
		if dd, err := time.Parse("2006-01-02", d.Date); err == nil {
			wd.Weekday = dd.Weekday()
		}
		if d.DayOffKind != nil {
			wd.DayOffKind = domain.Kind(*d.DayOffKind)
		}
		snap.Week = append(snap.Week, wd)
	}
	return snap
}
