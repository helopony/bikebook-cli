package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoctorMissingAPIKeyFailsAuthCheck(t *testing.T) {
	withTempHome(t)
	opts := rootOptions{apiBase: "https://example.test/public/v1", env: EnvAuto}

	result, exit := runDoctor(context.Background(), &opts, http.DefaultClient)

	if exit != ExitAuthentication {
		t.Fatalf("exit = %d, want %d", exit, ExitAuthentication)
	}
	if len(result.Checks) != 3 {
		t.Fatalf("checks = %d, want 3", len(result.Checks))
	}
	if result.Checks[1].Name != "auth" || result.Checks[1].Status != "fail" {
		t.Fatalf("auth check = %+v", result.Checks[1])
	}
	if result.Checks[2].Status != "skip" {
		t.Fatalf("connectivity should skip: %+v", result.Checks[2])
	}
}

func TestDoctorConnectivitySuccess(t *testing.T) {
	withTempHome(t)
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/public/v1/jobs" || r.URL.Query().Get("limit") != "1" {
			t.Fatalf("unexpected probe URL: %s", r.URL.String())
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"has_more":false,"next_cursor":null}}`))
	}))
	t.Cleanup(server.Close)

	opts := rootOptions{apiBase: server.URL + "/public/v1", apiKey: "bbk_live_secret", env: EnvAuto}

	result, exit := runDoctor(context.Background(), &opts, server.Client())

	if exit != ExitSuccess {
		t.Fatalf("exit = %d, checks = %+v", exit, result.Checks)
	}
	if gotAuth != "Bearer bbk_live_secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if result.Checks[2].Status != "ok" || !strings.Contains(result.Checks[2].Detail, "200 OK") {
		t.Fatalf("connectivity check = %+v", result.Checks[2])
	}
	if strings.Contains(result.Checks[1].Detail, "bbk_live_secret") || !strings.Contains(result.Checks[1].Detail, "bbk_live_***") {
		t.Fatalf("auth detail did not redact key: %s", result.Checks[1].Detail)
	}
}

func TestDoctorConnectivityMapsHTTPFailure(t *testing.T) {
	withTempHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"authentication_required","message":"Bad key"},"request_id":"req_1"}`))
	}))
	t.Cleanup(server.Close)

	opts := rootOptions{apiBase: server.URL, apiKey: "bbk_live_secret", env: EnvAuto}

	result, exit := runDoctor(context.Background(), &opts, server.Client())

	if exit != ExitAuthentication {
		t.Fatalf("exit = %d, want %d", exit, ExitAuthentication)
	}
	got := result.Checks[2]
	if got.Status != "fail" || !strings.Contains(got.Detail, "Bad key") {
		t.Fatalf("connectivity check = %+v", got)
	}
}
