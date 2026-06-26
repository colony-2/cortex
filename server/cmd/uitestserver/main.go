package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/colony-2/colony2/server/internal/cortex"
)

func main() {
	var cfg cortex.Config
	var cors string
	flag.StringVar(&cfg.Addr, "addr", cortexEnv("CORTEX_ADDR", ":8081"), "listen address")
	flag.StringVar(&cfg.WorkingDir, "working-dir", cortexEnv("CORTEX_WORKING_DIR", "."), "cell working directory")
	flag.StringVar(&cfg.DefaultTenantID, "tenant-id", cortexEnv("CORTEX_TENANT_ID", "1"), "default JobDB tenant ID")
	flag.StringVar(&cors, "cors-origins", cortexEnv("CORTEX_CORS_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"), "comma-separated CORS origins")
	flag.Parse()
	cfg.CORSOrigins = cortex.SplitCSV(cors)

	srv, err := cortex.NewUITestServer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("cortex ui test api listening on %s", cfg.Addr)
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
