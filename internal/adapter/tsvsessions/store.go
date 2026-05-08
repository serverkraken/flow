package tsvsessions

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/adapter/atomicfile"
	"github.com/serverkraken/flow/internal/adapter/textscan"
	"github.com/serverkraken/flow/internal/domain"
)

// Store reads and writes sessions to a TSV file. The zero value is unusable;
// construct via New.
type Store struct {
	path string
}

// New returns a Store backed by path. The file is created on first append;
// a missing file is treated as an empty log on read.
func New(path string) *Store {
	return &Store{path: path}
}

// LoadAll returns every session in the log, oldest first. A missing file
// returns (nil, nil) — first launch is normal, not a failure.
func (s *Store) LoadAll() ([]domain.Session, error) {
	return s.read(nil)
}

// LoadFiltered returns sessions for which keep returns true.
func (s *Store) LoadFiltered(keep func(domain.Session) bool) ([]domain.Session, error) {
	return s.read(keep)
}

// Append writes a single session row. The parent directory is created on
// demand so callers can configure the path before the .tmux dir exists.
// The row is assembled in memory and emitted via a single Write call
// (POSIX guarantees atomic O_APPEND for writes <= PIPE_BUF — session
// rows are ~50 bytes); see atomicfile.Append for the fsync discipline.
func (s *Store) Append(sess domain.Session) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if _, err := writeSessionLine(&buf, sess); err != nil {
		return err
	}
	return atomicfile.Append(s.path, buf.Bytes(), 0o644)
}

// Rewrite replaces the entire log atomically (see atomicfile.WriteFile).
func (s *Store) Rewrite(sessions []domain.Session) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	for _, sess := range sessions {
		if _, werr := writeSessionLine(&buf, sess); werr != nil {
			return werr
		}
	}
	return atomicfile.WriteFile(s.path, buf.Bytes(), 0o644)
}

func (s *Store) read(keep func(domain.Session) bool) ([]domain.Session, error) {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var sessions []domain.Session
	sc := textscan.New(f)
	for sc.Scan() {
		sess, ok := parseLine(sc.Text())
		if !ok {
			continue
		}
		if keep == nil || keep(sess) {
			sessions = append(sessions, sess)
		}
	}
	return sessions, sc.Err()
}

// parseLine parses one TSV row. Blank lines, comments (#), and malformed
// rows are skipped silently — invalid rows in older logs must not break
// newer readers.
//
// Tag and note are split with a column cap of 6 (SplitN .., "\t", 6) so
// any tabs inside the note are kept as-is in the note value rather than
// truncating the row. Tabs in tag are forbidden by the writer (replaced
// with spaces). If a hand-edited row has 7+ tabs, parts[4] becomes the
// tag and parts[5] absorbs the rest of the line including any embedded
// tabs — the older comment claimed tag absorbed up to the 5th tab,
// which is the opposite of the actual SplitN cap.
func parseLine(raw string) (domain.Session, bool) {
	line := strings.TrimRight(raw, "\r\n")
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return domain.Session{}, false
	}
	parts := strings.SplitN(line, "\t", 6)
	if len(parts) < 4 {
		return domain.Session{}, false
	}
	date, err := time.ParseInLocation("2006-01-02", parts[0], time.Local)
	if err != nil {
		return domain.Session{}, false
	}
	startHM, err := domain.ParseHM(parts[1])
	if err != nil {
		return domain.Session{}, false
	}
	stopHM, err := domain.ParseHM(parts[2])
	if err != nil {
		return domain.Session{}, false
	}
	elapsedSec, err := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
	if err != nil {
		return domain.Session{}, false
	}
	base := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.Local)
	s := domain.Session{
		Date:    base,
		Start:   base.Add(startHM),
		Stop:    base.Add(stopHM),
		Elapsed: time.Duration(elapsedSec) * time.Second,
	}
	if len(parts) >= 5 {
		s.Tag = strings.TrimSpace(parts[4])
	}
	if len(parts) >= 6 {
		s.Note = strings.TrimSpace(parts[5])
	}
	return s, true
}

// sanitizeTSVField replaces tab/CR/LF in user-provided strings with a
// single space so they round-trip through the TSV row format. Without
// this, a note containing a literal tab corrupts the column layout and
// the parser silently truncates everything after the embedded tab.
func sanitizeTSVField(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\t', '\r', '\n':
			return ' '
		}
		return r
	}, s)
}

// writeSessionLine writes one TSV row, omitting trailing tag/note columns
// when empty so the file stays compact and 4-column historical readers
// still parse newly written rows.
func writeSessionLine(w io.Writer, s domain.Session) (int, error) {
	base := fmt.Sprintf(
		"%s\t%s\t%s\t%d",
		s.Date.Format("2006-01-02"),
		s.Start.Format("15:04"),
		s.Stop.Format("15:04"),
		int64(s.Elapsed.Seconds()),
	)
	tag := sanitizeTSVField(s.Tag)
	note := sanitizeTSVField(s.Note)
	if tag != "" || note != "" {
		base += "\t" + tag
	}
	if note != "" {
		base += "\t" + note
	}
	return fmt.Fprintln(w, base)
}
