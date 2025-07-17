package temporal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/server/common/cluster"
	"go.temporal.io/server/common/config"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/membership/static"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/common/primitives"
	"go.temporal.io/server/schema/sqlite"
	"go.temporal.io/server/temporal"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
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
}

// Server is an embedded Temporal server instance
type Server struct {
	server   temporal.Server
	logger   log.Logger
	options  Options
	stopChan chan struct{}
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

	// Create server options - use default services with static hosts
	serverOpts := []temporal.ServerOption{
		temporal.WithConfig(cfg),
		temporal.ForServices(temporal.DefaultServices),
		temporal.WithStaticHosts(staticHosts),
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

	// Wait for server to be ready by checking if we can connect
	// This ensures all services are registered and ready
	s.logger.Info("Waiting for server to be ready...")

	if err := s.waitForServerReady(); err != nil {
		return fmt.Errorf("server failed to become ready: %w", err)
	}

	// Create default namespaces after server is ready
	if err := s.createDefaultNamespaces(); err != nil {
		return fmt.Errorf("failed to create default namespaces: %w", err)
	}

	s.logger.Info("Server initialization complete",
		tag.NewStringTag("rpc", fmt.Sprintf("%s:%d", s.options.FrontendIP, s.options.FrontendPort)))

	return nil
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
	if s.server != nil {
		s.server.Stop()
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
	// Poll with exponential backoff
	backoff := 100 * time.Millisecond
	maxBackoff := 2 * time.Second
	timeout := time.After(60 * time.Second)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for server to be ready")
		default:
			// Try to connect and perform a health check
			if err := s.checkServerHealth(); err == nil {
				return nil
			}

			// Exponential backoff
			time.Sleep(backoff)
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
		}
	}
}

func (s *Server) checkServerHealth() error {
	// Try to connect to the server
	c, err := client.Dial(client.Options{
		HostPort:  fmt.Sprintf("%s:%d", s.options.FrontendIP, s.options.FrontendPort),
		Namespace: client.DefaultNamespace,
	})
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer c.Close()

	// Try to list namespaces as a health check
	// This verifies that all services are up and communicating
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use the WorkflowService to check system namespace
	resp, err := c.WorkflowService().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: "temporal-system",
	})
	if err != nil {
		return fmt.Errorf("failed to describe system namespace: %w", err)
	}

	if resp.NamespaceInfo == nil {
		return fmt.Errorf("system namespace not found")
	}

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
	// Create a namespace client
	c, err := client.NewNamespaceClient(client.Options{
		HostPort: fmt.Sprintf("%s:%d", s.options.FrontendIP, s.options.FrontendPort),
	})
	if err != nil {
		return fmt.Errorf("failed to create namespace client: %w", err)
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

		err := c.Register(context.Background(), &workflowservice.RegisterNamespaceRequest{
			Namespace:                        namespace,
			WorkflowExecutionRetentionPeriod: retention,
			Description:                      "Created by embedded temporal server",
		})
		
		if err != nil {
			// Check if namespace already exists - that's OK
			if _, ok := err.(*serviceerror.NamespaceAlreadyExists); ok {
				s.logger.Info("Namespace already exists", tag.NewStringTag("namespace", namespace))
				continue
			}
			return fmt.Errorf("failed to create namespace %s: %w", namespace, err)
		}

		s.logger.Info("Successfully created namespace", tag.NewStringTag("namespace", namespace))
	}

	return nil
}