package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadCommandsAllGETsHaveHappyPath(t *testing.T) {
	withTempHome(t)
	if got := len(readCommandSpecs()); got != 47 {
		t.Fatalf("read command count = %d, want 47", got)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer bbk_live_secret" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Bikebook-Request-Id") == "" {
			t.Fatal("missing request id")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/list") {
			t.Fatalf("unexpected generated path: %s", r.URL.Path)
		}
		if strings.HasSuffix(r.URL.Path, "s") || strings.Contains(r.URL.Path, "chat_messages") || strings.Contains(r.URL.Path, "part_authorisations") {
			_, _ = w.Write([]byte(`{"data":[{"id":"item_1"}],"pagination":{"has_more":false,"next_cursor":null}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"item_1"}`))
	}))
	t.Cleanup(server.Close)

	for _, spec := range readCommandSpecs() {
		t.Run(spec.Group+" "+spec.Use, func(t *testing.T) {
			args := []string{"--api-key", "bbk_live_secret", "--api-base", server.URL, "--json", spec.Group, spec.Use}
			for _, pathArg := range spec.PathArgs {
				args = append(args, samplePathArg(pathArg))
			}
			var stdout, stderr bytes.Buffer
			code := ExecuteWithArgs(args, &stdout, &stderr)
			if code != ExitSuccess {
				t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
			}
			if !json.Valid(stdout.Bytes()) {
				t.Fatalf("stdout is not json: %s", stdout.String())
			}
		})
	}
}

func TestActivitiesListFiltersForwarded(t *testing.T) {
	withTempHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/v1/activities" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		for name, want := range map[string]string{
			"job_id":        "job_1",
			"activity_type": "status_changed,comment",
			"created_from":  "2026-06-01T00:00:00Z",
			"created_to":    "2026-06-30T23:59:59Z",
		} {
			if got := r.URL.Query().Get(name); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"has_more":false,"next_cursor":null}}`))
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL,
		"--json",
		"activities", "list",
		"--job-id", "job_1",
		"--activity-type", "status_changed,comment",
		"--created-from", "2026-06-01T00:00:00Z",
		"--created-to", "2026-06-30T23:59:59Z",
	}, &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
}

func TestJobsListJSONFollowsCursorPagination(t *testing.T) {
	withTempHome(t)
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("cursor") {
		case "":
			if r.URL.Query().Get("limit") != "1" {
				t.Fatalf("first page limit = %q", r.URL.Query().Get("limit"))
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"job_1"}],"pagination":{"has_more":true,"next_cursor":"cur_2"}}`))
		case "cur_2":
			_, _ = w.Write([]byte(`{"data":[{"id":"job_2"}],"pagination":{"has_more":false,"next_cursor":null}}`))
		default:
			t.Fatalf("unexpected cursor %q", r.URL.Query().Get("cursor"))
		}
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL + "/public/v1",
		"--json",
		"jobs", "list", "--limit", "1", "--all", "--max", "2",
	}, &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 2 || payload.Data[0]["id"] != "job_1" || payload.Data[1]["id"] != "job_2" {
		t.Fatalf("unexpected payload: %s", stdout.String())
	}
	if len(requests) != 2 || !strings.Contains(requests[1], "cursor=cur_2") {
		t.Fatalf("unexpected requests: %#v", requests)
	}
}

func TestCreatedDateFiltersForwardedForJobsAndInvoices(t *testing.T) {
	withTempHome(t)
	tests := []struct {
		group string
	}{
		{group: "jobs"},
		{group: "invoices"},
	}

	for _, tt := range tests {
		t.Run(tt.group, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("created_from") != "2026-06-07" {
					t.Fatalf("created_from = %q", r.URL.Query().Get("created_from"))
				}
				if r.URL.Query().Get("created_to") != "2026-06-08" {
					t.Fatalf("created_to = %q", r.URL.Query().Get("created_to"))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":[],"pagination":{"has_more":false,"next_cursor":null}}`))
			}))
			t.Cleanup(server.Close)

			var stdout, stderr bytes.Buffer
			code := ExecuteWithArgs([]string{
				"--api-key", "bbk_live_secret",
				"--api-base", server.URL,
				"--json",
				tt.group, "list",
				"--created-from", "2026-06-07",
				"--created-to", "2026-06-08",
			}, &stdout, &stderr)
			if code != ExitSuccess {
				t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
			}
		})
	}
}

func TestPaymentListFiltersForwarded(t *testing.T) {
	withTempHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/v1/payments" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		for name, want := range map[string]string{
			"business_id":     "org_1",
			"invoice_id":      "inv_1",
			"charged_on_from": "2026-06-01T00:00:00Z",
			"charged_on_to":   "2026-06-30T23:59:59Z",
			"amount_min":      "10.50",
			"amount_max":      "250",
		} {
			if got := r.URL.Query().Get(name); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"has_more":false,"next_cursor":null}}`))
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL,
		"--json",
		"payments", "list",
		"--business-id", "org_1",
		"--invoice-id", "inv_1",
		"--charged-on-from", "2026-06-01T00:00:00Z",
		"--charged-on-to", "2026-06-30T23:59:59Z",
		"--amount-min", "10.50",
		"--amount-max", "250",
	}, &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
}

func TestTeamMembersListFiltersForwarded(t *testing.T) {
	withTempHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/public/v1/team_members" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		for name, want := range map[string]string{
			"business_id": "org_1",
			"is_mechanic": "true",
			"enabled":     "true",
		} {
			if got := r.URL.Query().Get(name); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"has_more":false,"next_cursor":null}}`))
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL,
		"--json",
		"team-members", "list",
		"--business-id", "org_1",
		"--is-mechanic", "true",
		"--enabled", "true",
	}, &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
}

func TestJobsListRawEmitsNDJSONDataRows(t *testing.T) {
	withTempHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"job_1"},{"id":"job_2"}],"pagination":{"has_more":false,"next_cursor":null}}`))
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL,
		"--raw",
		"jobs", "list",
	}, &stdout, &stderr)

	if code != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"job_1"`) || !strings.Contains(lines[1], `"job_2"`) {
		t.Fatalf("unexpected raw output: %s", stdout.String())
	}
}

func TestJobsGetMapsUpstreamError(t *testing.T) {
	withTempHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"resource_not_found","message":"Missing job"},"request_id":"req_1"}`))
	}))
	t.Cleanup(server.Close)

	var stdout, stderr bytes.Buffer
	code := ExecuteWithArgs([]string{
		"--api-key", "bbk_live_secret",
		"--api-base", server.URL,
		"--json",
		"jobs", "get", "job_1",
	}, &stdout, &stderr)

	if code != ExitNotFound {
		t.Fatalf("exit = %d, stderr = %s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "resource_not_found") {
		t.Fatalf("stderr missing API error: %s", stderr.String())
	}
}

func samplePathArg(name string) string {
	return strings.TrimSuffix(name, "_id") + "_1"
}
