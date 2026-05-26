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
		"--profile",
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
	for _, want := range []string{`"version":"1.2.3"`, `"commit":"abc123"`, `"api_base":"` + defaultAPIBase + `"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output missing %q: %s", want, got)
		}
	}
}

func TestExecuteWithArgsReturnsUsageExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := ExecuteWithArgs([]string{"--unknown"}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should stay data-only, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "cli_invalid_input") {
		t.Fatalf("stderr missing structured local error: %s", stderr.String())
	}
}

func TestGlobalAuthEnvMismatchReturnsUsageExitCode(t *testing.T) {
	withTempHome(t)
	var stdout, stderr bytes.Buffer

	code := ExecuteWithArgs([]string{"version", "--api-key", "bbk_live_secret", "--env", "test"}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should stay data-only, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "cli_env_key_mismatch") {
		t.Fatalf("stderr missing mismatch error: %s", stderr.String())
	}
}
