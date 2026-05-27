package cli

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/helopony/bikebook-cli/internal/openapi"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type DescribeDocument struct {
	SchemaVersion string                  `json:"schema_version"`
	GeneratedFrom DescribeGeneratedFrom   `json:"generated_from"`
	ExitCodes     []DescribeExitCode      `json:"exit_codes"`
	Commands      []DescribeCommand       `json:"commands"`
	OpenAPI       DescribeOpenAPISnapshot `json:"openapi"`
}

type DescribeGeneratedFrom struct {
	OpenAPI string `json:"openapi"`
	Cobra   string `json:"cobra"`
}

type DescribeExitCode struct {
	Code    int    `json:"code"`
	Meaning string `json:"meaning"`
}

type DescribeCommand struct {
	Name                 string               `json:"name"`
	Path                 []string             `json:"path"`
	Use                  string               `json:"use"`
	Short                string               `json:"short,omitempty"`
	Args                 []DescribeArg        `json:"args"`
	Flags                []DescribeFlag       `json:"flags"`
	AcceptedInputFields  []DescribeInputField `json:"accepted_input_fields"`
	ExpectedOutputSchema any                  `json:"expected_output_schema,omitempty"`
	ExitCodes            []int                `json:"exit_codes"`
	Example              DescribeExample      `json:"example"`
}

type DescribeArg struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

type DescribeFlag struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

type DescribeInputField struct {
	Name        string `json:"name"`
	Location    string `json:"location"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
	Schema      any    `json:"schema,omitempty"`
}

type DescribeExample struct {
	Command string `json:"command"`
}

type DescribeOpenAPISnapshot struct {
	Title      string                     `json:"title"`
	Version    string                     `json:"version"`
	Operations []DescribeOpenAPIOperation `json:"operations"`
}

type DescribeOpenAPIOperation struct {
	OperationID string               `json:"operation_id"`
	Method      string               `json:"method"`
	Path        string               `json:"path"`
	Tags        []string             `json:"tags,omitempty"`
	Summary     string               `json:"summary,omitempty"`
	Inputs      []DescribeInputField `json:"inputs"`
	Output      any                  `json:"output,omitempty"`
	ExitCodes   []int                `json:"exit_codes"`
}

type openAPISpec struct {
	Info  openAPIInfo                            `json:"info"`
	Paths map[string]map[string]openAPIOperation `json:"paths"`
}

type openAPIInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type openAPIOperation struct {
	OperationID string                     `json:"operationId"`
	Tags        []string                   `json:"tags"`
	Summary     string                     `json:"summary"`
	Parameters  []openAPIParameter         `json:"parameters"`
	RequestBody *openAPIRequestBody        `json:"requestBody"`
	Responses   map[string]openAPIResponse `json:"responses"`
}

type openAPIParameter struct {
	Name        string `json:"name"`
	In          string `json:"in"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Schema      any    `json:"schema"`
}

type openAPIRequestBody struct {
	Required bool                          `json:"required"`
	Content  map[string]openAPIContentType `json:"content"`
}

type openAPIResponse struct {
	Description string                        `json:"description"`
	Content     map[string]openAPIContentType `json:"content"`
}

type openAPIContentType struct {
	Schema any `json:"schema"`
}

func newDescribeCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "describe [command]",
		Short: "Emit machine-readable CLI schema",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			contract := contractFromOptions(*opts, cmd.OutOrStdout())
			root := cmd.Root()
			doc, err := BuildDescribeDocument(root)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				return RenderData(cmd.OutOrStdout(), contract, doc)
			}
			needle := strings.Join(args, " ")
			for _, command := range doc.Commands {
				if command.Name == needle || commandNameWithoutRoot(command) == needle {
					return RenderData(cmd.OutOrStdout(), contract, command)
				}
			}
			return NewLocalError(ExitUsage, "cli_unknown_command", "command not found in describe schema", "command", "run `bikebook describe` to list available commands", "")
		},
	}
}

func commandNameWithoutRoot(command DescribeCommand) string {
	if len(command.Path) <= 1 {
		return command.Name
	}
	return strings.Join(command.Path[1:], " ")
}

func BuildDescribeDocument(root *cobra.Command) (DescribeDocument, error) {
	apiSnapshot, err := buildOpenAPISnapshot()
	if err != nil {
		return DescribeDocument{}, err
	}

	return DescribeDocument{
		SchemaVersion: "1",
		GeneratedFrom: DescribeGeneratedFrom{
			OpenAPI: "public-v1.json",
			Cobra:   "registered command tree",
		},
		ExitCodes: describeExitCodes(),
		Commands:  describeCommands(root),
		OpenAPI:   apiSnapshot,
	}, nil
}

func describeCommands(root *cobra.Command) []DescribeCommand {
	commands := []DescribeCommand{}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Hidden {
			return
		}
		if cmd.Name() == "help" || cmd.Name() == "completion" {
			return
		}
		commands = append(commands, describeCommand(root, cmd))
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})
	return commands
}

func describeCommand(root, cmd *cobra.Command) DescribeCommand {
	path := commandPath(root, cmd)
	return DescribeCommand{
		Name:                 strings.Join(path, " "),
		Path:                 path,
		Use:                  cmd.UseLine(),
		Short:                cmd.Short,
		Args:                 describeArgs(cmd),
		Flags:                describeFlags(cmd),
		AcceptedInputFields:  describeCommandInputs(cmd),
		ExpectedOutputSchema: describeCommandOutput(cmd),
		ExitCodes:            []int{ExitSuccess, ExitGeneric, ExitUsage, ExitValidation, ExitAuthentication, ExitForbidden, ExitNotFound, ExitConflict, ExitRateLimited, ExitNetwork},
		Example:              DescribeExample{Command: exampleCommand(path)},
	}
}

func commandPath(root, cmd *cobra.Command) []string {
	pieces := []string{}
	for current := cmd; current != nil; current = current.Parent() {
		if current == root {
			pieces = append([]string{root.Name()}, pieces...)
			break
		}
		pieces = append([]string{current.Name()}, pieces...)
	}
	return pieces
}

func describeArgs(cmd *cobra.Command) []DescribeArg {
	args := []DescribeArg{}
	for _, token := range strings.Fields(cmd.Use) {
		if strings.HasPrefix(token, "<") && strings.HasSuffix(token, ">") {
			args = append(args, DescribeArg{Name: strings.Trim(token, "<>"), Required: true})
		}
		if strings.HasPrefix(token, "[") && strings.HasSuffix(token, "]") {
			args = append(args, DescribeArg{Name: strings.Trim(token, "[]"), Required: false})
		}
	}
	return args
}

func describeFlags(cmd *cobra.Command) []DescribeFlag {
	flagsByName := map[string]DescribeFlag{}
	visitFlagSet(cmd.InheritedFlags(), flagsByName)
	visitFlagSet(cmd.Flags(), flagsByName)

	flags := make([]DescribeFlag, 0, len(flagsByName))
	for _, flag := range flagsByName {
		flags = append(flags, flag)
	}
	sort.Slice(flags, func(i, j int) bool {
		return flags[i].Name < flags[j].Name
	})
	return flags
}

func visitFlagSet(flags *pflag.FlagSet, out map[string]DescribeFlag) {
	flags.VisitAll(func(flag *pflag.Flag) {
		out[flag.Name] = DescribeFlag{
			Name:        flag.Name,
			Shorthand:   flag.Shorthand,
			Type:        flag.Value.Type(),
			Required:    flag.Annotations[cobra.BashCompOneRequiredFlag] != nil,
			Default:     flag.DefValue,
			Description: flag.Usage,
		}
	})
}

func describeCommandInputs(cmd *cobra.Command) []DescribeInputField {
	if cmd.Name() == "config" || commandHasAncestor(cmd, "config") {
		return []DescribeInputField{{Name: "api_key", Location: "stdin/env", Required: false, Description: "API key for config set api-key. Prefer stdin or BIKEBOOK_API_KEY."}}
	}
	if cmd.Name() == "raw" {
		return []DescribeInputField{
			{Name: "method", Location: "argument", Required: true, Description: "HTTP method to send to BikeBook."},
			{Name: "path", Location: "argument", Required: true, Description: "API path relative to --api-base, including optional query string."},
			{Name: "data", Location: "flag/stdin", Required: false, Description: "Request body from --data @file, --data -, literal --data, or stdin for write methods."},
			{Name: "header", Location: "flag", Required: false, Description: "Additional request header, repeatable."},
		}
	}
	return nil
}

func describeCommandOutput(cmd *cobra.Command) any {
	switch cmd.Name() {
	case "describe":
		return map[string]string{"type": "DescribeDocument or DescribeCommand"}
	case "doctor":
		return map[string]string{"type": "array", "items": "DoctorCheck"}
	case "raw":
		return map[string]string{"type": "upstream API response"}
	case "upgrade":
		return map[string]string{"type": "UpgradeResult"}
	case "version":
		return map[string]string{"type": "VersionInfo"}
	default:
		return nil
	}
}

func exampleCommand(path []string) string {
	if len(path) == 0 {
		return "bikebook --help"
	}
	switch strings.Join(path, " ") {
	case "bikebook config set":
		return "printf %s \"$BIKEBOOK_API_KEY\" | bikebook config set api-key"
	case "bikebook describe":
		return "bikebook describe version"
	case "bikebook doctor":
		return "bikebook doctor --json"
	default:
		return strings.Join(path, " ") + " --help"
	}
}

func buildOpenAPISnapshot() (DescribeOpenAPISnapshot, error) {
	var spec openAPISpec
	if err := json.Unmarshal([]byte(openapi.PublicV1JSON), &spec); err != nil {
		return DescribeOpenAPISnapshot{}, err
	}
	operations := []DescribeOpenAPIOperation{}
	for path, methods := range spec.Paths {
		for method, operation := range methods {
			if !isHTTPMethod(method) {
				continue
			}
			operations = append(operations, DescribeOpenAPIOperation{
				OperationID: operation.OperationID,
				Method:      strings.ToUpper(method),
				Path:        path,
				Tags:        operation.Tags,
				Summary:     operation.Summary,
				Inputs:      operationInputs(operation),
				Output:      responseSchema(operation),
				ExitCodes:   []int{ExitSuccess, ExitValidation, ExitAuthentication, ExitForbidden, ExitNotFound, ExitConflict, ExitRateLimited, ExitNetwork},
			})
		}
	}
	sort.Slice(operations, func(i, j int) bool {
		if operations[i].Path == operations[j].Path {
			return operations[i].Method < operations[j].Method
		}
		return operations[i].Path < operations[j].Path
	})
	return DescribeOpenAPISnapshot{Title: spec.Info.Title, Version: spec.Info.Version, Operations: operations}, nil
}

func operationInputs(operation openAPIOperation) []DescribeInputField {
	inputs := make([]DescribeInputField, 0, len(operation.Parameters)+1)
	for _, parameter := range operation.Parameters {
		inputs = append(inputs, DescribeInputField{
			Name:        parameter.Name,
			Location:    parameter.In,
			Required:    parameter.Required,
			Description: parameter.Description,
			Schema:      parameter.Schema,
		})
	}
	if operation.RequestBody != nil {
		inputs = append(inputs, DescribeInputField{
			Name:     "body",
			Location: "body",
			Required: operation.RequestBody.Required,
			Schema:   contentSchema(operation.RequestBody.Content),
		})
	}
	return inputs
}

func responseSchema(operation openAPIOperation) any {
	if response, ok := operation.Responses["200"]; ok {
		return contentSchema(response.Content)
	}
	if response, ok := operation.Responses["201"]; ok {
		return contentSchema(response.Content)
	}
	return nil
}

func contentSchema(content map[string]openAPIContentType) any {
	if content == nil {
		return nil
	}
	if jsonContent, ok := content["application/json"]; ok {
		return jsonContent.Schema
	}
	for _, value := range content {
		return value.Schema
	}
	return nil
}

func isHTTPMethod(method string) bool {
	switch strings.ToLower(method) {
	case "get", "post", "put", "patch", "delete":
		return true
	default:
		return false
	}
}

func describeExitCodes() []DescribeExitCode {
	return []DescribeExitCode{
		{Code: ExitSuccess, Meaning: "Success"},
		{Code: ExitGeneric, Meaning: "Generic error"},
		{Code: ExitUsage, Meaning: "Usage error"},
		{Code: ExitValidation, Meaning: "Validation or bad request"},
		{Code: ExitAuthentication, Meaning: "Authentication failed"},
		{Code: ExitForbidden, Meaning: "Forbidden"},
		{Code: ExitNotFound, Meaning: "Not found"},
		{Code: ExitConflict, Meaning: "Conflict"},
		{Code: ExitRateLimited, Meaning: "Rate limited"},
		{Code: ExitNetwork, Meaning: "Network or upstream failure"},
	}
}
