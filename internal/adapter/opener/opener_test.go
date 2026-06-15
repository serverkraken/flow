package opener

import (
	"runtime"
	"testing"
)

func TestCommandFor(t *testing.T) {
	bin, args := commandFor("https://example.com")
	switch runtime.GOOS {
	case "darwin":
		if bin != "open" || len(args) != 1 || args[0] != "https://example.com" {
			t.Fatalf("darwin: got %s %v", bin, args)
		}
	case "linux":
		if bin != "xdg-open" {
			t.Fatalf("linux: got %s", bin)
		}
	}
}
