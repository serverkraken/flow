// Package opener launches the OS default handler for a URL (the default
// browser for http/https). Used by the docs TUI to follow real weblinks.
package opener

import (
	"os/exec"
	"runtime"
)

// Opener opens URLs in the OS default application.
type Opener struct{}

// New returns a ready Opener.
func New() *Opener { return &Opener{} }

// Open launches url without blocking. Errors from a missing opener binary are
// returned but the process is never waited on.
func (Opener) Open(url string) error {
	bin, args := commandFor(url)
	return exec.Command(bin, args...).Start()
}

func commandFor(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}
