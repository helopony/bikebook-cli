package cli

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/helopony/bikebook-cli/internal/api"
	"github.com/spf13/cobra"
)

const defaultReadMax = 1000

type readCommandValues struct {
	query map[string]*string
	all   bool
	max   int
}

type readCommandSpec struct {
	Group       string
	Use         string
	Short       string
	PathArgs    []string
	QueryFlags  []string
	Pageable    bool
	ExecutePage func(context.Context, *api.Client, []string, []api.RequestEditorFn) (*http.Response, error)
}

func newReadCommands(opts *rootOptions) []*cobra.Command {
	groups := map[string]*cobra.Command{}
	order := []string{}
	for _, spec := range readCommandSpecs() {
		group, ok := groups[spec.Group]
		if !ok {
			group = &cobra.Command{
				Use:   spec.Group,
				Short: readGroupShort(spec.Group),
			}
			groups[spec.Group] = group
			order = append(order, spec.Group)
		}
		group.AddCommand(newReadCommand(opts, spec))
	}

	commands := make([]*cobra.Command, 0, len(order))
	for _, name := range order {
		commands = append(commands, groups[name])
	}
	return commands
}

func newReadCommand(opts *rootOptions, spec readCommandSpec) *cobra.Command {
	values := readCommandValues{
		query: map[string]*string{},
		max:   defaultReadMax,
	}
	cmd := &cobra.Command{
		Use:   readUse(spec),
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

			client, err := api.NewClient(generatedClientBaseURL(opts.apiBase), api.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}))
			if err != nil {
				return NewLocalError(ExitUsage, "cli_invalid_input", err.Error(), "api_base", "", "")
			}
			body, err := executeReadRequest(cmd.Context(), client, spec, args, values, *opts, auth.APIKey)
			if err != nil {
				return err
			}
			return RenderData(cmd.OutOrStdout(), contract, body)
		},
	}
	for _, flag := range spec.QueryFlags {
		name := kebab(flag)
		values.query[flag] = new(string)
		cmd.Flags().StringVar(values.query[flag], name, "", "filter by "+flag)
	}
	if spec.Pageable {
		ensurePaginationFlags(cmd, spec, &values)
	}
	return cmd
}

func ensurePaginationFlags(cmd *cobra.Command, spec readCommandSpec, values *readCommandValues) {
	if !stringSliceContains(spec.QueryFlags, "limit") {
		values.query["limit"] = new(string)
		cmd.Flags().StringVar(values.query["limit"], "limit", "", "maximum resources to return in one page")
	}
	if !stringSliceContains(spec.QueryFlags, "cursor") {
		values.query["cursor"] = new(string)
		cmd.Flags().StringVar(values.query["cursor"], "cursor", "", "page cursor")
	}
	cmd.Flags().BoolVar(&values.all, "all", false, "follow cursor pagination until exhausted")
	cmd.Flags().IntVar(&values.max, "max", defaultReadMax, "maximum resources to return when using --all")
}

func readUse(spec readCommandSpec) string {
	use := spec.Use
	for _, arg := range spec.PathArgs {
		use += " <" + kebab(arg) + ">"
	}
	return use
}

func executeReadRequest(ctx context.Context, client *api.Client, spec readCommandSpec, pathArgs []string, values readCommandValues, opts rootOptions, apiKey string) (any, error) {
	if values.all && values.max <= 0 {
		return nil, NewLocalError(ExitUsage, "cli_invalid_input", "--max must be greater than zero", "max", "", "")
	}

	query := compactQuery(values.query)
	first, err := executeReadPage(ctx, client, spec, pathArgs, opts, apiKey, query)
	if err != nil || !values.all || !spec.Pageable {
		return first, err
	}

	accumulator, ok := newPaginatedAccumulator(first)
	if !ok {
		return first, nil
	}
	accumulator.truncate(values.max)
	for accumulator.hasMore() && accumulator.len() < values.max {
		nextCursor := accumulator.nextCursor()
		if nextCursor == "" {
			break
		}
		query["cursor"] = nextCursor
		page, err := executeReadPage(ctx, client, spec, pathArgs, opts, apiKey, query)
		if err != nil {
			return nil, err
		}
		if !accumulator.appendPage(page, values.max) {
			break
		}
	}
	return accumulator.body, nil
}

func executeReadPage(ctx context.Context, client *api.Client, spec readCommandSpec, pathArgs []string, opts rootOptions, apiKey string, query map[string]string) (any, error) {
	resp, err := spec.ExecutePage(ctx, client, pathArgs, []api.RequestEditorFn{readRequestEditor(opts, apiKey, query)})
	if err != nil {
		return nil, NewLocalError(ExitNetwork, "cli_network_error", err.Error(), "", "", "")
	}
	defer resp.Body.Close()

	parsedBody, rawBytes, err := decodeRawResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, rawResponseError(resp.StatusCode, parsedBody, rawBytes)
	}
	return parsedBody, nil
}

func readRequestEditor(opts rootOptions, apiKey string, query map[string]string) api.RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "application/json")
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

func compactQuery(values map[string]*string) map[string]string {
	out := map[string]string{}
	for name, value := range values {
		if value != nil && strings.TrimSpace(*value) != "" {
			out[name] = *value
		}
	}
	return out
}

func generatedClientBaseURL(apiBase string) string {
	return strings.TrimSuffix(strings.TrimRight(apiBase, "/"), "/public/v1")
}

type paginatedAccumulator struct {
	body map[string]any
	data []any
}

func newPaginatedAccumulator(body any) (*paginatedAccumulator, bool) {
	envelope, ok := body.(map[string]any)
	if !ok {
		return nil, false
	}
	data, ok := envelope["data"].([]any)
	if !ok {
		return nil, false
	}
	return &paginatedAccumulator{body: envelope, data: data}, true
}

func (a *paginatedAccumulator) appendPage(page any, max int) bool {
	envelope, ok := page.(map[string]any)
	if !ok {
		return false
	}
	rows, ok := envelope["data"].([]any)
	if !ok {
		return false
	}
	remaining := max - len(a.data)
	if remaining <= 0 {
		a.body["pagination"] = envelope["pagination"]
		return false
	}
	if len(rows) > remaining {
		rows = rows[:remaining]
	}
	a.data = append(a.data, rows...)
	a.body["data"] = a.data
	a.body["pagination"] = envelope["pagination"]
	return true
}

func (a *paginatedAccumulator) truncate(max int) {
	if len(a.data) <= max {
		return
	}
	a.data = a.data[:max]
	a.body["data"] = a.data
}

func (a *paginatedAccumulator) len() int {
	return len(a.data)
}

func (a *paginatedAccumulator) hasMore() bool {
	pagination, ok := a.body["pagination"].(map[string]any)
	if !ok {
		return false
	}
	hasMore, _ := pagination["has_more"].(bool)
	return hasMore
}

func (a *paginatedAccumulator) nextCursor() string {
	pagination, ok := a.body["pagination"].(map[string]any)
	if !ok {
		return ""
	}
	cursor, _ := pagination["next_cursor"].(string)
	return cursor
}

func readCommandSpecs() []readCommandSpec {
	return []readCommandSpec{
		{Group: "assets", Use: "list", Short: "List assets", QueryFlags: []string{"business_id", "customer_id", "name", "serial_number", "make", "model", "bike_type", "sort", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ListAsset(ctx, nil, editors...)
		}},
		{Group: "assets", Use: "list-for-customer", Short: "List assets for a customer", PathArgs: []string{"customer_id"}, QueryFlags: []string{"name", "serial_number", "make", "model", "bike_type", "sort", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ListForCustomer(ctx, args[0], nil, editors...)
		}},
		{Group: "assets", Use: "get", Short: "Get an asset", PathArgs: []string{"asset_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.GetAsset(ctx, args[0], nil, editors...)
		}},
		{Group: "back-orders", Use: "list", Short: "List back orders", QueryFlags: []string{"business_id", "status", "stock_variation_id", "job_id", "invoice_id", "customer_id", "sku", "source", "sort", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.List(ctx, nil, editors...)
		}},
		{Group: "back-orders", Use: "get", Short: "Get a back order", PathArgs: []string{"back_order_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.Get(ctx, args[0], nil, editors...)
		}},
		{Group: "businesses", Use: "list", Short: "List businesses", Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ListBusiness(ctx, nil, editors...)
		}},
		{Group: "businesses", Use: "get", Short: "Get a business", PathArgs: []string{"business_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.GetBusiness(ctx, args[0], nil, editors...)
		}},
		{Group: "businesses", Use: "services", Short: "List services for a business", PathArgs: []string{"business_id"}, QueryFlags: []string{"name", "sort", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.Services(ctx, args[0], nil, editors...)
		}},
		{Group: "businesses", Use: "availability", Short: "Get business availability", PathArgs: []string{"business_id"}, QueryFlags: []string{"from", "to"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.Availability(ctx, args[0], nil, editors...)
		}},
		{Group: "businesses", Use: "availability-slots", Short: "Get business availability slots", PathArgs: []string{"business_id"}, QueryFlags: []string{"date"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.AvailabilitySlots(ctx, args[0], nil, editors...)
		}},
		{Group: "businesses", Use: "next-available-slot", Short: "Get the next available slot for a business", PathArgs: []string{"business_id"}, QueryFlags: []string{"from", "to"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.NextAvailableSlot(ctx, args[0], nil, editors...)
		}},
		{Group: "businesses", Use: "job-statuses", Short: "List job statuses for a business", PathArgs: []string{"business_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.JobStatuses(ctx, args[0], nil, editors...)
		}},
		{Group: "chat", Use: "messages", Short: "List customer chat messages", PathArgs: []string{"customer_id"}, QueryFlags: []string{"job_id", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ChatMessages(ctx, args[0], nil, editors...)
		}},
		{Group: "customers", Use: "list", Short: "List customers", QueryFlags: []string{"business_id", "name", "email", "phone", "sort", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.Customers(ctx, nil, editors...)
		}},
		{Group: "customers", Use: "get", Short: "Get a customer", PathArgs: []string{"customer_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.Customer(ctx, args[0], nil, editors...)
		}},
		{Group: "invoice-items", Use: "get", Short: "Get an invoice item", PathArgs: []string{"invoice_item_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.GetInvoiceItem(ctx, args[0], nil, editors...)
		}},
		{Group: "integrations", Use: "business", Short: "Get business integration", PathArgs: []string{"business_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.BusinessIntegration(ctx, args[0], nil, editors...)
		}},
		{Group: "integrations", Use: "invoice-sync", Short: "Get invoice integration sync", PathArgs: []string{"invoice_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.InvoiceIntegrationSync(ctx, args[0], nil, editors...)
		}},
		{Group: "integrations", Use: "invoice-sync-events", Short: "List invoice integration sync events", PathArgs: []string{"invoice_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.InvoiceIntegrationSyncEvents(ctx, args[0], nil, editors...)
		}},
		{Group: "invoices", Use: "list", Short: "List invoices", QueryFlags: []string{"business_id", "job_id", "invoice_number", "customer_id", "customer_email", "status", "sort", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ListInvoice(ctx, nil, editors...)
		}},
		{Group: "invoices", Use: "get", Short: "Get an invoice", PathArgs: []string{"invoice_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.GetInvoice(ctx, args[0], nil, editors...)
		}},
		{Group: "invoices", Use: "pdf", Short: "Get invoice PDF metadata", PathArgs: []string{"invoice_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.Pdf(ctx, args[0], nil, editors...)
		}},
		{Group: "job-reports", Use: "list", Short: "List job reports", QueryFlags: []string{"business_id", "job_id", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ListJobReport(ctx, nil, editors...)
		}},
		{Group: "job-reports", Use: "list-for-job", Short: "List reports for a job", PathArgs: []string{"job_id"}, QueryFlags: []string{"business_id", "job_id", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ListJobReportForJob(ctx, args[0], nil, editors...)
		}},
		{Group: "job-reports", Use: "get", Short: "Get a job report", PathArgs: []string{"job_report_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.GetJobReport(ctx, args[0], nil, editors...)
		}},
		{Group: "job-reports", Use: "get-for-job", Short: "Get a report for a job", PathArgs: []string{"job_id", "job_report_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.GetJobReportForJob(ctx, args[0], args[1], nil, editors...)
		}},
		{Group: "jobs", Use: "list", Short: "List jobs", QueryFlags: []string{"business_id", "job_number", "customer_id", "customer_email", "accepted_status", "status_id", "sort", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.Jobs(ctx, nil, editors...)
		}},
		{Group: "jobs", Use: "get", Short: "Get a job", PathArgs: []string{"job_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.Job(ctx, args[0], nil, editors...)
		}},
		{Group: "jobs", Use: "part-authorisations", Short: "List part authorisations for a job", PathArgs: []string{"job_id"}, QueryFlags: []string{"limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.PartAuthorisations(ctx, args[0], nil, editors...)
		}},
		{Group: "payments", Use: "list", Short: "List payments", QueryFlags: []string{"business_id", "invoice_id", "sort", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ListPayment(ctx, nil, editors...)
		}},
		{Group: "payments", Use: "get", Short: "Get a payment", PathArgs: []string{"payment_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.GetPayment(ctx, args[0], nil, editors...)
		}},
		{Group: "services", Use: "list", Short: "List services", QueryFlags: []string{"business_id", "name", "sort", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ListService(ctx, nil, editors...)
		}},
		{Group: "services", Use: "get", Short: "Get a service", PathArgs: []string{"service_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.GetService(ctx, args[0], nil, editors...)
		}},
		{Group: "stock", Use: "list", Short: "List stock variations", QueryFlags: []string{"business_id", "query", "sku", "ean", "barcode", "sort", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ListStockVariation(ctx, nil, editors...)
		}},
		{Group: "stock", Use: "get", Short: "Get a stock variation", PathArgs: []string{"stock_variation_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.GetStockVariation(ctx, args[0], nil, editors...)
		}},
		{Group: "webhook-deliveries", Use: "list-for-endpoint", Short: "List deliveries for a webhook endpoint", PathArgs: []string{"endpoint_id"}, QueryFlags: []string{"status", "event_type", "limit", "cursor"}, Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ListForEndpoint(ctx, args[0], nil, editors...)
		}},
		{Group: "webhook-deliveries", Use: "get", Short: "Get a webhook delivery", PathArgs: []string{"delivery_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.GetWebhookDelivery(ctx, args[0], nil, editors...)
		}},
		{Group: "webhook-endpoints", Use: "list", Short: "List webhook endpoints", Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ListWebhookEndpoint(ctx, nil, editors...)
		}},
		{Group: "webhook-endpoints", Use: "get", Short: "Get a webhook endpoint", PathArgs: []string{"endpoint_id"}, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.GetWebhookEndpoint(ctx, args[0], nil, editors...)
		}},
		{Group: "webhook-events", Use: "list", Short: "List webhook event types", Pageable: true, ExecutePage: func(ctx context.Context, c *api.Client, args []string, editors []api.RequestEditorFn) (*http.Response, error) {
			return c.ListWebhookEvent(ctx, nil, editors...)
		}},
	}
}

func readGroupShort(group string) string {
	switch group {
	case "back-orders":
		return "Read back order resources"
	case "integrations":
		return "Read integration resources"
	case "invoice-items":
		return "Read invoice item resources"
	case "job-reports":
		return "Read job report resources"
	case "webhook-deliveries":
		return "Read webhook delivery resources"
	case "webhook-endpoints":
		return "Read webhook endpoint resources"
	case "webhook-events":
		return "Read webhook event resources"
	default:
		return "Read " + group + " resources"
	}
}

func kebab(value string) string {
	return strings.ReplaceAll(value, "_", "-")
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
