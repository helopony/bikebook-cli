package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestWriteCommandsAllEndpointsHaveHappyPath(t *testing.T) {
	withTempHome(t)
	if got := len(writeCommandSpecs()); got != 17 {
		t.Fatalf("write command count = %d, want 17", got)
	}

	for _, spec := range writeCommandSpecs() {
		t.Run(spec.Group+" "+spec.Use, func(t *testing.T) {
			var gotBody string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != spec.Method {
					t.Fatalf("method = %s, want %s", r.Method, spec.Method)
				}
				if r.Header.Get("Authorization") != "Bearer bbk_live_secret" {
					t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
				}
				key := r.Header.Get("Idempotency-Key")
				parsed, err := uuid.Parse(key)
				if err != nil || parsed.Version() != 7 {
					t.Fatalf("Idempotency-Key = %q, parse err = %v", key, err)
				}
				body, _ := io.ReadAll(r.Body)
				gotBody = string(body)
				w.Header().Set("Content-Type", "application/json")
				if spec.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				_, _ = w.Write([]byte(`{"id":"ok_1"}`))
			}))
			t.Cleanup(server.Close)

			args := []string{"--api-key", "bbk_live_secret", "--api-base", server.URL, "--json", spec.Group, spec.Use}
			for _, pathArg := range spec.PathArgs {
				args = append(args, samplePathArg(pathArg))
			}
			if spec.HasBody {
				args = append(args, "--field", "name=Example")
			}
			if spec.Destructive {
				args = append(args, "--yes")
			}
			var stdout, stderr bytes.Buffer
			code := ExecuteWithArgs(args, &stdout, &stderr)
			if code != ExitSuccess {
				t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "Idempotency-Key:") {
				t.Fatalf("stderr missing idempotency key: %s", stderr.String())
			}
			if spec.HasBody && !json.Valid([]byte(gotBody)) {
				t.Fatalf("body is not json: %q", gotBody)
			}
		})
	}
}

func TestWriteDryRunPrintsRequestWithoutExecuting(t *testing.T) {
	withTempHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("dry-run executed network request")
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL + "/public/v1",
		"--idempotency-key", "idem_123",
		"--json",
		"jobs", "create", "--field", "customer_id=cus_1", "--dry-run",
	}, &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	var payload writeDryRun
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Method != http.MethodPost || !strings.HasSuffix(payload.URL, "/public/v1/jobs") {
		t.Fatalf("unexpected dry-run request: %+v", payload)
	}
	if payload.Headers["Authorization"] != "Bearer bbk_live_***" {
		t.Fatalf("Authorization not redacted: %+v", payload.Headers)
	}
	if payload.Headers["Idempotency-Key"] != "idem_123" {
		t.Fatalf("idempotency header = %q", payload.Headers["Idempotency-Key"])
	}
	if !strings.Contains(stderr.String(), "Idempotency-Key: idem_123") {
		t.Fatalf("stderr missing provided key: %s", stderr.String())
	}
}

func TestWriteBodyFromFileAndStdin(t *testing.T) {
	withTempHome(t)
	bodyFile := t.TempDir() + "/body.json"
	if err := os.WriteFile(bodyFile, []byte(`{"customer_id":"cus_file"}`), 0600); err != nil {
		t.Fatal(err)
	}
	seenBodies := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seenBodies = append(seenBodies, string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL,
		"--json",
		"jobs", "create", "--from-file", bodyFile,
	}, &stdout, &stderr)
	if code != ExitSuccess {
		t.Fatalf("file exit = %d, stderr = %s", code, stderr.String())
	}

	cmd, opts := newRootCommand()
	stdout.Reset()
	stderr.Reset()
	SetOutput(cmd, &stdout, &stderr)
	cmd.SetIn(strings.NewReader(`{"name":"Ada"}`))
	cmd.SetArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL,
		"--json",
		"customers", "update", "customer_1", "--from-stdin",
	})
	code = ExitSuccess
	if err := cmd.Execute(); err != nil {
		code = RenderError(&stderr, contractFromOptions(*opts, &stdout), err)
	}
	if code != ExitSuccess {
		t.Fatalf("stdin exit = %d, stderr = %s", code, stderr.String())
	}
	if len(seenBodies) != 2 || seenBodies[0] != `{"customer_id":"cus_file"}` || seenBodies[1] != `{"name":"Ada"}` {
		t.Fatalf("unexpected bodies: %#v", seenBodies)
	}
}

func TestDestructiveWriteRequiresYes(t *testing.T) {
	withTempHome(t)
	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", "https://api.example.test",
		"--json",
		"webhook-endpoints", "delete", "endpoint_1",
	}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "cli_confirmation_required") {
		t.Fatalf("stderr missing confirmation error: %s", stderr.String())
	}
}

func TestWriteConflictIncludesIdempotencyHint(t *testing.T) {
	withTempHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"idempotency_conflict","message":"Replay mismatch"},"request_id":"req_1"}`))
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL,
		"--json",
		"jobs", "create", "--field", "customer_id=cus_1",
	}, &stdout, &stderr)

	if code != ExitConflict {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Idempotency-Key") {
		t.Fatalf("stderr missing conflict hint: %s", stderr.String())
	}
}
