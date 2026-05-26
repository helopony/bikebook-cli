package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/helopony/bikebook-cli/internal/api"
)

func TestResolveOutputMode(t *testing.T) {
	tests := []struct {
		name        string
		jsonFlag    bool
		rawFlag     bool
		stdoutIsTTY bool
		want        OutputMode
	}{
		{name: "raw flag wins", jsonFlag: true, rawFlag: true, stdoutIsTTY: true, want: OutputRaw},
		{name: "json flag", jsonFlag: true, stdoutIsTTY: true, want: OutputJSON},
		{name: "tty defaults human", stdoutIsTTY: true, want: OutputHuman},
		{name: "piped defaults raw", stdoutIsTTY: false, want: OutputRaw},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveOutputMode(tt.jsonFlag, tt.rawFlag, tt.stdoutIsTTY); got != tt.want {
				t.Fatalf("ResolveOutputMode() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestContractHonorsEnvironment(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("BIKEBOOK_NON_INTERACTIVE", "true")

	contract := contractFromOptions(rootOptions{}, &bytes.Buffer{})

	if !contract.NoColor {
		t.Fatal("NO_COLOR should enable no-color")
	}
	if !contract.NonInteractive {
		t.Fatal("BIKEBOOK_NON_INTERACTIVE should enable non-interactive mode")
	}
}

func TestExitCodeForHTTPStatus(t *testing.T) {
	tests := map[int]int{
		400: ExitValidation,
		401: ExitAuthentication,
		403: ExitForbidden,
		404: ExitNotFound,
		409: ExitConflict,
		429: ExitRateLimited,
		500: ExitNetwork,
		503: ExitNetwork,
	}

	for status, want := range tests {
		if got := ExitCodeForHTTPStatus(status); got != want {
			t.Fatalf("ExitCodeForHTTPStatus(%d) = %d, want %d", status, got, want)
		}
	}
}

func TestRenderAPIErrorIncludesEnvelopeAndCLIBlock(t *testing.T) {
	requestID := "req_123"
	code := "resource_not_found"
	message := "Job not found"
	parameter := "job_id"
	err := NewAPIError(404, api.ApiErrorResponse{
		Error: &api.ApiError{
			Code:      &code,
			Message:   &message,
			Parameter: &parameter,
		},
		RequestId: &requestID,
	}, "try `bikebook jobs list`", "https://developers.bikebook.com/reference#Jobs_Job")

	var stderr bytes.Buffer
	exit := RenderError(&stderr, Contract{OutputMode: OutputJSON}, err)

	if exit != ExitNotFound {
		t.Fatalf("exit = %d, want %d", exit, ExitNotFound)
	}
	got := stderr.String()
	for _, want := range []string{
		`"code": "resource_not_found"`,
		`"request_id": "req_123"`,
		`"exit_code": 6`,
		`"http_status": 404`,
		`"hint": "try ` + "`bikebook jobs list`" + `"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered error missing %s:\n%s", want, got)
		}
	}
}

func TestRenderLocalErrorUsesCLIPrefixedCode(t *testing.T) {
	err := NewLocalError(ExitUsage, "cli_invalid_input", "missing job_id", "job_id", "", "")

	var stderr bytes.Buffer
	exit := RenderError(&stderr, Contract{OutputMode: OutputRaw}, err)

	if exit != ExitUsage {
		t.Fatalf("exit = %d, want %d", exit, ExitUsage)
	}
	if !strings.Contains(stderr.String(), `"code":"cli_invalid_input"`) {
		t.Fatalf("local error missing cli_* code: %s", stderr.String())
	}
}

func TestRenderDataSnapshots(t *testing.T) {
	fixture := map[string]any{
		"data": []map[string]any{
			{
				"id":     "job_123",
				"object": "job",
				"status": "open",
			},
			{
				"id":     "job_456",
				"object": "job",
				"status": "complete",
			},
		},
		"pagination": map[string]any{
			"has_more":    false,
			"next_cursor": nil,
		},
	}

	tests := []struct {
		name string
		mode OutputMode
	}{
		{name: "human", mode: OutputHuman},
		{name: "json", mode: OutputJSON},
		{name: "raw", mode: OutputRaw},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := RenderData(&out, Contract{OutputMode: tt.mode}, fixture); err != nil {
				t.Fatalf("RenderData() error = %v", err)
			}
			assertGolden(t, "testdata/output_"+tt.name+".golden", out.String())
		})
	}
}

func assertGolden(t *testing.T, path, got string) {
	t.Helper()
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if got != string(wantBytes) {
		t.Fatalf("snapshot mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", path, got, string(wantBytes))
	}
}
