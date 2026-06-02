package httpserver

import (
	"encoding/hex"
	"testing"
)

type sessVal struct {
	Sub   string
	Email string
}

func TestUnit_Session_EncodeDecodeRoundtrip(t *testing.T) {
	t.Parallel()
	hash, _ := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	block, _ := hex.DecodeString("fedcba9876543210fedcba9876543210")
	s := NewSession(hash, block)

	in := sessVal{Sub: "user-1", Email: "alice@example.com"}
	enc, err := s.Encode("flow_session", in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	var out sessVal
	if err := s.Decode("flow_session", enc, &out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out != in {
		t.Fatalf("got %+v, want %+v", out, in)
	}
}

func TestUnit_Session_TamperedValue_FailsDecode(t *testing.T) {
	t.Parallel()
	hash, _ := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	block, _ := hex.DecodeString("fedcba9876543210fedcba9876543210")
	s := NewSession(hash, block)

	enc, _ := s.Encode("flow_session", sessVal{Sub: "user-1"})
	// flip a character mid-string to corrupt the MAC
	tampered := enc[:len(enc)/2] + "X" + enc[len(enc)/2+1:]

	var out sessVal
	if err := s.Decode("flow_session", tampered, &out); err == nil {
		t.Fatal("expected decode error on tampered cookie")
	}
}

func TestUnit_Session_NewFromHex_InvalidHash_ReturnsError(t *testing.T) {
	t.Parallel()
	if _, err := NewSessionFromHex("not-hex", "fedcba9876543210fedcba9876543210"); err == nil {
		t.Error("expected error on invalid hash hex")
	}
}

func TestUnit_Session_NewFromHex_InvalidBlock_ReturnsError(t *testing.T) {
	t.Parallel()
	if _, err := NewSessionFromHex("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "not-hex"); err == nil {
		t.Error("expected error on invalid block hex")
	}
}
