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

// LoadAllLegacy returns every session in the log, oldest first. A missing file
// returns (nil, nil) — first launch is normal, not a failure.
//
// This is the legacy 0-arg form. New code should call Load(userID) instead.
// Deleted in Task 19 when the tsvsessions adapter is removed entirely.
func (s *Store) LoadAllLegacy() ([]domain.Session, error) {
	return s.read(nil)
}

// LoadFilteredLegacy returns sessions for which keep returns true.
//
// This is the legacy 1-arg form. New code should call LoadFiltered(userID, keep)
// instead. Deleted in Task 19 when the tsvsessions adapter is removed entirely.
func (s *Store) LoadFilteredLegacy(keep func(domain.Session) bool) ([]domain.Session, error) {
	return s.read(keep)
}

// Load implements the new ports.SessionStore (Task 3 of M2-M3 Plan B).
// This shim translates Upsert/Delete back to the legacy LoadAllLegacy/Rewrite
// semantics. Adapter is deleted entirely in Task 19 of Plan B.
func (s *Store) Load(_ string) ([]domain.Session, error) {
	return s.LoadAllLegacy()
}

// LoadFiltered implements the new ports.SessionStore (Task 3 of M2-M3 Plan B).
func (s *Store) LoadFiltered(_ string, keep func(domain.Session) bool) ([]domain.Session, error) {
	all, err := s.LoadAllLegacy()
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, x := range all {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out, nil
}

// Upsert implements the new ports.SessionStore (Task 3 of M2-M3 Plan B).
// Matches on (Date, Start) since legacy rows have no ID.
func (s *Store) Upsert(in domain.Session) error {
	cur, err := s.LoadAllLegacy()
	if err != nil {
		return err
	}
	idx := -1
	for i := range cur {
		if cur[i].Date.Equal(in.Date) && cur[i].Start.Equal(in.Start) {
			idx = i
			break
		}
	}
	if idx >= 0 {
		cur[idx] = in
	} else {
		cur = append(cur, in)
	}
	return s.Rewrite(cur)
}

// UpsertBatch implements the new ports.SessionStore (Task 3 of M2-M3 Plan B).
func (s *Store) UpsertBatch(in []domain.Session) error {
	for _, ss := range in {
		if err := s.Upsert(ss); err != nil {
			return err
		}
	}
	return nil
}

// Delete implements the new ports.SessionStore (Task 3 of M2-M3 Plan B).
// Legacy adapter cannot delete by ID — silently no-op. The TUI's
// Delete path goes through the use case which loads → filters → Rewrite.
func (s *Store) Delete(_, _ string) error {
	return nil
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

// AppendBatch writes several session rows in a single O_APPEND call so a
// partial failure on retry cannot duplicate the rows that were already
// flushed before the crash. The whole batch is assembled in memory and
// emitted with one call to atomicfile.Append. An empty batch is a no-op.
//
// Why: review finding B1 — `stopAt` / `Pause` / `Toggle` previously
// looped Append for each midnight-split part. A failure on part N>0 left
// parts 1..N-1 persisted, and the natural retry path produced
// duplicates because SplitAtMidnight is deterministic.
func (s *Store) AppendBatch(sessions []domain.Session) error {
	if len(sessions) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	for _, sess := range sessions {
		if _, err := writeSessionLine(&buf, sess); err != nil {
			return err
		}
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
