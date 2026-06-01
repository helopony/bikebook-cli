package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/helopony/bikebook-cli/internal/api"
	"golang.org/x/term"
)

const (
	ExitSuccess = iota
	ExitGeneric
	ExitUsage
	ExitValidation
	ExitAuthentication
	ExitForbidden
	ExitNotFound
	ExitConflict
	ExitRateLimited
	ExitNetwork
)

type OutputMode string

const (
	OutputHuman OutputMode = "human"
	OutputJSON  OutputMode = "json"
	OutputRaw   OutputMode = "raw"
)

type Contract struct {
	OutputMode     OutputMode
	Quiet          bool
	NoColor        bool
	NonInteractive bool
	Debug          bool
	APIBase        string
	APIKey         string
	Profile        string
	Env            string
	RequestID      string
	IdempotencyKey string
}

type CLIError struct {
	Envelope   api.ErrorResponse
	Exit       int
	Status     int
	Hint       string
	DocsURL    string
	localCode  string
	localParam string
}

type SilentExitError struct {
	Code int
}

func (e SilentExitError) Error() string {
	return "silent exit"
}

type cliErrorContext struct {
	ExitCode   int     `json:"exit_code"`
	HTTPStatus *int    `json:"http_status,omitempty"`
	Hint       *string `json:"hint,omitempty"`
	DocsURL    *string `json:"docs_url,omitempty"`
}

type renderedError struct {
	Error     *api.Error      `json:"error,omitempty"`
	RequestID *string         `json:"request_id,omitempty"`
	CLI       cliErrorContext `json:"cli"`
}

func (e *CLIError) Error() string {
	if e.Envelope.Error != nil && e.Envelope.Error.Message != nil {
		return *e.Envelope.Error.Message
	}
	if e.localCode != "" {
		return e.localCode
	}
	return "unknown error"
}

func NewAPIError(status int, envelope api.ErrorResponse, hint, docsURL string) *CLIError {
	return &CLIError{
		Envelope: envelope,
		Exit:     ExitCodeForHTTPStatus(status),
		Status:   status,
		Hint:     hint,
		DocsURL:  docsURL,
	}
}

func NewLocalError(exit int, code, message, parameter, hint, docsURL string) *CLIError {
	err := api.Error{
		Code:    errorCodePtr(code),
		Message: stringPtr(message),
	}
	if parameter != "" {
		err.Parameter = stringPtr(parameter)
	}

	return &CLIError{
		Envelope: api.ErrorResponse{
			Error: &err,
		},
		Exit:       exit,
		Hint:       hint,
		DocsURL:    docsURL,
		localCode:  code,
		localParam: parameter,
	}
}

func ExitCodeForHTTPStatus(status int) int {
	switch status {
	case 400:
		return ExitValidation
	case 401:
		return ExitAuthentication
	case 403:
		return ExitForbidden
	case 404:
		return ExitNotFound
	case 409:
		return ExitConflict
	case 429:
		return ExitRateLimited
	default:
		if status >= 500 {
			return ExitNetwork
		}
		return ExitGeneric
	}
}

func ResolveOutputMode(jsonFlag, rawFlag, stdoutIsTTY bool) OutputMode {
	switch {
	case rawFlag:
		return OutputRaw
	case jsonFlag:
		return OutputJSON
	case stdoutIsTTY:
		return OutputHuman
	default:
		return OutputRaw
	}
}

func contractFromOptions(opts rootOptions, stdout io.Writer) Contract {
	return Contract{
		OutputMode:     ResolveOutputMode(opts.json, opts.raw, isTerminal(stdout)),
		Quiet:          opts.quiet,
		NoColor:        opts.noColor || envEnabled("NO_COLOR"),
		NonInteractive: envEnabled("BIKEBOOK_NON_INTERACTIVE"),
		Debug:          opts.debug,
		APIBase:        opts.apiBase,
		APIKey:         opts.apiKey,
		Profile:        opts.profile,
		Env:            opts.env,
		RequestID:      opts.requestID,
		IdempotencyKey: opts.idempotencyKey,
	}
}

func envEnabled(name string) bool {
	value, ok := os.LookupEnv(name)
	if !ok {
		return false
	}
	value = strings.TrimSpace(strings.ToLower(value))
	return value != "" && value != "0" && value != "false"
}

func isTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func RenderData(w io.Writer, contract Contract, data any) error {
	switch contract.OutputMode {
	case OutputHuman:
		return renderHuman(w, data)
	case OutputJSON:
		return renderJSON(w, data, true)
	case OutputRaw:
		return renderRaw(w, data)
	default:
		return renderJSON(w, data, false)
	}
}

func RenderError(w io.Writer, contract Contract, err error) int {
	if err == nil {
		return ExitSuccess
	}
	var silent SilentExitError
	if errors.As(err, &silent) {
		return silent.Code
	}

	cliErr := normalizeError(err)
	payload := errorPayload(cliErr)

	var renderErr error
	if contract.OutputMode == OutputHuman {
		renderErr = renderHumanError(w, payload)
	} else {
		renderErr = renderJSON(w, payload, contract.OutputMode == OutputJSON)
	}
	if renderErr != nil {
		_, _ = fmt.Fprintf(w, "error: %v\n", renderErr)
		return ExitGeneric
	}

	return cliErr.Exit
}

func normalizeError(err error) *CLIError {
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		return cliErr
	}

	return NewLocalError(ExitUsage, "cli_invalid_input", err.Error(), "", "", "")
}

func errorPayload(err *CLIError) renderedError {
	payload := renderedError{
		Error:     err.Envelope.Error,
		RequestID: err.Envelope.RequestId,
		CLI: cliErrorContext{
			ExitCode: err.Exit,
		},
	}
	if err.Status != 0 {
		payload.CLI.HTTPStatus = &err.Status
	}
	if err.Hint != "" {
		payload.CLI.Hint = &err.Hint
	}
	if err.DocsURL != "" {
		payload.CLI.DocsURL = &err.DocsURL
	}
	return payload
}

func renderJSON(w io.Writer, data any, pretty bool) error {
	var (
		bytes []byte
		err   error
	)
	if pretty {
		bytes, err = json.MarshalIndent(data, "", "  ")
	} else {
		bytes, err = json.Marshal(data)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(bytes))
	return err
}

func renderRaw(w io.Writer, data any) error {
	rows, ok := rowsFromData(data)
	if !ok {
		return renderJSON(w, data, false)
	}

	for _, row := range rows {
		if err := renderJSON(w, row, false); err != nil {
			return err
		}
	}
	return nil
}

func renderHuman(w io.Writer, data any) error {
	switch typed := data.(type) {
	case string:
		_, err := fmt.Fprintln(w, typed)
		return err
	case fmt.Stringer:
		_, err := fmt.Fprintln(w, typed.String())
		return err
	}

	rows, ok := rowsFromData(data)
	if !ok || len(rows) == 0 {
		return renderJSON(w, data, true)
	}

	columns := orderedColumns(rows)
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, strings.ToUpper(strings.Join(columns, "\t"))); err != nil {
		return err
	}
	for _, row := range rows {
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			values = append(values, humanValue(row[column]))
		}
		if _, err := fmt.Fprintln(table, strings.Join(values, "\t")); err != nil {
			return err
		}
	}
	return table.Flush()
}

func renderHumanError(w io.Writer, payload renderedError) error {
	message := "unknown error"
	if payload.Error != nil && payload.Error.Message != nil {
		message = *payload.Error.Message
	}
	if _, err := fmt.Fprintf(w, "Error: %s\n", message); err != nil {
		return err
	}
	if payload.Error != nil && payload.Error.Code != nil {
		if _, err := fmt.Fprintf(w, "Code: %s\n", *payload.Error.Code); err != nil {
			return err
		}
	}
	if payload.RequestID != nil {
		if _, err := fmt.Fprintf(w, "Request ID: %s\n", *payload.RequestID); err != nil {
			return err
		}
	}
	if payload.CLI.Hint != nil {
		if _, err := fmt.Fprintf(w, "Hint: %s\n", *payload.CLI.Hint); err != nil {
			return err
		}
	}
	return nil
}

func rowsFromData(data any) ([]map[string]any, bool) {
	if envelope, ok := data.(map[string]any); ok {
		if nested, ok := envelope["data"]; ok {
			return rowsFromData(nested)
		}
	}

	value := reflect.ValueOf(data)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Slice && value.Kind() != reflect.Array {
		return nil, false
	}

	rows := make([]map[string]any, 0, value.Len())
	for i := 0; i < value.Len(); i++ {
		row, ok := mapFromValue(value.Index(i).Interface())
		if !ok {
			return nil, false
		}
		rows = append(rows, row)
	}
	return rows, true
}

func mapFromValue(value any) (map[string]any, bool) {
	if row, ok := value.(map[string]any); ok {
		return row, true
	}

	bytes, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	row := map[string]any{}
	if err := json.Unmarshal(bytes, &row); err != nil {
		return nil, false
	}
	return row, true
}

func orderedColumns(rows []map[string]any) []string {
	seen := map[string]bool{}
	for _, row := range rows {
		for column := range row {
			seen[column] = true
		}
	}

	priority := []string{"id", "object", "name", "status", "created_at", "updated_at"}
	columns := make([]string, 0, len(seen))
	for _, column := range priority {
		if seen[column] {
			columns = append(columns, column)
			delete(seen, column)
		}
	}

	rest := make([]string, 0, len(seen))
	for column := range seen {
		rest = append(rest, column)
	}
	sort.Strings(rest)
	return append(columns, rest...)
}

func humanValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		bytes, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(bytes)
	}
}

func stringPtr(value string) *string {
	return &value
}

func errorCodePtr(value string) *api.ErrorCode {
	code := api.ErrorCode(value)
	return &code
}
