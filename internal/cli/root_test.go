package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootHelpIncludesGlobalFlags(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	SetOutput(cmd, &out, &out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"agent-first CLI",
		"--api-base",
		"--api-key",
		"--debug",
		"--env",
		"--idempotency-key",
		"--json",
		"--no-color",
		"--raw",
		"--request-id",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	previousVersion, previousCommit := version, commit
	t.Cleanup(func() {
		version = previousVersion
		commit = previousCommit
	})
	SetVersionInfo("1.2.3", "abc123")

	cmd := NewRootCommand()
	var out bytes.Buffer
	SetOutput(cmd, &out, &out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}

	got := out.String()
	for _, want := range []string{"bikebook 1.2.3", "commit abc123", defaultAPIBase} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output missing %q: %s", want, got)
		}
	}
}
