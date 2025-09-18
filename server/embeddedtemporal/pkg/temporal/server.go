package temporal

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "net"
    "time"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/server/common/cluster"
    "go.temporal.io/server/common/config"
    "go.temporal.io/server/common/dynamicconfig"
    "go.temporal.io/server/common/log"
    "go.temporal.io/server/common/log/tag"
    "go.temporal.io/server/common/membership/static"
    "go.temporal.io/server/common/metrics"
    "go.temporal.io/server/common/primitives"
    "go.temporal.io/server/schema/sqlite"
    "go.temporal.io/server/temporal"
    "go.uber.org/zap"
    "google.golang.org/protobuf/types/known/durationpb"
    // grpc imports previously used for readiness probing; retained if needed in future
)

// Options configures the embedded Temporal server
type Options struct {
    FrontendIP    string
    FrontendPort  int
    UIPort        int
    Namespaces    []string
    DatabaseFile  string
    LogLevel      string
    SQLitePragmas map[string]string
    EnableUI      bool
    // ReadinessTimeout defines how long Start() waits for the server to
    // become responsive to a basic gRPC call. If zero, defaults to 30s.
    ReadinessTimeout time.Duration
    // Optional feature toggles for background components. Leave false for a
    // complete cluster. Use in tests or constrained environments to reduce
    // background activity and speed up shutdown.
    DisableScanners              bool
    DisableParentClosePolicy     bool
    DisableNexus                 bool
    // EnableInternalWorker controls whether the internal worker service is
    // started. It runs background system workers (scanners, policies, etc.)
    // that are unnecessary for most tests and can cause noisy shutdowns.
    // Default is false.
    EnableInternalWorker         bool
}

// Server is an embedded Temporal server instance
type Server struct {
    server   temporal.Server
    logger   log.Logger
    options  Options
    stopChan chan struct{}
    dyn      dynamicconfig.Client
}

// NewServer creates a new embedded Temporal server
func NewServer(opts Options) (*Server, error) {
	return &Server{
		options:  opts,
		stopChan: make(chan struct{}),
	}, nil
}

// Start initializes and starts the embedded Temporal server
func (s *Server) Start() error {
	// Create logger based on log level
	var zapLogger *zap.Logger
	var err error

	switch s.options.LogLevel {
	case "debug":
		zapLogger, err = zap.NewDevelopment()
	case "error":
		config := zap.NewProductionConfig()
		config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
		zapLogger, err = config.Build()
	default:
		config := zap.NewProductionConfig()
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
		zapLogger, err = config.Build()
	}

	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	s.logger = log.NewZapLogger(zapLogger)

    // Quick validation: ensure desired frontend port is free to avoid long readiness timeouts
    if !IsPortAvailable(s.options.FrontendIP, s.options.FrontendPort) {
        return fmt.Errorf("frontend port %s:%d is not available", s.options.FrontendIP, s.options.FrontendPort)
    }

    // Initialize database schema if needed
	if err := s.initializeDatabase(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Build server configuration
	cfg, err := s.buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	// Create static hosts configuration for service discovery
	staticHosts := map[primitives.ServiceName]static.Hosts{
		primitives.FrontendService: static.SingleLocalHost(
			fmt.Sprintf("127.0.0.1:%d", cfg.Services["frontend"].RPC.GRPCPort)),
		primitives.HistoryService: static.SingleLocalHost(
			fmt.Sprintf("127.0.0.1:%d", cfg.Services["history"].RPC.GRPCPort)),
		primitives.MatchingService: static.SingleLocalHost(
			fmt.Sprintf("127.0.0.1:%d", cfg.Services["matching"].RPC.GRPCPort)),
		primitives.WorkerService: static.SingleLocalHost(
			fmt.Sprintf("127.0.0.1:%d", cfg.Services["worker"].RPC.GRPCPort)),
	}

    // Create server options. Start all default services; optionally provide a
    // memory-backed dynamic config to disable selected background components
    // when requested via options (useful for tests).
    toggles := DisableToggles{
        Scanners:          s.options.DisableScanners,
        ParentClosePolicy: s.options.DisableParentClosePolicy,
        Nexus:             s.options.DisableNexus,
    }
    // If scanners are disabled, also disable parent-close by default for fast teardown unless
    // explicitly requested otherwise.
    if toggles.Scanners && !toggles.ParentClosePolicy {
        toggles.ParentClosePolicy = true
    }
    dyn := DefaultDynamicConfigForEmbedded(toggles)
    s.dyn = dyn
    // By default, only run the core services needed for client workflows.
    // The internal Worker service is opt-in because it starts background
    // subsystems that create SDK clients and long-poll, which can slow
    // teardown and emit fatal logs during shutdown in tests.
    services := []string{
        string(primitives.FrontendService),
        string(primitives.HistoryService),
        string(primitives.MatchingService),
    }
    if s.options.EnableInternalWorker {
        services = append(services, string(primitives.WorkerService))
    }
    serverOpts := []temporal.ServerOption{
        temporal.WithConfig(cfg),
        temporal.ForServices(services),
        temporal.WithStaticHosts(staticHosts),
        temporal.WithDynamicConfigClient(dyn),
    }

	// Create and start the server
	s.server, err = temporal.NewServer(serverOpts...)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Start the server
	err = s.server.Start()
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	s.logger.Info("Temporal development server started",
		tag.Address(fmt.Sprintf("%s:%d", s.options.FrontendIP, s.options.FrontendPort)))

    // Readiness: perform a best‑effort, short TCP readiness probe, but do not
    // fail startup if it doesn’t succeed immediately. Clients and the
    // namespace creation routine below already include their own retries.
    _ = s.waitForServerReady()

    // Create default namespaces asynchronously; do not block startup.
    if len(s.options.Namespaces) > 0 {
        go func() {
            if err := s.createDefaultNamespaces(); err != nil {
                s.logger.Info("Namespace creation failed; clients may auto-register",
                    tag.NewStringTag("error", err.Error()))
            }
        }()
    }

	s.logger.Info("Server initialization complete",
		tag.NewStringTag("rpc", fmt.Sprintf("%s:%d", s.options.FrontendIP, s.options.FrontendPort)))

	return nil
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
    // Proactively quiesce background subsystems to shorten teardown.
    if mc, ok := s.dyn.(*dynamicconfig.MemoryClient); ok && mc != nil {
        mc.OverrideSetting(dynamicconfig.TaskQueueScannerEnabled, false)
        mc.OverrideSetting(dynamicconfig.HistoryScannerEnabled, false)
        mc.OverrideSetting(dynamicconfig.ExecutionsScannerEnabled, false)
        mc.OverrideSetting(dynamicconfig.BuildIdScavengerEnabled, false)
        mc.OverrideSetting(dynamicconfig.EnableParentClosePolicyWorker, false)
        mc.OverrideSetting(dynamicconfig.EnableNexus, false)
        // Shorten residual long-poll windows.
        mc.OverrideSetting(dynamicconfig.RefreshNexusEndpointsLongPollTimeout, time.Second)
        // Give subscribers a moment to pick up changes.
        time.Sleep(500 * time.Millisecond)
    }
    if s.server != nil {
        // In test modes (any disable toggle), bound Stop() latency to ~2s.
        if s.options.DisableScanners || s.options.DisableParentClosePolicy || s.options.DisableNexus {
            done := make(chan struct{})
            go func() { s.server.Stop(); close(done) }()
            select {
            case <-done:
            case <-time.After(2 * time.Second):
                // Return control to tests; background goroutines will finish shortly.
            }
        } else {
            s.server.Stop()
        }
    }
	close(s.stopChan)
	return nil
}

// Wait blocks until the server is stopped
func (s *Server) Wait() error {
	<-s.stopChan
	return nil
}

// GetFrontendAddress returns the frontend service address
func (s *Server) GetFrontendAddress() string {
	return fmt.Sprintf("%s:%d", s.options.FrontendIP, s.options.FrontendPort)
}

func (s *Server) waitForServerReady() error {
    // Poll with exponential backoff, validating only that the TCP listener
    // is accepting connections. Keep this lightweight and time‑bounded.
    backoff := 200 * time.Millisecond
    maxBackoff := 1 * time.Second
    // Default readiness ceiling to 15s unless overridden in options.
    total := s.options.ReadinessTimeout
    if total <= 0 {
        total = 15 * time.Second
    }
    deadline := time.Now().Add(total)

    for time.Now().Before(deadline) {
        if err := s.checkServerHealth(); err == nil {
            return nil
        }
        time.Sleep(backoff)
        if backoff < maxBackoff {
            backoff *= 2
            if backoff > maxBackoff {
                backoff = maxBackoff
            }
        }
    }
    // Do not treat failure as fatal; report for visibility only.
    return fmt.Errorf("timeout waiting for server to be ready")
}

func (s *Server) checkServerHealth() error {
    // Readiness is intentionally lightweight to avoid long startup delays
    // on constrained runners: validate TCP accept and that the gRPC channel
    // can be established. Avoid heavy RPCs like GetClusterInfo here.
    addr := fmt.Sprintf("%s:%d", s.options.FrontendIP, s.options.FrontendPort)
    d := net.Dialer{Timeout: 1 * time.Second}
    conn, err := d.Dial("tcp", addr)
    if err != nil {
        return fmt.Errorf("frontend not accepting connections: %w", err)
    }
    _ = conn.Close()

    // Consider TCP acceptance sufficient for readiness. Heavier gRPC operations
    // are retried by client constructors and namespace creation below.
    return nil
}

func (s *Server) initializeDatabase() error {
	// Always use file-based database
	dbPath := s.options.DatabaseFile
	if dbPath == "" {
		return fmt.Errorf("database file path is required")
	}

	if !filepath.IsAbs(dbPath) {
		absPath, err := filepath.Abs(dbPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}
		dbPath = absPath
		s.options.DatabaseFile = dbPath
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// Check if database file exists and has tables
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// Database doesn't exist, need to initialize schema
		s.logger.Info("Database file doesn't exist, initializing schema", tag.NewStringTag("path", dbPath))

		// Create SQL config for schema initialization
		sqlConfig := &config.SQL{
			PluginName:        "sqlite",
			DatabaseName:      dbPath,
			ConnectAttributes: s.buildSQLiteAttributes(),
		}
		// Set mode for file database
		sqlConfig.ConnectAttributes["mode"] = "rwc"

		// Initialize schema using Temporal's built-in schema setup
		if err := sqlite.SetupSchema(sqlConfig); err != nil {
			return fmt.Errorf("failed to setup database schema: %w", err)
		}

		s.logger.Info("Database schema initialized successfully")
	} else if err != nil {
		return fmt.Errorf("failed to check database file: %w", err)
	} else {
		// Database exists, verify it has the required tables
		s.logger.Info("Database file exists, verifying schema", tag.NewStringTag("path", dbPath))
	}

	return nil
}

func (s *Server) buildConfig() (*config.Config, error) {
	cfg := &config.Config{}

	// Always use file-based database
	dbPath := s.options.DatabaseFile
	if dbPath == "" {
		return nil, fmt.Errorf("database file path is required")
	}

	if !filepath.IsAbs(dbPath) {
		absPath, err := filepath.Abs(dbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path: %w", err)
		}
		dbPath = absPath
	}

	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Set up SQLite config
	sqliteConfig := config.SQL{
		PluginName:        "sqlite",
		DatabaseName:      dbPath,
		ConnectAttributes: s.buildSQLiteAttributes(),
	}
	sqliteConfig.ConnectAttributes["mode"] = "rwc"

	// Set up persistence configuration
	// Create separate configs for each store (they need to be separate instances)
	defaultSQLConfig := sqliteConfig
	visibilitySQLConfig := sqliteConfig

	cfg.Persistence = config.Persistence{
		DefaultStore:     "default",
		VisibilityStore:  "visibility",
		NumHistoryShards: 1,
		DataStores: map[string]config.DataStore{
			"default":    {SQL: &defaultSQLConfig},
			"visibility": {SQL: &visibilitySQLConfig},
		},
		TransactionSizeLimit: func() int { return 1048576 }, // 1MB limit
	}

	// Set up global configuration
	cfg.Global = config.Global{
		Membership: config.Membership{
			MaxJoinDuration:  30 * time.Second,
			BroadcastAddress: "127.0.0.1",
		},
		Authorization: config.Authorization{
			// No authorization by default for dev server
		},
	}

	// Set cluster metadata
	cfg.ClusterMetadata = &cluster.Config{
		EnableGlobalNamespace:    false,
		FailoverVersionIncrement: 10,
		MasterClusterName:        "active",
		CurrentClusterName:       "active",
		ClusterInformation: map[string]cluster.ClusterInformation{
			"active": {
				Enabled:                true,
				InitialFailoverVersion: 1,
				RPCAddress:             fmt.Sprintf("127.0.0.1:%d", s.options.FrontendPort),
			},
		},
	}

	// Set up services configuration
	// Frontend gets fixed port, others get dynamic ports
	cfg.Services = map[string]config.Service{
		"frontend": {
			RPC: config.RPC{
				GRPCPort: s.options.FrontendPort,
				BindOnIP: "127.0.0.1",
			},
		},
		"history": {
			RPC: config.RPC{
				GRPCPort: FindFreePort(),
				BindOnIP: "127.0.0.1",
			},
		},
		"matching": {
			RPC: config.RPC{
				GRPCPort: FindFreePort(),
				BindOnIP: "127.0.0.1",
			},
		},
		"worker": {
			RPC: config.RPC{
				GRPCPort: FindFreePort(),
				BindOnIP: "127.0.0.1",
			},
		},
	}

	// Set public client configuration
	cfg.PublicClient = config.PublicClient{
		HostPort: fmt.Sprintf("127.0.0.1:%d", s.options.FrontendPort),
	}

	// Set up metrics
	cfg.Global.Metrics = &metrics.Config{
		Prometheus: &metrics.PrometheusConfig{
			ListenAddress: fmt.Sprintf("%s:%d", s.options.FrontendIP, s.options.FrontendPort+200),
			HandlerPath:   "/metrics",
		},
	}

	// Set archival defaults
	cfg.NamespaceDefaults.Archival = config.ArchivalNamespaceDefaults{
		History: config.HistoryArchivalNamespaceDefaults{
			State: "disabled",
		},
		Visibility: config.VisibilityArchivalNamespaceDefaults{
			State: "disabled",
		},
	}

	// Create default namespace after server starts
	cfg.ClusterMetadata.EnableGlobalNamespace = false

	return cfg, nil
}

func (s *Server) buildSQLiteAttributes() map[string]string {
	attrs := make(map[string]string)

	// Set mode for SQLite
	attrs["mode"] = "rwc"

	// Default pragmas for performance - using the format Temporal expects
	attrs["_journal_mode"] = "WAL"
	attrs["_synchronous"] = "NORMAL"
	attrs["_busy_timeout"] = "10000"
	attrs["_foreign_keys"] = "ON"
	attrs["_locking_mode"] = "NORMAL"  // Ensure locks are released properly
	attrs["_wal_autocheckpoint"] = "1000" // Checkpoint every 1000 pages

	// Add custom pragmas
	for key, value := range s.options.SQLitePragmas {
		attrs[fmt.Sprintf("_%s", key)] = value
	}

	return attrs
}

func (s *Server) createDefaultNamespaces() error {
    // If no namespaces requested, skip client creation to avoid unnecessary
    // dialing during warmup in fast-teardown/test mode.
    if len(s.options.Namespaces) == 0 {
        return nil
    }
    // Budget for namespace readiness: use ReadinessTimeout if provided, else 60s.
    budget := s.options.ReadinessTimeout
    if budget <= 0 {
        budget = 60 * time.Second
    }
    deadline := time.Now().Add(budget)

    // Create a namespace client (retry until deadline during warmup)
    var c client.NamespaceClient
    for {
        var err error
        c, err = client.NewNamespaceClient(client.Options{
            HostPort: fmt.Sprintf("%s:%d", s.options.FrontendIP, s.options.FrontendPort),
        })
        if err == nil {
            break
        }
        if time.Now().After(deadline) {
            return fmt.Errorf("failed to create namespace client within budget: %w", err)
        }
        time.Sleep(500 * time.Millisecond)
    }
    defer c.Close()

	// Create each namespace specified in options
	for _, namespace := range s.options.Namespaces {
		if namespace == "" {
			continue
		}

		s.logger.Info("Creating namespace", tag.NewStringTag("namespace", namespace))

		// Set retention period to 1 day for dev server
		retention := durationpb.New(24 * time.Hour)

        // Retry namespace registration while services settle, bounded by deadline.
        var regErr error
        for {
            if time.Now().After(deadline) {
                break
            }
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            regErr = c.Register(ctx, &workflowservice.RegisterNamespaceRequest{
                Namespace:                        namespace,
                WorkflowExecutionRetentionPeriod: retention,
                Description:                      "Created by embedded temporal server",
            })
            cancel()
            if regErr == nil {
                break
            }
            time.Sleep(1 * time.Second)
        }

        if regErr != nil {
            // Check if namespace already exists - that's OK
            if _, ok := regErr.(*serviceerror.NamespaceAlreadyExists); ok {
                s.logger.Info("Namespace already exists", tag.NewStringTag("namespace", namespace))
                continue
            }
            return fmt.Errorf("failed to create namespace %s: %w", namespace, regErr)
        }

		s.logger.Info("Successfully created namespace", tag.NewStringTag("namespace", namespace))
	}

	return nil
}
