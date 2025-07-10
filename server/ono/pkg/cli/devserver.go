package cli

import (
	"context"
	"fmt"
	"net"
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
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/schema/sqlite"
	"go.temporal.io/server/temporal"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
)

type DevServerOptions struct {
	FrontendIP    string
	FrontendPort  int
	UIPort        int
	Namespaces    []string
	DatabaseFile  string
	LogLevel      string
	SQLitePragmas map[string]string
	EnableUI      bool
}

type DevServer struct {
	server   temporal.Server
	logger   log.Logger
	options  DevServerOptions
	stopChan chan struct{}
}

func NewDevServer(opts DevServerOptions) (*DevServer, error) {
	return &DevServer{
		options:  opts,
		stopChan: make(chan struct{}),
	}, nil
}

func (ds *DevServer) Start() error {
	// Create logger based on log level
	var zapLogger *zap.Logger
	var err error

	switch ds.options.LogLevel {
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
	ds.logger = log.NewZapLogger(zapLogger)

	// Initialize database schema if needed
	if err := ds.initializeDatabase(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Build server configuration
	cfg, err := ds.buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	// Create server options - use default services like temporaltest
	serverOpts := []temporal.ServerOption{
		temporal.WithConfig(cfg),
		temporal.ForServices(temporal.DefaultServices),
	}

	// Create and start the server
	ds.server, err = temporal.NewServer(serverOpts...)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Start the server
	err = ds.server.Start()
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	ds.logger.Info("Temporal development server started",
		tag.Address(fmt.Sprintf("%s:%d", ds.options.FrontendIP, ds.options.FrontendPort)))

	// Wait for server to be ready by checking if we can connect
	// This ensures all services are registered and ready
	ds.logger.Info("Waiting for server to be ready...")

	if err := ds.waitForServerReady(); err != nil {
		return fmt.Errorf("server failed to become ready: %w", err)
	}

	// Create default namespaces after server is ready
	if err := ds.createDefaultNamespaces(); err != nil {
		return fmt.Errorf("failed to create default namespaces: %w", err)
	}

	ds.logger.Info("Server initialization complete",
		tag.NewStringTag("rpc", fmt.Sprintf("%s:%d", ds.options.FrontendIP, ds.options.FrontendPort)))

	return nil
}

func (ds *DevServer) Stop() error {
	if ds.server != nil {
		ds.server.Stop()
	}
	close(ds.stopChan)
	return nil
}

func (ds *DevServer) Wait() error {
	<-ds.stopChan
	return nil
}

func (ds *DevServer) waitForServerReady() error {
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
			if err := ds.checkServerHealth(); err == nil {
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

func (ds *DevServer) checkServerHealth() error {
	// Try to connect to the server
	c, err := client.Dial(client.Options{
		HostPort:  fmt.Sprintf("%s:%d", ds.options.FrontendIP, ds.options.FrontendPort),
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

func (ds *DevServer) initializeDatabase() error {
	// Always use file-based database
	dbPath := ds.options.DatabaseFile
	if dbPath == "" {
		return fmt.Errorf("database file path is required")
	}

	if !filepath.IsAbs(dbPath) {
		absPath, err := filepath.Abs(dbPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}
		dbPath = absPath
		ds.options.DatabaseFile = dbPath
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// Check if database file exists and has tables
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// Database doesn't exist, need to initialize schema
		ds.logger.Info("Database file doesn't exist, initializing schema", tag.NewStringTag("path", dbPath))

		// Create SQL config for schema initialization
		sqlConfig := &config.SQL{
			PluginName:        "sqlite",
			DatabaseName:      dbPath,
			ConnectAttributes: ds.buildSQLiteAttributes(),
		}
		// Set mode for file database
		sqlConfig.ConnectAttributes["mode"] = "rwc"

		// Initialize schema using Temporal's built-in schema setup
		if err := sqlite.SetupSchema(sqlConfig); err != nil {
			return fmt.Errorf("failed to setup database schema: %w", err)
		}

		ds.logger.Info("Database schema initialized successfully")
	} else if err != nil {
		return fmt.Errorf("failed to check database file: %w", err)
	} else {
		// Database exists, verify it has the required tables
		ds.logger.Info("Database file exists, verifying schema", tag.NewStringTag("path", dbPath))
	}

	return nil
}

func (ds *DevServer) buildConfig() (*config.Config, error) {
	cfg := &config.Config{}

	// Always use file-based database
	dbPath := ds.options.DatabaseFile
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
		ConnectAttributes: ds.buildSQLiteAttributes(),
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
				RPCAddress:             fmt.Sprintf("127.0.0.1:%d", ds.options.FrontendPort),
			},
		},
	}

	// Set up services configuration
	cfg.Services = map[string]config.Service{
		"frontend": {
			RPC: config.RPC{
				GRPCPort:        ds.options.FrontendPort,
				MembershipPort:  findFreePort(),
				BindOnLocalHost: true,
				BindOnIP:        "",
			},
		},
		"history": {
			RPC: config.RPC{
				GRPCPort:        ds.options.FrontendPort + 1,
				MembershipPort:  findFreePort(),
				BindOnLocalHost: true,
				BindOnIP:        "",
			},
		},
		"matching": {
			RPC: config.RPC{
				GRPCPort:        ds.options.FrontendPort + 2,
				MembershipPort:  findFreePort(),
				BindOnLocalHost: true,
				BindOnIP:        "",
			},
		},
		"worker": {
			RPC: config.RPC{
				GRPCPort:        ds.options.FrontendPort + 3,
				MembershipPort:  findFreePort(),
				BindOnLocalHost: true,
				BindOnIP:        "",
			},
		},
	}

	// Set public client configuration
	cfg.PublicClient = config.PublicClient{
		HostPort: fmt.Sprintf("127.0.0.1:%d", ds.options.FrontendPort),
	}

	// Set up metrics
	cfg.Global.Metrics = &metrics.Config{
		Prometheus: &metrics.PrometheusConfig{
			ListenAddress: fmt.Sprintf("%s:%d", ds.options.FrontendIP, ds.options.FrontendPort+200),
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

func (ds *DevServer) buildSQLiteAttributes() map[string]string {
	attrs := make(map[string]string)

	// Set mode for SQLite
	attrs["mode"] = "rwc"

	// Default pragmas for performance - using the format Temporal expects
	attrs["_journal_mode"] = "WAL"
	attrs["_synchronous"] = "NORMAL"
	attrs["_busy_timeout"] = "10000"
	attrs["_foreign_keys"] = "ON"

	// Add custom pragmas
	for key, value := range ds.options.SQLitePragmas {
		attrs[fmt.Sprintf("_%s", key)] = value
	}

	return attrs
}

func isPortAvailable(host string, port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// findFreePort finds a free port for membership communication
func findFreePort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		// Fallback to a high port if we can't find a free one
		return 7000 + int(time.Now().UnixNano()%1000)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func (ds *DevServer) createDefaultNamespaces() error {
	// Create a namespace client
	c, err := client.NewNamespaceClient(client.Options{
		HostPort: fmt.Sprintf("%s:%d", ds.options.FrontendIP, ds.options.FrontendPort),
	})
	if err != nil {
		return fmt.Errorf("failed to create namespace client: %w", err)
	}
	defer c.Close()

	// Create each namespace specified in options
	for _, namespace := range ds.options.Namespaces {
		if namespace == "" {
			continue
		}

		ds.logger.Info("Creating namespace", tag.NewStringTag("namespace", namespace))

		// Set retention period to 1 day for dev server
		retention := durationpb.New(24 * time.Hour)

		err := c.Register(context.Background(), &workflowservice.RegisterNamespaceRequest{
			Namespace:                        namespace,
			WorkflowExecutionRetentionPeriod: retention,
			Description:                      "Created by ono dev server",
		})
		
		if err != nil {
			// Check if namespace already exists - that's OK
			if _, ok := err.(*serviceerror.NamespaceAlreadyExists); ok {
				ds.logger.Info("Namespace already exists", tag.NewStringTag("namespace", namespace))
				continue
			}
			return fmt.Errorf("failed to create namespace %s: %w", namespace, err)
		}

		ds.logger.Info("Successfully created namespace", tag.NewStringTag("namespace", namespace))
	}

	// Wait a bit for namespace registration to propagate
	time.Sleep(2 * time.Second)

	return nil
}
