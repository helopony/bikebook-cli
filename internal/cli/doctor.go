package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/helopony/bikebook-cli/internal/api"
	"github.com/spf13/cobra"
)

type DoctorResult struct {
	Checks []DoctorCheck `json:"checks"`
}

type DoctorCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
	ExitCode int    `json:"exit_code"`
}

func newDoctorCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check BikeBook CLI environment, auth, and connectivity",
		RunE: func(cmd *cobra.Command, args []string) error {
			contract := contractFromOptions(*opts, cmd.OutOrStdout())
			result, exitCode := runDoctor(cmd.Context(), opts, http.DefaultClient)
			if err := RenderData(cmd.OutOrStdout(), contract, result.Checks); err != nil {
				return err
			}
			if exitCode != ExitSuccess {
				return SilentExitError{Code: exitCode}
			}
			return nil
		},
	}
}

func runDoctor(ctx context.Context, opts *rootOptions, client *http.Client) (DoctorResult, int) {
	checks := []DoctorCheck{}
	exitCode := ExitSuccess

	auth, err := ResolveAuth(*opts)
	if err != nil {
		checks = append(checks,
			DoctorCheck{Name: "environment", Status: "fail", Detail: err.Error(), ExitCode: ExitUsage},
			DoctorCheck{Name: "auth", Status: "skip", Detail: "key resolution failed", ExitCode: ExitUsage},
			DoctorCheck{Name: "connectivity", Status: "skip", Detail: "auth unavailable", ExitCode: ExitUsage},
		)
		return DoctorResult{Checks: checks}, ExitUsage
	}

	envDetail := auth.Env
	if envDetail == "" {
		envDetail = EnvAuto
	}
	checks = append(checks, DoctorCheck{
		Name:     "environment",
		Status:   "ok",
		Detail:   "env=" + envDetail + " api_base=" + opts.apiBase + " profile=" + auth.Profile,
		ExitCode: ExitSuccess,
	})

	if auth.APIKey == "" {
		checks = append(checks,
			DoctorCheck{Name: "auth", Status: "fail", Detail: "no API key resolved", ExitCode: ExitAuthentication},
			DoctorCheck{Name: "connectivity", Status: "skip", Detail: "auth unavailable", ExitCode: ExitAuthentication},
		)
		return DoctorResult{Checks: checks}, ExitAuthentication
	}

	checks = append(checks, DoctorCheck{
		Name:     "auth",
		Status:   "ok",
		Detail:   "source=" + string(auth.Source) + " key=" + auth.Redacted,
		ExitCode: ExitSuccess,
	})

	probe, probeExit := probeConnectivity(ctx, client, opts.apiBase, auth.APIKey, opts.requestID)
	checks = append(checks, probe)
	if probeExit != ExitSuccess {
		exitCode = probeExit
	}

	return DoctorResult{Checks: checks}, exitCode
}

func probeConnectivity(ctx context.Context, client *http.Client, apiBase, apiKey, requestID string) (DoctorCheck, int) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	url := strings.TrimRight(apiBase, "/") + "/jobs?limit=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return DoctorCheck{Name: "connectivity", Status: "fail", Detail: err.Error(), ExitCode: ExitNetwork}, ExitNetwork
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	if requestID != "" {
		req.Header.Set("X-Bikebook-Request-Id", requestID)
	}

	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return DoctorCheck{Name: "connectivity", Status: "fail", Detail: err.Error(), ExitCode: ExitNetwork}, ExitNetwork
	}
	defer resp.Body.Close()

	elapsed := time.Since(started).Round(time.Millisecond)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return DoctorCheck{Name: "connectivity", Status: "ok", Detail: "GET /jobs?limit=1 " + resp.Status + " in " + elapsed.String(), ExitCode: ExitSuccess}, ExitSuccess
	}

	apiError := api.ErrorResponse{}
	_ = json.NewDecoder(resp.Body).Decode(&apiError)
	detail := "GET /jobs?limit=1 " + resp.Status
	if apiError.Error != nil && apiError.Error.Message != nil {
		detail += ": " + *apiError.Error.Message
	}
	exit := ExitCodeForHTTPStatus(resp.StatusCode)
	return DoctorCheck{Name: "connectivity", Status: "fail", Detail: detail, ExitCode: exit}, exit
}
