package gitremote_test

import (
	"os/exec"
	"testing"

	"github.com/serverkraken/flow/internal/gitremote"
)

func TestOriginSlug(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init")
	run("remote", "add", "origin", "git@github.com:serverkraken/flow.git")

	slug, ok, err := gitremote.OriginSlug(dir)
	if err != nil || !ok || slug != "github.com/serverkraken/flow" {
		t.Fatalf("%q %v %v", slug, ok, err)
	}

	// non-repo dir → ok=false, no error
	if _, ok, err := gitremote.OriginSlug(t.TempDir()); ok || err != nil {
		t.Fatalf("non-repo: ok=%v err=%v", ok, err)
	}
}
