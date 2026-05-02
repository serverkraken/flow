package tsvsessions

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
func (s *Store) Append(sess domain.Session) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	_, err = writeSessionLine(f, sess)
	return err
}

// Rewrite replaces the entire log atomically by writing to a sibling
// temp-file and renaming over the target.
func (s *Store) Rewrite(sessions []domain.Session) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	for _, sess := range sessions {
		if _, werr := writeSessionLine(f, sess); werr != nil {
			f.Close() //nolint:errcheck
			return werr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
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
	sc := bufio.NewScanner(f)
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
func parseLine(raw string) (domain.Session, bool) {
	line := strings.TrimRight(raw, "\r\n")
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return domain.Session{}, false
	}
	parts := strings.Split(line, "\t")
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

// writeSessionLine writes one TSV row, omitting trailing tag/note columns
// when empty so the file stays compact and 4-column historical readers
// still parse newly written rows.
func writeSessionLine(w io.Writer, s domain.Session) (int, error) {
	base := fmt.Sprintf("%s\t%s\t%s\t%d",
		s.Date.Format("2006-01-02"),
		s.Start.Format("15:04"),
		s.Stop.Format("15:04"),
		int64(s.Elapsed.Seconds()),
	)
	if s.Tag != "" || s.Note != "" {
		base += "\t" + s.Tag
	}
	if s.Note != "" {
		base += "\t" + s.Note
	}
	return fmt.Fprintln(w, base)
}
