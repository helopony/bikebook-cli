package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

const defaultAPIBase = "https://api.bikebook.com/public/v1"

var (
	version = "dev"
	commit  = "none"
	apiBase = defaultAPIBase
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

func Execute() error {
	return NewRootCommand().Execute()
}

func NewRootCommand() *cobra.Command {
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
		Version:       versionString(),
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

	cmd.AddCommand(newVersionCommand())

	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), versionString())
			return err
		},
	}
}

func versionString() string {
	return fmt.Sprintf("bikebook %s (commit %s, api_base %s)", version, commit, apiBase)
}

func SetOutput(cmd *cobra.Command, out, errOut io.Writer) {
	cmd.SetOut(out)
	cmd.SetErr(errOut)
}
