package websession

import (
	"testing"
	"time"
)

func TestIssueParseRoundTrip(t *testing.T) {
	c := NewCodec("0123456789abcdef0123456789abcdef", time.Hour)
	tok, err := c.Issue("user-42")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := c.Parse(tok)
	if err != nil || got != "user-42" {
		t.Fatalf("parse = %q err=%v", got, err)
	}
}

func TestParseRejectsForeignSecret(t *testing.T) {
	a := NewCodec("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", time.Hour)
	b := NewCodec("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", time.Hour)
	tok, _ := a.Issue("u")
	if _, err := b.Parse(tok); err == nil {
		t.Fatal("a token signed by A must not verify under B")
	}
}
