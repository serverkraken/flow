package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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
			seg := renderStatus(time.Now(), statusCachePath(), func() {
				_ = spawnWorktimeStatusRefresh()
			}, ro)
			if seg != "" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), seg)
			}
			return nil // ALWAYS exit 0, never stderr (Spec §2)
		},
	}
}

var spawnWorktimeStatusRefresh = startWorktimeStatusRefresh

// startWorktimeStatusRefresh starts a detached cache worker. It deliberately
// does not inherit the render command's context: the renderer exits as soon as
// the child is started, while the worker owns its bounded refresh lifecycle.
func startWorktimeStatusRefresh() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(executable, "worktime", "status-refresh")
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

type statusRenderOpts struct {
	Palette      statusline.StatusPalette
	MaxStreakMin int
}

// renderStatus is the cache-only tick. Stale or absent data schedules a
// detached refresh and returns immediately; it never performs I/O beyond the
// local cache read and process spawn.
func renderStatus(now time.Time, cachePath string, triggerRefresh func(), ro statusRenderOpts) string {
	entry, ok := statuscache.Read(cachePath)
	if ok && entry.Fresh(now) {
		return render(entry.Status, now, ro, false)
	}
	if triggerRefresh != nil {
		triggerRefresh()
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
