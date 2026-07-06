package webui

import (
	"strings"
	"testing"
)

func TestReadingTime(t *testing.T) {
	if got := ReadingTime(strings.Repeat("wort ", 440)); got != 2 {
		t.Fatalf("ReadingTime = %d, want 2", got)
	}
	if ReadingTime("kurz") < 1 {
		t.Fatal("ReadingTime must be >= 1")
	}
}

func TestReadingTime_StripsFrontmatter(t *testing.T) {
	body := "---\ntags: [go]\n---\n" + strings.Repeat("wort ", 220)
	if got := ReadingTime(body); got != 1 {
		t.Fatalf("ReadingTime with frontmatter = %d, want 1 (frontmatter must not inflate word count)", got)
	}
}

func TestReadingTime_Empty(t *testing.T) {
	if got := ReadingTime(""); got != 1 {
		t.Fatalf("ReadingTime(\"\") = %d, want minimum 1", got)
	}
}
