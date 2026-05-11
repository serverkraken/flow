package domain_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestNoteType_IsValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		nt   domain.NoteType
		want bool
	}{
		{domain.TypeDaily, true},
		{domain.TypeProject, true},
		{domain.TypeFree, true},
		{domain.NoteType(""), false},
		{domain.NoteType("unknown"), false},
	}
	for _, tc := range cases {
		t.Run(string(tc.nt), func(t *testing.T) {
			t.Parallel()
			if got := tc.nt.IsValid(); got != tc.want {
				t.Errorf("%q.IsValid() = %v, want %v", tc.nt, got, tc.want)
			}
		})
	}
}

func TestFrontmatter_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		fm      domain.Frontmatter
		wantErr bool
	}{
		{"valid daily", domain.Frontmatter{ID: "daily/2026-04-25", Type: domain.TypeDaily}, false},
		{"valid project", domain.Frontmatter{ID: "projects/foo/bar/2026-04-25", Type: domain.TypeProject, Project: "github.com/foo/bar"}, false},
		{"valid free", domain.Frontmatter{ID: "notes/setup", Type: domain.TypeFree}, false},

		{"empty id", domain.Frontmatter{Type: domain.TypeDaily}, true},
		{"invalid type empty", domain.Frontmatter{ID: "x"}, true},
		{"invalid type unknown", domain.Frontmatter{ID: "x", Type: "garbage"}, true},
		{"project without project field", domain.Frontmatter{ID: "x", Type: domain.TypeProject}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.fm.Validate()
			if tc.wantErr {
				if !errors.Is(err, domain.ErrInvalidFrontmatter) {
					t.Fatalf("expected ErrInvalidFrontmatter, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestHasFrontmatter(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content []byte
		want    bool
	}{
		{"with delim", []byte("---\nid: x\n---\nbody\n"), true},
		{"plain markdown", []byte("# Heading\n"), false},
		{"only opening dashes no newline", []byte("---"), false},
		{"empty", nil, false},
		{"with utf-8 BOM", append([]byte{0xEF, 0xBB, 0xBF}, []byte("---\nid: x\n---\nbody\n")...), true},
		{"with CRLF", []byte("---\r\nid: x\r\n---\r\nbody\r\n"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.HasFrontmatter(tc.content); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseFrontmatter(t *testing.T) {
	t.Parallel()

	t.Run("standard", func(t *testing.T) {
		t.Parallel()
		content := []byte("---\nid: daily/2026-04-25\ntype: daily\n---\nbody line\n")
		fm, body, err := domain.ParseFrontmatter(content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fm.ID != "daily/2026-04-25" || fm.Type != domain.TypeDaily {
			t.Errorf("got fm=%+v", fm)
		}
		if string(body) != "body line\n" {
			t.Errorf("got body=%q", body)
		}
	})

	t.Run("no trailing newline after closing delim", func(t *testing.T) {
		t.Parallel()
		content := []byte("---\nid: daily/2026-04-25\ntype: daily\n---")
		fm, body, err := domain.ParseFrontmatter(content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fm.ID != "daily/2026-04-25" {
			t.Errorf("got fm=%+v", fm)
		}
		if len(body) != 0 {
			t.Errorf("expected empty body, got %q", body)
		}
	})

	t.Run("empty frontmatter with body", func(t *testing.T) {
		t.Parallel()
		content := []byte("---\n---\nbody\n")
		fm, body, err := domain.ParseFrontmatter(content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(fm, domain.Frontmatter{}) {
			t.Errorf("expected zero fm, got %+v", fm)
		}
		if string(body) != "body\n" {
			t.Errorf("got body=%q", body)
		}
	})

	t.Run("empty frontmatter no body", func(t *testing.T) {
		t.Parallel()
		content := []byte("---\n---")
		fm, body, err := domain.ParseFrontmatter(content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(fm, domain.Frontmatter{}) {
			t.Errorf("expected zero fm, got %+v", fm)
		}
		if len(body) != 0 {
			t.Errorf("expected empty body, got %q", body)
		}
	})

	t.Run("missing frontmatter", func(t *testing.T) {
		t.Parallel()
		_, _, err := domain.ParseFrontmatter([]byte("# Heading\nbody\n"))
		if !errors.Is(err, domain.ErrNoFrontmatter) {
			t.Errorf("expected ErrNoFrontmatter, got %v", err)
		}
	})

	t.Run("unterminated frontmatter", func(t *testing.T) {
		t.Parallel()
		_, _, err := domain.ParseFrontmatter([]byte("---\nid: x\nno closer\n"))
		if !errors.Is(err, domain.ErrMalformedFrontmatter) {
			t.Errorf("expected ErrMalformedFrontmatter, got %v", err)
		}
	})

	t.Run("malformed yaml", func(t *testing.T) {
		t.Parallel()
		_, _, err := domain.ParseFrontmatter([]byte("---\nid: x\n  bad indent\n  : nope\n---\n"))
		if !errors.Is(err, domain.ErrMalformedFrontmatter) {
			t.Errorf("expected ErrMalformedFrontmatter, got %v", err)
		}
	})

	t.Run("utf-8 BOM tolerated", func(t *testing.T) {
		t.Parallel()
		content := append([]byte{0xEF, 0xBB, 0xBF},
			[]byte("---\nid: daily/2026-04-25\ntype: daily\n---\nbody\n")...)
		fm, body, err := domain.ParseFrontmatter(content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fm.ID != "daily/2026-04-25" || string(body) != "body\n" {
			t.Errorf("got fm=%+v body=%q", fm, body)
		}
	})

	t.Run("CRLF line endings tolerated", func(t *testing.T) {
		t.Parallel()
		content := []byte("---\r\nid: daily/2026-04-25\r\ntype: daily\r\n---\r\nbody line\r\n")
		fm, body, err := domain.ParseFrontmatter(content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fm.ID != "daily/2026-04-25" {
			t.Errorf("got fm=%+v", fm)
		}
		// Body should be normalised to LF too.
		if string(body) != "body line\n" {
			t.Errorf("got body=%q (expected LF-normalised)", body)
		}
	})
}

func TestFrontmatter_Serialize(t *testing.T) {
	t.Parallel()

	fm := domain.Frontmatter{
		ID:    "daily/2026-04-25",
		Type:  domain.TypeDaily,
		Title: "kompendium aufsetzen",
		Tags:  []string{"tmux", "plugin"},
	}
	body := []byte("# kompendium\nbody content\n")

	out := fm.Serialize(body)

	if !bytes.HasPrefix(out, []byte("---\n")) {
		t.Errorf("output must start with frontmatter delim, got %q", out)
	}
	if !bytes.Contains(out, body) {
		t.Errorf("output must contain body, got %q", out)
	}

	parsed, parsedBody, err := domain.ParseFrontmatter(out)
	if err != nil {
		t.Fatalf("roundtrip ParseFrontmatter: %v", err)
	}
	if parsed.ID != fm.ID || parsed.Type != fm.Type || parsed.Title != fm.Title {
		t.Errorf("roundtrip mismatch: got %+v want %+v", parsed, fm)
	}
	if string(parsedBody) != string(body) {
		t.Errorf("roundtrip body mismatch: got %q want %q", parsedBody, body)
	}
}

// TestFrontmatter_PreservesUnknownKeys verifies that Get→Put round-trip
// (which is what CaptureDaily does on every capture) keeps user-added
// frontmatter keys like `mood:` or `weather:` rather than silently
// dropping them through the closed Frontmatter struct.
func TestFrontmatter_PreservesUnknownKeys(t *testing.T) {
	t.Parallel()

	content := []byte("---\nid: daily/2026-04-25\ntype: daily\nmood: focused\nweather: sunny\n---\nbody\n")
	fm, body, err := domain.ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if fm.Extra["mood"] != "focused" || fm.Extra["weather"] != "sunny" {
		t.Errorf("Extra missing user keys: got %+v", fm.Extra)
	}

	out := fm.Serialize(body)
	if !bytes.Contains(out, []byte("mood: focused")) {
		t.Errorf("serialized output dropped mood key: %s", out)
	}
	if !bytes.Contains(out, []byte("weather: sunny")) {
		t.Errorf("serialized output dropped weather key: %s", out)
	}
}

// TestParseFrontmatter_RejectsOversizedYAML guards against
// billion-laughs / alias-amplification attacks against yaml.v3. The
// parser must reject frontmatter blocks beyond the 64 KiB cap before
// yaml.Unmarshal sees them — otherwise a few KiB of nested aliases can
// blow up into gigabytes during expansion.
func TestParseFrontmatter_RejectsOversizedYAML(t *testing.T) {
	t.Parallel()

	// Padding alone (one big tag value) is the cheap proof. The amount
	// merely needs to exceed the 64 KiB cap; the bomb threat itself
	// isn't simulated — it's neutralised by refusing oversized input.
	big := strings.Repeat("x", 70<<10)
	content := []byte("---\nid: x\ntype: free\ntitle: " + big + "\n---\nbody\n")

	_, _, err := domain.ParseFrontmatter(content)
	if err == nil {
		t.Fatal("expected size-cap error on oversized frontmatter, got nil")
	}
	if !errors.Is(err, domain.ErrMalformedFrontmatter) {
		t.Errorf("error should wrap ErrMalformedFrontmatter, got %v", err)
	}
}
