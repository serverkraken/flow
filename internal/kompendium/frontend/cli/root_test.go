package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/frontend/cli"
)

func TestNewRootCmd_Version(t *testing.T) {
	t.Parallel()

	cmd := cli.NewRootCmd(cli.Deps{})
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), cli.Version) {
		t.Errorf("expected version %q in output, got %q", cli.Version, out.String())
	}
}

func TestNewRootCmd_Help(t *testing.T) {
	t.Parallel()

	cmd := cli.NewRootCmd(cli.Deps{})
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got := out.String()
	for _, want := range []string{"kompendium", "Markdown notebook", "new", "ls", "search"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in help output, got %q", want, got)
		}
	}
}

func TestExecute_Version(t *testing.T) {
	t.Parallel()

	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.Execute([]string{"--version"}, out, errOut, cli.Deps{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out.String(), cli.Version) {
		t.Errorf("expected version, got %q", out.String())
	}
}

func TestVersionSubcommand(t *testing.T) {
	t.Parallel()

	stdout, _, err := runCmd(t, cli.Deps{}, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(stdout, cli.Version) {
		t.Errorf("expected %q in output, got %q", cli.Version, stdout)
	}
}
