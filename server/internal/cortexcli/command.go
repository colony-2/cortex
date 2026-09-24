// Package cortexcli shares the production and development Cortex command line.
package cortexcli

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"

	"github.com/colony-2/colony2/server/internal/cortex"
	"github.com/spf13/cobra"
)

type Options struct {
	Version     string
	StaticFS    fs.FS
	DefaultCORS string
}

func NewCommand(opts Options) *cobra.Command { return newCommand(opts, serve) }

func newCommand(opts Options, run func(*cobra.Command, cortex.Config) error) *cobra.Command {
	if opts.Version == "" {
		opts.Version = "dev"
	}
	var cfg cortex.Config
	var jobdb, legacy, cors string
	root := &cobra.Command{
		Use: "cortex", Short: "Web interface for c2j jobs", Version: opts.Version,
		SilenceUsage: true, SilenceErrors: true, Args: cobra.NoArgs,
	}
	root.SetVersionTemplate("cortex version {{.Version}}\n")
	flags := root.PersistentFlags()
	flags.StringVar(&jobdb, "jobdb", "", "JobDB URI (http(s)://host/tenant); defaults to C2J_JOBDB or .c2j/config.yaml")
	flags.StringVar(&cfg.Addr, "addr", envOr("CORTEX_ADDR", ":8080"), "Listen address")
	flags.StringVar(&cfg.WorkingDir, "working-dir", envOr("CORTEX_WORKING_DIR", "."), "Directory for c2j config discovery")
	flags.StringVar(&cors, "cors-origins", envOr("CORTEX_CORS_ORIGINS", opts.DefaultCORS), "Comma-separated CORS origins")
	flags.StringVar(&legacy, "jobdb-url", "", "Legacy JobDB server URL or URI")
	flags.StringVar(&cfg.DefaultTenantID, "tenant-id", envOr("CORTEX_TENANT_ID", ""), "Legacy tenant for a server-only JobDB URL")
	_ = flags.MarkDeprecated("jobdb-url", "use --jobdb http(s)://host/tenant")
	_ = flags.MarkDeprecated("tenant-id", "include the tenant in --jobdb http(s)://host/tenant")
	root.MarkFlagsMutuallyExclusive("jobdb", "jobdb-url")
	root.MarkFlagsMutuallyExclusive("jobdb", "tenant-id")
	start := func(cmd *cobra.Command, args []string) error {
		resolved, err := resolveConnection(cmd.Context(), cfg, jobdb, legacy)
		if err != nil {
			return err
		}
		resolved.CORSOrigins = cortex.SplitCSV(cors)
		resolved.StaticFS = opts.StaticFS
		return run(cmd, resolved)
	}
	root.RunE = start
	root.AddCommand(&cobra.Command{Use: "serve", Short: "Serve the Cortex API and UI", Args: cobra.NoArgs, RunE: start})
	root.AddCommand(&cobra.Command{
		Use: "version", Short: "Print Cortex build information", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "cortex version %s\n", opts.Version)
			return err
		},
	})
	return root
}

func serve(cmd *cobra.Command, cfg cortex.Config) error {
	srv, err := cortex.NewRemoteServer(cfg)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "cortex listening on %s (tenant %s)\n", cfg.Addr, cfg.DefaultTenantID)
	return http.ListenAndServe(cfg.Addr, srv)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
