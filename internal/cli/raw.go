package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/helopony/bikebook-cli/internal/api"
	"github.com/spf13/cobra"
)

type rawOptions struct {
	data    string
	headers []string
}

type rawResponse struct {
	Status     int         `json:"status"`
	StatusText string      `json:"status_text"`
	Headers    http.Header `json:"headers,omitempty"`
	Body       any         `json:"body,omitempty"`
}

func newRawCommand(opts *rootOptions) *cobra.Command {
	rawOpts := rawOptions{}
	cmd := &cobra.Command{
		Use:   "raw <METHOD> <PATH>",
		Short: "Issue an authenticated raw BikeBook API request",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			contract := contractFromOptions(*opts, cmd.OutOrStdout())
			auth, err := ResolveAuth(*opts)
			if err != nil {
				return err
			}
			if auth.APIKey == "" {
				return NewLocalError(ExitAuthentication, "cli_authentication_required", "API key is required", "api_key", "set BIKEBOOK_API_KEY or run `bikebook config set api-key`", "")
			}

			client := &http.Client{Timeout: 30 * time.Second}
			response, err := executeRawRequest(cmd.Context(), client, *opts, rawOpts, auth.APIKey, args[0], args[1], cmd.InOrStdin())
			if err != nil {
				return err
			}
			return RenderData(cmd.OutOrStdout(), contract, response.Body)
		},
	}
	cmd.Flags().StringVar(&rawOpts.data, "data", "", "request body; use @file.json or - for stdin")
	cmd.Flags().StringArrayVar(&rawOpts.headers, "header", nil, "additional request header, repeatable (Name: value)")
	return cmd
}

func executeRawRequest(ctx context.Context, client *http.Client, opts rootOptions, rawOpts rawOptions, apiKey, method, path string, stdin io.Reader) (rawResponse, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return rawResponse{}, NewLocalError(ExitUsage, "cli_invalid_input", "HTTP method is required", "method", "", "")
	}

	body, err := rawRequestBody(rawOpts.data, method, stdin)
	if err != nil {
		return rawResponse{}, err
	}
	url := rawRequestURL(opts.apiBase, path)
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return rawResponse{}, NewLocalError(ExitUsage, "cli_invalid_input", err.Error(), "path", "", "")
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if opts.requestID != "" {
		req.Header.Set("X-Bikebook-Request-Id", opts.requestID)
	} else {
		req.Header.Set("X-Bikebook-Request-Id", uuid.NewString())
	}
	if methodNeedsIdempotency(method) {
		key := opts.idempotencyKey
		if key == "" {
			generated, err := uuid.NewV7()
			if err != nil {
				return rawResponse{}, err
			}
			key = generated.String()
		}
		req.Header.Set("Idempotency-Key", key)
	}
	if err := applyRawHeaders(req.Header, rawOpts.headers); err != nil {
		return rawResponse{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return rawResponse{}, NewLocalError(ExitNetwork, "cli_network_error", err.Error(), "", "", "")
	}
	defer resp.Body.Close()

	parsedBody, rawBytes, err := decodeRawResponseBody(resp)
	if err != nil {
		return rawResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return rawResponse{}, rawResponseError(resp.StatusCode, parsedBody, rawBytes)
	}
	return rawResponse{Status: resp.StatusCode, StatusText: resp.Status, Headers: resp.Header, Body: parsedBody}, nil
}

func rawRequestURL(apiBase, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return strings.TrimRight(apiBase, "/") + "/" + strings.TrimLeft(path, "/")
}

func rawRequestBody(data, method string, stdin io.Reader) (io.Reader, error) {
	if data != "" {
		bodyBytes, err := rawDataBytes(data, stdin)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(bodyBytes), nil
	}
	if methodAllowsBody(method) && !isTerminal(stdin) {
		stdinBytes, err := io.ReadAll(stdin)
		if err != nil {
			return nil, err
		}
		if len(stdinBytes) > 0 {
			return bytes.NewReader(stdinBytes), nil
		}
	}
	return nil, nil
}

func rawDataBytes(data string, stdin io.Reader) ([]byte, error) {
	if data == "-" {
		return io.ReadAll(stdin)
	}
	if strings.HasPrefix(data, "@") {
		path := strings.TrimPrefix(data, "@")
		if path == "" {
			return nil, NewLocalError(ExitUsage, "cli_invalid_input", "--data @ requires a file path", "data", "", "")
		}
		return os.ReadFile(path)
	}
	return []byte(data), nil
}

func methodAllowsBody(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

func methodNeedsIdempotency(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch
}

func applyRawHeaders(headers http.Header, values []string) error {
	for _, value := range values {
		name, headerValue, ok := strings.Cut(value, ":")
		if !ok {
			name, headerValue, ok = strings.Cut(value, "=")
		}
		if !ok {
			return NewLocalError(ExitUsage, "cli_invalid_header", "header must be in `Name: value` format", "header", "", "")
		}
		name = textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name))
		headerValue = strings.TrimSpace(headerValue)
		if name == "" {
			return NewLocalError(ExitUsage, "cli_invalid_header", "header name is required", "header", "", "")
		}
		headers.Add(name, headerValue)
	}
	return nil
}

func decodeRawResponseBody(resp *http.Response) (any, []byte, error) {
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if len(bytes) == 0 {
		return nil, bytes, nil
	}

	contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if contentType == "application/json" || json.Valid(bytes) {
		var payload any
		if err := json.Unmarshal(bytes, &payload); err != nil {
			return nil, bytes, NewLocalError(ExitGeneric, "cli_invalid_response", err.Error(), "", "", "")
		}
		return payload, bytes, nil
	}
	return string(bytes), bytes, nil
}

func rawResponseError(status int, parsedBody any, rawBytes []byte) error {
	return responseErrorWithHint(status, parsedBody, rawBytes, "", "")
}

func responseErrorWithHint(status int, parsedBody any, rawBytes []byte, hint, docsURL string) error {
	envelope := api.ErrorResponse{}
	if len(rawBytes) > 0 {
		_ = json.Unmarshal(rawBytes, &envelope)
	}
	if envelope.Error == nil {
		message := http.StatusText(status)
		if message == "" {
			message = "upstream request failed"
		}
		if text, ok := parsedBody.(string); ok && strings.TrimSpace(text) != "" {
			message = strings.TrimSpace(text)
		}
		envelope.Error = &api.Error{Code: errorCodePtr("upstream_error"), Message: stringPtr(message)}
	}
	return NewAPIError(status, envelope, hint, docsURL)
}
