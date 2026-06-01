package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestDescribeDocumentIncludesCommandsAndOpenAPI(t *testing.T) {
	root := NewRootCommand()
	doc, err := BuildDescribeDocument(root)
	if err != nil {
		t.Fatalf("BuildDescribeDocument() error = %v", err)
	}

	if len(doc.Commands) == 0 {
		t.Fatal("describe document has no commands")
	}
	if len(doc.OpenAPI.Operations) != 82 {
		t.Fatalf("OpenAPI operations = %d, want 82", len(doc.OpenAPI.Operations))
	}

	commandsByName := map[string]DescribeCommand{}
	for _, command := range doc.Commands {
		if command.Name == "" {
			t.Fatal("command has empty name")
		}
		if len(command.ExitCodes) == 0 {
			t.Fatalf("command %s has no exit codes", command.Name)
		}
		if command.Example.Command == "" {
			t.Fatalf("command %s has no example", command.Name)
		}
		commandsByName[command.Name] = command
	}

	for _, want := range []string{
		"bikebook",
		"bikebook config set",
		"bikebook config profiles list",
		"bikebook describe",
		"bikebook doctor",
		"bikebook version",
	} {
		if _, ok := commandsByName[want]; !ok {
			t.Fatalf("missing command %q", want)
		}
	}

	assertRegisteredCommandsDescribed(t, root, commandsByName)
}

func TestDescribeSingleCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := ExecuteWithArgs([]string{"--json", "describe", "version"}, &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	var command DescribeCommand
	if err := json.Unmarshal(stdout.Bytes(), &command); err != nil {
		t.Fatalf("unmarshal describe output: %v\n%s", err, stdout.String())
	}
	if command.Name != "bikebook version" {
		t.Fatalf("command name = %q", command.Name)
	}
	if command.ExpectedOutputSchema == nil {
		t.Fatal("version command missing expected output schema")
	}
}

func TestDescribeUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := ExecuteWithArgs([]string{"describe", "missing"}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "cli_unknown_command") {
		t.Fatalf("stderr missing unknown command code: %s", stderr.String())
	}
}

func assertRegisteredCommandsDescribed(t *testing.T, root *cobra.Command, described map[string]DescribeCommand) {
	t.Helper()
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Hidden || cmd.Name() == "help" || cmd.Name() == "completion" {
			return
		}
		name := strings.Join(commandPath(root, cmd), " ")
		if _, ok := described[name]; !ok {
			t.Fatalf("registered command %q missing from describe schema", name)
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
}
