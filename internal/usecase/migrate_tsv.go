package usecase

import (
	"crypto/sha1" //nolint:gosec // SHA-1 is used for UUID v5 namespace derivation, not security
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// MigrateTSV reads the legacy ~/.tmux/worktime.log, maps every row to a
// Session in the new SQLite store, and renames the TSV to .migrated-<ts>.
//
// Idempotency: each row's UUID is UUIDv5(namespace, date|start|tag|note).
// Re-running the migration against the same database produces the same UUIDs
// → Upsert with a matching ID is a no-op (update with identical fields).
type MigrateTSV struct {
	users    ports.UserStore
	projects ports.ProjectStore
	sessions ports.SessionStore
}

// NewMigrateTSV constructs a MigrateTSV use case.
func NewMigrateTSV(u ports.UserStore, p ports.ProjectStore, s ports.SessionStore) *MigrateTSV {
	return &MigrateTSV{users: u, projects: p, sessions: s}
}

// MigrateResult carries the outcome of a successful migration run.
type MigrateResult struct {
	// Inserted is the count of rows written (including idempotent re-runs).
	Inserted int
	// SkippedMalformed counts lines that could not be parsed.
	SkippedMalformed int
	// DefaultProject is the project all migrated sessions were assigned to.
	DefaultProject domain.Project
	// ArchivedTo is the path the TSV was renamed to after migration.
	// Empty when the file was already absent (graceful no-op).
	ArchivedTo string
}

// migrationNamespace is the fixed UUID v5 namespace for TSV row IDs.
// Value: a9c8b5d2-7e3f-4d1e-9c0a-1234567890ab as a 16-byte array.
var migrationNamespace = [16]byte{
	0xa9, 0xc8, 0xb5, 0xd2,
	0x7e, 0x3f, 0x4d, 0x1e,
	0x9c, 0x0a,
	0x12, 0x34, 0x56, 0x78, 0x90, 0xab,
}

// Run executes the migration. userID must resolve to an existing User.
// defaultProjectName is the project name (and auto-slug) all sessions are
// assigned to — created via EnsureBySlug when absent.
//
// If tsvPath does not exist the function returns a zero MigrateResult without
// error (graceful no-op for machines that never had the legacy log).
func (m *MigrateTSV) Run(userID, tsvPath, defaultProjectName string) (MigrateResult, error) {
	// Validate the user exists.
	if _, err := m.users.GetByID(userID); err != nil {
		return MigrateResult{}, fmt.Errorf("migrate-tsv: user not found: %w", err)
	}

	// Graceful no-op when the file is absent.
	if _, err := os.Stat(tsvPath); errors.Is(err, os.ErrNotExist) {
		return MigrateResult{}, nil
	}

	// Ensure the default project exists.
	slug := SlugFromName(defaultProjectName)
	proj, err := m.projects.EnsureBySlug(userID, defaultProjectName, slug)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("migrate-tsv: ensure project: %w", err)
	}

	// Read and parse the TSV.
	raw, err := os.ReadFile(tsvPath)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("migrate-tsv: read tsv: %w", err)
	}

	lines := strings.Split(string(raw), "\n")
	var result MigrateResult
	result.DefaultProject = proj

	for _, line := range lines {
		line = strings.TrimRight(line, "\r\n")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		sess, ok := parseMigrationLine(line)
		if !ok {
			result.SkippedMalformed++
			continue
		}

		// Assign deterministic UUID v5 and user/project context.
		sess.ID = uuidV5(migrationNamespace, migrationKey(sess))
		sess.UserID = userID
		sess.ProjectID = proj.ID
		sess.UpdatedAt = time.Now().UTC()

		if err := m.sessions.Upsert(sess); err != nil {
			return MigrateResult{}, fmt.Errorf("migrate-tsv: upsert session: %w", err)
		}
		result.Inserted++
	}

	// Rename the TSV to prevent the legacy adapter from reloading it.
	ts := time.Now().UTC().Format("20060102T150405Z")
	archivePath := tsvPath + ".migrated-" + ts
	if err := os.Rename(tsvPath, archivePath); err != nil {
		return MigrateResult{}, fmt.Errorf("migrate-tsv: archive tsv: %w", err)
	}
	result.ArchivedTo = archivePath

	return result, nil
}

// parseMigrationLine parses one TSV row from the legacy worktime.log format.
// Format: date\tstart\tstop\telapsed[\ttag[\tnote]]
// date:    YYYY-MM-DD
// start:   HH:MM (local time)
// stop:    HH:MM (local time)
// elapsed: integer seconds
//
// Returns (session, true) on success; (zero, false) for blank, comment, or
// malformed lines. Mirrors the parse logic in tsvsessions.parseLine.
func parseMigrationLine(raw string) (domain.Session, bool) {
	parts := strings.SplitN(raw, "\t", 6)
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

// migrationKey returns a stable string key for a TSV row that identifies
// it uniquely within the migration namespace. Uses Date, Start, Tag, and
// Note — same fields the plan specifies for UUIDv5 determinism.
func migrationKey(s domain.Session) string {
	return fmt.Sprintf(
		"%s|%s|%s|%s",
		s.Date.Format("2006-01-02"),
		s.Start.Format("15:04"),
		s.Tag,
		s.Note,
	)
}

// uuidV5 computes a UUID version 5 (SHA-1 name-based) from namespace and name.
// Algorithm per RFC 4122 §4.3:
//  1. SHA1( namespace_bytes || name_bytes )
//  2. Take the first 16 bytes.
//  3. Set version nibble (byte[6] high nibble) to 5.
//  4. Set variant bits (byte[8] high 2 bits) to 10.
//
// Returns the canonical 8-4-4-4-12 hex string.
func uuidV5(namespace [16]byte, name string) string {
	h := sha1.New() //nolint:gosec // SHA-1 is mandated by RFC 4122 §4.3 for UUID v5
	_, _ = h.Write(namespace[:])
	_, _ = h.Write([]byte(name))
	sum := h.Sum(nil) // 20 bytes

	var b [16]byte
	copy(b[:], sum[:16])
	b[6] = (b[6] & 0x0f) | 0x50 // version 5
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10xx

	hex16 := hex.EncodeToString(b[:])
	return hex16[0:8] + "-" + hex16[8:12] + "-" + hex16[12:16] + "-" + hex16[16:20] + "-" + hex16[20:]
}
