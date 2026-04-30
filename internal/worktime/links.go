package worktime

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Link is one persisted attachment from a worktime day to a Kompendium note ID.
type Link struct {
	Date   time.Time
	NoteID string
}

// linksPath returns the path to the TSV file storing link rows.
func linksPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".tmux", "worktime-links.tsv"), nil
}

// ListLinks returns the note IDs attached to the given date, in insertion order.
func ListLinks(date time.Time) ([]string, error) {
	all, err := readAllLinks()
	if err != nil {
		return nil, err
	}
	key := date.Format("2006-01-02")
	var ids []string
	for _, l := range all {
		if l.Date.Format("2006-01-02") == key {
			ids = append(ids, l.NoteID)
		}
	}
	return ids, nil
}

// AddLink attaches a note ID to a date. Idempotent: adding the same pair twice is a no-op.
func AddLink(date time.Time, noteID string) error {
	noteID = strings.TrimSpace(noteID)
	if noteID == "" {
		return errors.New("note id darf nicht leer sein")
	}
	if strings.ContainsAny(noteID, "\t\n\r") {
		return errors.New("note id enthält ungültige zeichen")
	}

	existing, err := ListLinks(date)
	if err != nil {
		return err
	}
	for _, id := range existing {
		if id == noteID {
			return nil
		}
	}

	path, err := linksPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	_, err = fmt.Fprintf(f, "%s\t%s\n", date.Format("2006-01-02"), noteID)
	return err
}

// RemoveLink detaches a note ID from a date. Removing a non-existent pair is a no-op.
func RemoveLink(date time.Time, noteID string) error {
	all, err := readAllLinks()
	if err != nil {
		return err
	}
	key := date.Format("2006-01-02")
	kept := make([]Link, 0, len(all))
	removed := false
	for _, l := range all {
		if !removed && l.Date.Format("2006-01-02") == key && l.NoteID == noteID {
			removed = true
			continue
		}
		kept = append(kept, l)
	}
	if !removed {
		return nil
	}
	return writeAllLinks(kept)
}

func readAllLinks() ([]Link, error) {
	path, err := linksPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var links []Link
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		date, err := time.ParseInLocation("2006-01-02", parts[0], time.Local)
		if err != nil {
			continue
		}
		noteID := strings.TrimSpace(parts[1])
		if noteID == "" {
			continue
		}
		links = append(links, Link{Date: date, NoteID: noteID})
	}
	return links, sc.Err()
}

func writeAllLinks(links []Link) error {
	path, err := linksPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	for _, l := range links {
		if _, err := fmt.Fprintf(f, "%s\t%s\n", l.Date.Format("2006-01-02"), l.NoteID); err != nil {
			f.Close() //nolint:errcheck
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
