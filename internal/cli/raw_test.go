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
)

func TestRawGETSendsAuthCorrelationAndCustomHeaders(t *testing.T) {
	withTempHome(t)
	var gotAuth, gotRequestID, gotCustom string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotRequestID = r.Header.Get("X-Bikebook-Request-Id")
		gotCustom = r.Header.Get("X-Test")
		if r.URL.Path != "/public/v1/jobs" || r.URL.Query().Get("limit") != "1" {
			t.Fatalf("unexpected URL: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"job_1"}],"pagination":{"has_more":false,"next_cursor":null}}`))
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL + "/public/v1",
		"--request-id", "req_test",
		"--json",
		"raw", "--header", "X-Test: yes", "GET", "/jobs?limit=1",
	}, &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if gotAuth != "Bearer bbk_live_secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotRequestID != "req_test" {
		t.Fatalf("request id = %q", gotRequestID)
	}
	if gotCustom != "yes" {
		t.Fatalf("custom header = %q", gotCustom)
	}
	if !strings.Contains(stdout.String(), `"job_1"`) {
		t.Fatalf("stdout missing response body: %s", stdout.String())
	}
}

func TestRawPOSTReadsDataFileAndGeneratesIdempotencyKey(t *testing.T) {
	withTempHome(t)
	bodyFile := t.TempDir() + "/body.json"
	if err := os.WriteFile(bodyFile, []byte(`{"customer_id":"cus_1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var gotBody, gotIdempotency string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdempotency = r.Header.Get("Idempotency-Key")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"job_2"}`))
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL,
		"--json",
		"raw", "POST", "/jobs", "--data", "@" + bodyFile,
	}, &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if gotBody != `{"customer_id":"cus_1"}` {
		t.Fatalf("body = %q", gotBody)
	}
	if gotIdempotency == "" {
		t.Fatal("missing generated Idempotency-Key")
	}
	if !strings.Contains(stdout.String(), `"job_2"`) {
		t.Fatalf("stdout missing response: %s", stdout.String())
	}
}

func TestRawPATCHUsesProvidedIdempotencyAndStdin(t *testing.T) {
	withTempHome(t)
	var gotIdempotency, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdempotency = r.Header.Get("Idempotency-Key")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	cmd, opts := newRootCommand()
	var stdout, stderr bytes.Buffer
	SetOutput(cmd, &stdout, &stderr)
	cmd.SetIn(strings.NewReader(`{"status":"done"}`))
	cmd.SetArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL,
		"--idempotency-key", "idem_123",
		"--json",
		"raw", "PATCH", "/jobs/job_1",
	})
	code := ExitSuccess
	if err := cmd.Execute(); err != nil {
		code = RenderError(&stderr, contractFromOptions(*opts, &stdout), err)
	}

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if gotIdempotency != "idem_123" {
		t.Fatalf("Idempotency-Key = %q", gotIdempotency)
	}
	if gotBody != `{"status":"done"}` {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestRawMapsUpstreamError(t *testing.T) {
	withTempHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"resource_not_found","message":"Missing"},"request_id":"req_1"}`))
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL,
		"--json",
		"raw", "GET", "/missing",
	}, &stdout, &stderr)

	if code != ExitNotFound {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty on error: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "resource_not_found") {
		t.Fatalf("stderr missing API error: %s", stderr.String())
	}
}

func TestRawRequestURL(t *testing.T) {
	got := rawRequestURL("https://api.example/public/v1/", "/jobs?limit=1")
	if got != "https://api.example/public/v1/jobs?limit=1" {
		t.Fatalf("url = %q", got)
	}
}

func TestRawResponseJSONDecode(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}
	payload, _, err := decodeRawResponseBody(resp)
	if err != nil {
		t.Fatal(err)
	}
	bytes, _ := json.Marshal(payload)
	if string(bytes) != `{"ok":true}` {
		t.Fatalf("payload = %s", string(bytes))
	}
}
