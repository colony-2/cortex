package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/colony-2/colony2/server/internal/cortex"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
)

func main() {
	var cfg cortex.Config
	var cors string
	var jobdbOnly bool
	flag.BoolVar(&jobdbOnly, "jobdb-only", false, "serve the toy JobDB HTTP API for production integration tests")
	flag.StringVar(&cfg.Addr, "addr", cortexEnv("CORTEX_ADDR", ":8081"), "listen address")
	flag.StringVar(&cfg.WorkingDir, "working-dir", cortexEnv("CORTEX_WORKING_DIR", "."), "cell working directory")
	flag.StringVar(&cfg.DefaultTenantID, "tenant-id", cortexEnv("CORTEX_TENANT_ID", "1"), "default JobDB tenant ID")
	flag.StringVar(&cors, "cors-origins", cortexEnv("CORTEX_CORS_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"), "comma-separated CORS origins")
	flag.Parse()
	cfg.CORSOrigins = cortex.SplitCSV(cors)

	var srv http.Handler
	if jobdbOnly {
		srv = remote.NewServer(toy.New())
	} else {
		var err error
		srv, err = cortex.NewUITestServer(cfg)
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("cortex test server listening on %s (jobdb-only=%t)", cfg.Addr, jobdbOnly)
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
