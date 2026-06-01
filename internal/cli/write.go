package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/helopony/bikebook-cli/internal/api"
	"github.com/spf13/cobra"
)

type writeCommandValues struct {
	query       map[string]*string
	fields      []string
	fromFile    string
	fromStdin   bool
	dryRun      bool
	yes         bool
	contentType string
}

type writeCommandSpec struct {
	Group       string
	Use         string
	Short       string
	Method      string
	PathArgs    []string
	QueryFlags  []string
	HasBody     bool
	Destructive bool
	Execute     func(context.Context, *api.Client, []string, io.Reader, string, []api.RequestEditorFn) (*http.Response, error)
}

type writeDryRun struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body,omitempty"`
}

func addWriteCommands(root *cobra.Command, opts *rootOptions) {
	groups := map[string]*cobra.Command{}
	for _, child := range root.Commands() {
		groups[child.Name()] = child
	}
	for _, spec := range writeCommandSpecs() {
		group := groups[spec.Group]
		if group == nil {
			group = &cobra.Command{
				Use:   spec.Group,
				Short: readGroupShort(spec.Group),
			}
			groups[spec.Group] = group
			root.AddCommand(group)
		}
		group.AddCommand(newWriteCommand(opts, spec))
	}
}

func newWriteCommand(opts *rootOptions, spec writeCommandSpec) *cobra.Command {
	values := writeCommandValues{
		query:       map[string]*string{},
		contentType: "application/json",
	}
	cmd := &cobra.Command{
		Use:   writeUse(spec),
		Short: spec.Short,
		Args:  cobra.ExactArgs(len(spec.PathArgs)),
		RunE: func(cmd *cobra.Command, args []string) error {
			contract := contractFromOptions(*opts, cmd.OutOrStdout())
			auth, err := ResolveAuth(*opts)
			if err != nil {
				return err
			}
			if auth.APIKey == "" {
				return NewLocalError(ExitAuthentication, "cli_authentication_required", "API key is required", "api_key", "set BIKEBOOK_API_KEY or run `bikebook config set api-key`", "")
			}
			if spec.Destructive && !values.yes {
				return NewLocalError(ExitUsage, "cli_confirmation_required", "destructive operation requires --yes", "yes", "rerun with --yes when you intend to execute this operation", "")
			}

			body, bodyBytes, err := writeRequestBody(spec, values, cmd.InOrStdin())
			if err != nil {
				return err
			}
			idempotencyKey, err := resolveWriteIdempotencyKey(*opts)
			if err != nil {
				return err
			}
			if !opts.quiet {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Idempotency-Key: %s\n", idempotencyKey)
			}

			query := compactQuery(values.query)
			if values.dryRun {
				dryRun, err := buildWriteDryRun(cmd.Context(), spec, args, bodyBytes, values.contentType, *opts, auth.APIKey, idempotencyKey, query)
				if err != nil {
					return err
				}
				return RenderData(cmd.OutOrStdout(), contract, dryRun)
			}

			client, err := api.NewClient(generatedClientBaseURL(opts.apiBase), api.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}))
			if err != nil {
				return NewLocalError(ExitUsage, "cli_invalid_input", err.Error(), "api_base", "", "")
			}
			response, err := executeWriteRequest(cmd.Context(), client, spec, args, body, values.contentType, *opts, auth.APIKey, idempotencyKey, query)
			if err != nil {
				return err
			}
			return RenderData(cmd.OutOrStdout(), contract, response)
		},
	}
	for _, flag := range spec.QueryFlags {
		values.query[flag] = new(string)
		cmd.Flags().StringVar(values.query[flag], kebab(flag), "", "filter by "+flag)
	}
	if spec.HasBody {
		cmd.Flags().StringArrayVar(&values.fields, "field", nil, "request JSON field as key=value, repeatable")
		cmd.Flags().StringVar(&values.fromFile, "from-file", "", "read request JSON body from file")
		cmd.Flags().BoolVar(&values.fromStdin, "from-stdin", false, "read request JSON body from stdin")
	}
	cmd.Flags().BoolVar(&values.dryRun, "dry-run", false, "print request without executing it")
	if spec.Destructive {
		cmd.Flags().BoolVar(&values.yes, "yes", false, "confirm destructive operation")
	}
	return cmd
}

func writeUse(spec writeCommandSpec) string {
	use := spec.Use
	for _, arg := range spec.PathArgs {
		use += " <" + kebab(arg) + ">"
	}
	return use
}

func writeRequestBody(spec writeCommandSpec, values writeCommandValues, stdin io.Reader) (io.Reader, []byte, error) {
	if !spec.HasBody {
		return nil, nil, nil
	}
	sources := 0
	if values.fromFile != "" {
		sources++
	}
	if values.fromStdin {
		sources++
	}
	if len(values.fields) > 0 {
		sources++
	}
	if sources == 0 {
		return nil, nil, NewLocalError(ExitUsage, "cli_body_required", "request body is required", "body", "use --field, --from-file, or --from-stdin", "")
	}
	if sources > 1 {
		return nil, nil, NewLocalError(ExitUsage, "cli_invalid_input", "choose only one body source", "body", "use --field, --from-file, or --from-stdin", "")
	}

	var bodyBytes []byte
	var err error
	switch {
	case values.fromFile != "":
		bodyBytes, err = os.ReadFile(values.fromFile)
	case values.fromStdin:
		bodyBytes, err = io.ReadAll(stdin)
	default:
		bodyBytes, err = json.Marshal(bodyFromFields(values.fields))
	}
	if err != nil {
		return nil, nil, NewLocalError(ExitUsage, "cli_invalid_input", err.Error(), "body", "", "")
	}
	if !json.Valid(bodyBytes) {
		return nil, nil, NewLocalError(ExitUsage, "cli_invalid_json", "request body must be valid JSON", "body", "", "")
	}
	return bytes.NewReader(bodyBytes), bodyBytes, nil
}

func bodyFromFields(fields []string) map[string]any {
	body := map[string]any{}
	for _, field := range fields {
		name, value, ok := strings.Cut(field, "=")
		if !ok {
			body[strings.TrimSpace(field)] = ""
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		body[name] = parseFieldValue(strings.TrimSpace(value))
	}
	return body
}

func parseFieldValue(value string) any {
	var parsed any
	if json.Unmarshal([]byte(value), &parsed) == nil {
		return parsed
	}
	return value
}

func resolveWriteIdempotencyKey(opts rootOptions) (string, error) {
	if opts.idempotencyKey != "" {
		return opts.idempotencyKey, nil
	}
	generated, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return generated.String(), nil
}

func executeWriteRequest(ctx context.Context, client *api.Client, spec writeCommandSpec, pathArgs []string, body io.Reader, contentType string, opts rootOptions, apiKey, idempotencyKey string, query map[string]string) (any, error) {
	resp, err := spec.Execute(ctx, client, pathArgs, body, contentType, []api.RequestEditorFn{writeRequestEditor(opts, apiKey, idempotencyKey, query)})
	if err != nil {
		return nil, NewLocalError(ExitNetwork, "cli_network_error", err.Error(), "", "", "")
	}
	defer resp.Body.Close()

	parsedBody, rawBytes, err := decodeRawResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		hint := ""
		if resp.StatusCode == http.StatusConflict {
			hint = "check the Idempotency-Key; a replay with different input may have been rejected"
		}
		return nil, responseErrorWithHint(resp.StatusCode, parsedBody, rawBytes, hint, "")
	}
	return parsedBody, nil
}

func buildWriteDryRun(ctx context.Context, spec writeCommandSpec, pathArgs []string, bodyBytes []byte, contentType string, opts rootOptions, apiKey, idempotencyKey string, query map[string]string) (writeDryRun, error) {
	capture := &captureDoer{}
	client, err := api.NewClient(generatedClientBaseURL(opts.apiBase), api.WithHTTPClient(capture))
	if err != nil {
		return writeDryRun{}, NewLocalError(ExitUsage, "cli_invalid_input", err.Error(), "api_base", "", "")
	}
	var body io.Reader
	if bodyBytes != nil {
		body = bytes.NewReader(bodyBytes)
	}
	_, err = spec.Execute(ctx, client, pathArgs, body, contentType, []api.RequestEditorFn{writeRequestEditor(opts, apiKey, idempotencyKey, query)})
	if err != nil {
		return writeDryRun{}, err
	}
	if capture.request == nil {
		return writeDryRun{}, NewLocalError(ExitGeneric, "cli_invalid_request", "failed to capture dry-run request", "", "", "")
	}
	return dryRunFromRequest(capture.request)
}

type captureDoer struct {
	request *http.Request
}

func (d *captureDoer) Do(req *http.Request) (*http.Response, error) {
	d.request = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     http.StatusText(http.StatusOK),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Request:    req,
	}, nil
}

func dryRunFromRequest(req *http.Request) (writeDryRun, error) {
	headers := map[string]string{}
	for name, values := range req.Header {
		if len(values) == 0 {
			continue
		}
		value := values[0]
		if strings.EqualFold(name, "Authorization") && strings.HasPrefix(value, "Bearer ") {
			value = "Bearer " + RedactAPIKey(strings.TrimPrefix(value, "Bearer "))
		}
		headers[name] = value
	}
	var body any
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return writeDryRun{}, err
		}
		if len(bodyBytes) > 0 {
			if err := json.Unmarshal(bodyBytes, &body); err != nil {
				body = string(bodyBytes)
			}
		}
	}
	return writeDryRun{Method: req.Method, URL: req.URL.String(), Headers: headers, Body: body}, nil
}

func writeRequestEditor(opts rootOptions, apiKey, idempotencyKey string, query map[string]string) api.RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)
		if opts.requestID != "" {
			req.Header.Set("X-Bikebook-Request-Id", opts.requestID)
		} else if req.Header.Get("X-Bikebook-Request-Id") == "" {
			req.Header.Set("X-Bikebook-Request-Id", uuid.NewString())
		}
		values := req.URL.Query()
		for name, value := range query {
			if value != "" {
				values.Set(name, value)
			}
		}
		req.URL.RawQuery = values.Encode()
		return nil
	}
}

func writeCommandSpecs() []writeCommandSpec {
	return []writeCommandSpec{
		{Group: "assets", Use: "create", Short: "Create an asset", Method: http.MethodPost, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.CreateAssetWithBody(ctx, nil, contentType, body, editors...)
		}},
		{Group: "assets", Use: "update", Short: "Update an asset", Method: http.MethodPatch, PathArgs: []string{"asset_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.UpdateAssetWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "assets", Use: "delete", Short: "Delete an asset", Method: http.MethodDelete, PathArgs: []string{"asset_id"}, Destructive: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.DeleteAsset(ctx, args[0], nil, editors...)
		}},
		{Group: "back-orders", Use: "create", Short: "Create a back order", Method: http.MethodPost, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.CreateWithBody(ctx, nil, contentType, body, editors...)
		}},
		{Group: "back-orders", Use: "receive", Short: "Receive back orders", Method: http.MethodPost, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ReceiveWithBody(ctx, nil, contentType, body, editors...)
		}},
		{Group: "back-orders", Use: "update", Short: "Update a back order", Method: http.MethodPatch, PathArgs: []string{"back_order_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.UpdateWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "back-orders", Use: "delete", Short: "Delete a back order", Method: http.MethodDelete, PathArgs: []string{"back_order_id"}, Destructive: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.Delete(ctx, args[0], nil, editors...)
		}},
		{Group: "services", Use: "create", Short: "Create a service", Method: http.MethodPost, PathArgs: []string{"business_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.CreateServiceWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "chat", Use: "create-message", Short: "Create a chat message", Method: http.MethodPost, PathArgs: []string{"customer_id"}, QueryFlags: []string{"job_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.CreateChatMessageWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "chat", Use: "create-attachments", Short: "Create chat attachments", Method: http.MethodPost, PathArgs: []string{"customer_id"}, QueryFlags: []string{"job_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.CreateChatAttachmentsWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "chat", Use: "mark-read", Short: "Mark chat messages read", Method: http.MethodPost, PathArgs: []string{"customer_id"}, QueryFlags: []string{"job_id"}, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.MarkRead(ctx, args[0], nil, editors...)
		}},
		{Group: "customers", Use: "create", Short: "Create a customer", Method: http.MethodPost, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.CreateCustomerWithBody(ctx, nil, contentType, body, editors...)
		}},
		{Group: "customers", Use: "update", Short: "Update a customer", Method: http.MethodPatch, PathArgs: []string{"customer_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.UpdateCustomerWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "integrations", Use: "trigger-customer-sync", Short: "Trigger customer integration sync", Method: http.MethodPost, PathArgs: []string{"business_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.TriggerCustomerSyncWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "integrations", Use: "trigger-stock-sync", Short: "Trigger stock integration sync", Method: http.MethodPost, PathArgs: []string{"business_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.TriggerStockSyncWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "integrations", Use: "change-invoice-auto-sync", Short: "Change invoice integration auto-sync", Method: http.MethodPost, PathArgs: []string{"invoice_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ChangeInvoiceAutoSyncWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "integrations", Use: "change-invoice-sync-completion", Short: "Change invoice integration sync completion", Method: http.MethodPost, PathArgs: []string{"invoice_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ChangeInvoiceSyncCompletionWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "integrations", Use: "retry-invoice-sync", Short: "Retry invoice integration sync", Method: http.MethodPost, PathArgs: []string{"invoice_id"}, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.RetryInvoiceIntegrationSync(ctx, args[0], nil, editors...)
		}},
		{Group: "integrations", Use: "trigger-invoice-sync", Short: "Trigger invoice integration sync", Method: http.MethodPost, PathArgs: []string{"invoice_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.TriggerInvoiceIntegrationSyncWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "invoice-items", Use: "update", Short: "Update an invoice item", Method: http.MethodPatch, PathArgs: []string{"invoice_item_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.UpdateInvoiceItemWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "invoice-items", Use: "delete", Short: "Delete an invoice item", Method: http.MethodDelete, PathArgs: []string{"invoice_item_id"}, Destructive: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.DeleteInvoiceItem(ctx, args[0], nil, editors...)
		}},
		{Group: "invoice-items", Use: "create", Short: "Create an invoice item", Method: http.MethodPost, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.CreateInvoiceItemWithBody(ctx, nil, contentType, body, editors...)
		}},
		{Group: "invoices", Use: "update", Short: "Update an invoice", Method: http.MethodPatch, PathArgs: []string{"invoice_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.UpdateInvoiceWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "invoices", Use: "change-status", Short: "Change invoice status", Method: http.MethodPost, PathArgs: []string{"invoice_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ChangeStatusWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "invoices", Use: "payment-link", Short: "Create invoice payment link", Method: http.MethodPost, PathArgs: []string{"invoice_id"}, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.PaymentLink(ctx, args[0], nil, editors...)
		}},
		{Group: "invoices", Use: "record-payment", Short: "Record invoice payment", Method: http.MethodPost, PathArgs: []string{"invoice_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.RecordPaymentWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "invoices", Use: "send", Short: "Send an invoice", Method: http.MethodPost, PathArgs: []string{"invoice_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.SendWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "invoices", Use: "refund", Short: "Refund a payment", Method: http.MethodPost, PathArgs: []string{"payment_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.RefundWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "jobs", Use: "submit-part-authorisation-decisions", Short: "Submit part authorisation decisions", Method: http.MethodPost, PathArgs: []string{"job_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.SubmitPartAuthorisationDecisionsWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "jobs", Use: "create", Short: "Create a job", Method: http.MethodPost, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.CreateJobBookingWithBody(ctx, nil, contentType, body, editors...)
		}},
		{Group: "jobs", Use: "update", Short: "Update a job", Method: http.MethodPatch, PathArgs: []string{"job_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.UpdateJobWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "jobs", Use: "change-accepted-status", Short: "Change job accepted status", Method: http.MethodPost, PathArgs: []string{"job_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ChangeAcceptedStatusWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "jobs", Use: "request-part-authorisation", Short: "Request part authorisation", Method: http.MethodPost, PathArgs: []string{"job_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.RequestPartAuthorisationWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "jobs", Use: "change-schedule", Short: "Change job schedule", Method: http.MethodPost, PathArgs: []string{"job_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ChangeScheduleWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "jobs", Use: "update-work-line", Short: "Update a job work line", Method: http.MethodPatch, PathArgs: []string{"job_id", "work_line_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.UpdateWorkLineWithBody(ctx, args[0], args[1], nil, contentType, body, editors...)
		}},
		{Group: "jobs", Use: "assign-work-line-mechanic", Short: "Assign a work line mechanic", Method: http.MethodPost, PathArgs: []string{"job_id", "work_line_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.AssignWorkLineMechanicWithBody(ctx, args[0], args[1], nil, contentType, body, editors...)
		}},
		{Group: "jobs", Use: "replace-work-line-services", Short: "Replace work line services", Method: http.MethodPatch, PathArgs: []string{"job_id", "work_line_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ReplaceWorkLineServicesWithBody(ctx, args[0], args[1], nil, contentType, body, editors...)
		}},
		{Group: "jobs", Use: "change-work-line-status", Short: "Change work line status", Method: http.MethodPost, PathArgs: []string{"job_id", "work_line_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ChangeWorkLineStatusWithBody(ctx, args[0], args[1], nil, contentType, body, editors...)
		}},
		{Group: "services", Use: "update", Short: "Update a service", Method: http.MethodPatch, PathArgs: []string{"business_id", "service_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.UpdateServiceWithBody(ctx, args[0], args[1], nil, contentType, body, editors...)
		}},
		{Group: "webhook-deliveries", Use: "replay", Short: "Replay a webhook delivery", Method: http.MethodPost, PathArgs: []string{"delivery_id"}, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.Replay(ctx, args[0], nil, editors...)
		}},
		{Group: "webhook-endpoints", Use: "create", Short: "Create a webhook endpoint", Method: http.MethodPost, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.CreateWebhookEndpointWithBody(ctx, nil, contentType, body, editors...)
		}},
		{Group: "webhook-endpoints", Use: "update", Short: "Update a webhook endpoint", Method: http.MethodPatch, PathArgs: []string{"endpoint_id"}, HasBody: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.UpdateWebhookEndpointWithBody(ctx, args[0], nil, contentType, body, editors...)
		}},
		{Group: "webhook-endpoints", Use: "delete", Short: "Delete a webhook endpoint", Method: http.MethodDelete, PathArgs: []string{"endpoint_id"}, Destructive: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.DeleteWebhookEndpoint(ctx, args[0], nil, editors...)
		}},
		{Group: "webhook-endpoints", Use: "rotate-secret", Short: "Rotate a webhook endpoint secret", Method: http.MethodPost, PathArgs: []string{"endpoint_id"}, Destructive: true, Execute: func(ctx context.Context, c *api.Client, args []string, body io.Reader, contentType string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.RotateSecret(ctx, args[0], nil, editors...)
		}},
	}
}
