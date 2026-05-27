package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnvTestDoesNotRewriteAPIBase(t *testing.T) {
	withTempHome(t)
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"job_1"}],"pagination":{"has_more":false,"next_cursor":null}}`))
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_test_secret",
		"--env", "test",
		"--api-base", server.URL + "/public/v1",
		"--json",
		"jobs", "list", "--limit", "1",
	}, &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if gotPath != "/public/v1/jobs" {
		t.Fatalf("path = %q, want generated client to keep explicit base host and public prefix", gotPath)
	}
}
