package kompendiumexec

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// Runner runs name args... and returns combined stdout. exec.ExitError
// is preserved so callers can inspect Stderr for parse-friendly errors.
type Runner func(name string, args ...string) ([]byte, error)

// StatFunc lets DailyExists fake the filesystem in unit tests.
type StatFunc func(path string) (os.FileInfo, error)

// Gateway implements ports.KompendiumGateway.
type Gateway struct {
	bin    string
	run    Runner
	statFn StatFunc
}

// New constructs a Gateway that shells out to bin. bin is typically
// resolved from $KOMPENDIUM_BIN with a "kompendium" default.
func New(bin string) *Gateway {
	return &Gateway{
		bin:    bin,
		run:    defaultRunner,
		statFn: os.Stat,
	}
}

// NewWithRunner is for tests. Passing a stat function lets tests
// simulate file presence without writing real files.
func NewWithRunner(bin string, r Runner, statFn StatFunc) *Gateway {
	if statFn == nil {
		statFn = os.Stat
	}
	return &Gateway{bin: bin, run: r, statFn: statFn}
}

func defaultRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// DailyExists reports whether the daily note for date exists. Best
// effort: a non-zero exit from `kompendium path` or a missing file are
// both reported as "not found".
func (g *Gateway) DailyExists(date time.Time) bool {
	path, err := g.ResolvePath(domain.DailyNoteID(date))
	if err != nil || path == "" {
		return false
	}
	_, err = g.statFn(path)
	return err == nil
}

// List returns every known note via `kompendium ls --json`. Empty
// output is normalised to nil; a non-zero exit surfaces an error
// containing the binary's stderr (truncated to one line).
func (g *Gateway) List() ([]domain.KompendiumNote, error) {
	out, err := g.run(g.bin, "ls", "--json")
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("kompendium ls: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("kompendium ls: %w", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return nil, nil
	}
	var notes []domain.KompendiumNote
	if err := json.Unmarshal(out, &notes); err != nil {
		return nil, fmt.Errorf("kompendium ls: parse: %w", err)
	}
	return notes, nil
}

// ResolvePath returns the filesystem path for a note ID, or "" when the
// ID can't be resolved.
func (g *Gateway) ResolvePath(id string) (string, error) {
	out, err := g.run(g.bin, "path", id)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
