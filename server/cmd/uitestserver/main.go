package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/colony-2/colony2/server/internal/cortex"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	sqliteruntime "github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
)

func main() {
	var cfg cortex.Config
	var cors string
	var jobdbOnly bool
	var sqlite bool
	flag.BoolVar(&jobdbOnly, "jobdb-only", false, "serve a test JobDB HTTP API for production integration tests")
	flag.BoolVar(&sqlite, "sqlite", false, "use a temporary SQLite JobDB with --jobdb-only for worker integration tests")
	flag.StringVar(&cfg.Addr, "addr", cortexEnv("CORTEX_ADDR", ":8081"), "listen address")
	flag.StringVar(&cfg.WorkingDir, "working-dir", cortexEnv("CORTEX_WORKING_DIR", "."), "cell working directory")
	flag.StringVar(&cfg.DefaultTenantID, "tenant-id", cortexEnv("CORTEX_TENANT_ID", "1"), "default JobDB tenant ID")
	flag.StringVar(&cors, "cors-origins", cortexEnv("CORTEX_CORS_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"), "comma-separated CORS origins")
	flag.Parse()
	if sqlite && !jobdbOnly {
		log.Fatal("--sqlite requires --jobdb-only")
	}
	cfg.CORSOrigins = cortex.SplitCSV(cors)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var srv http.Handler
	if jobdbOnly {
		if sqlite {
			runtime, err := sqliteruntime.StartEmbeddedRuntime(ctx)
			if err != nil {
				log.Fatal(err)
			}
			defer runtime.Shutdown()
			srv = remote.NewServer(runtime.Runtime)
		} else {
			srv = remote.NewServer(toy.New())
		}
	} else {
		var err error
		srv, err = cortex.NewUITestServer(cfg)
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("cortex test server listening on %s (jobdb-only=%t)", cfg.Addr, jobdbOnly)
	server := &http.Server{Addr: cfg.Addr, Handler: srv}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func cortexEnv(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
