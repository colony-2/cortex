package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/colony-2/colony2/server/internal/cortex"
	"github.com/colony-2/colony2/server/internal/webdist"
)

// Set by the release workflow through Go linker flags.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Printf("cortex version %s\n", version)
		return
	}

	var cfg cortex.Config
	var cors string
	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.StringVar(&cfg.Addr, "addr", cortexEnv("CORTEX_ADDR", ":8080"), "listen address")
	flag.StringVar(&cfg.JobDBURL, "jobdb-url", cortexEnv("JOBDB_URL", ""), "remote JobDB API URL")
	flag.StringVar(&cfg.WorkingDir, "working-dir", cortexEnv("CORTEX_WORKING_DIR", "."), "cell working directory")
	flag.StringVar(&cfg.DefaultTenantID, "tenant-id", cortexEnv("CORTEX_TENANT_ID", "1"), "default JobDB tenant ID")
	flag.StringVar(&cors, "cors-origins", cortexEnv("CORTEX_CORS_ORIGINS", ""), "comma-separated CORS origins")
	flag.Parse()
	if showVersion {
		fmt.Printf("cortex version %s\n", version)
		return
	}
	cfg.CORSOrigins = cortex.SplitCSV(cors)
	cfg.StaticFS = webdist.FS()

	srv, err := cortex.NewRemoteServer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("cortex listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, srv); err != nil {
		log.Fatal(err)
	}
}

func cortexEnv(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
