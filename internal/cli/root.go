package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

const defaultAPIBase = "https://api.bikebook.com/public/v1"

var (
	version = "dev"
	commit  = "none"
)

type rootOptions struct {
	json           bool
	raw            bool
	quiet          bool
	noColor        bool
	apiBase        string
	apiKey         string
	env            string
	requestID      string
	idempotencyKey string
	debug          bool
}

// SetVersionInfo is used by tests and build-time ldflags to override version metadata.
func SetVersionInfo(v, c string) {
	version = v
	commit = c
}

func Execute() int {
	return ExecuteWithArgs(os.Args[1:], os.Stdout, os.Stderr)
}

func ExecuteWithArgs(args []string, out, errOut io.Writer) int {
	cmd, opts := newRootCommand()
	SetOutput(cmd, out, errOut)
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		contract := contractFromOptions(*opts, out)
		return RenderError(errOut, contract, err)
	}
	return ExitSuccess
}

func NewRootCommand() *cobra.Command {
	cmd, _ := newRootCommand()
	return cmd
}

func newRootCommand() (*cobra.Command, *rootOptions) {
	opts := rootOptions{
		apiBase: defaultAPIBase,
		env:     "auto",
	}

	cmd := &cobra.Command{
		Use:           "bikebook",
		Short:         "Agent-first CLI for the BikeBook Workshop API",
		Long:          "bikebook is an agent-first CLI for the BikeBook Workshop Public API.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       versionString(defaultAPIBase),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.SetVersionTemplate("{{.Version}}\n")

	flags := cmd.PersistentFlags()
	flags.BoolVar(&opts.json, "json", false, "emit pretty JSON on stdout")
	flags.BoolVar(&opts.raw, "raw", false, "emit compact JSON or NDJSON on stdout")
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "suppress diagnostics on stderr")
	flags.BoolVar(&opts.noColor, "no-color", false, "disable ANSI color output")
	flags.StringVar(&opts.apiBase, "api-base", defaultAPIBase, "BikeBook API base URL")
	flags.StringVar(&opts.apiKey, "api-key", "", "BikeBook API key")
	flags.StringVar(&opts.env, "env", "auto", "BikeBook environment: auto, live, or test")
	flags.StringVar(&opts.requestID, "request-id", "", "request correlation ID to send as X-Bikebook-Request-Id")
	flags.StringVar(&opts.idempotencyKey, "idempotency-key", "", "idempotency key for write requests")
	flags.BoolVar(&opts.debug, "debug", false, "write HTTP debug diagnostics to stderr")

	cmd.AddCommand(newVersionCommand(&opts))

	return cmd, &opts
}

func newVersionCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			contract := contractFromOptions(*opts, cmd.OutOrStdout())
			if contract.OutputMode == OutputHuman {
				return RenderData(cmd.OutOrStdout(), contract, versionString(contract.APIBase))
			}
			return RenderData(cmd.OutOrStdout(), contract, map[string]string{
				"version":  version,
				"commit":   commit,
				"api_base": contract.APIBase,
			})
		},
	}
}

func versionString(baseURL string) string {
	return "bikebook " + version + " (commit " + commit + ", api_base " + baseURL + ")"
}

func SetOutput(cmd *cobra.Command, out, errOut io.Writer) {
	cmd.SetOut(out)
	cmd.SetErr(errOut)
}
