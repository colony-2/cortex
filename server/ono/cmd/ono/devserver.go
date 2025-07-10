package ono

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"time"

	"go.temporal.io/server/common/cluster"
	"go.temporal.io/server/common/config"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/metrics"
	"go.temporal.io/server/schema/sqlite"
	"go.temporal.io/server/temporal"
	"go.uber.org/zap"
)

type DevServerOptions struct {
	FrontendIP             string
	FrontendPort           int
	UIPort                 int
	Namespaces             []string
	DatabaseFile           string
	InMemory               bool
	LogLevel               string
	SQLitePragmas          map[string]string
	EnableUI               bool
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

	// Give the server a moment to fully initialize
	// This is needed because Start() returns before all services are ready
	time.Sleep(2 * time.Second)
	
	// The default namespace should be created automatically by Temporal server
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

func (ds *DevServer) initializeDatabase() error {
	if ds.options.InMemory {
		// For in-memory databases, we need to initialize schema after the server starts
		// The schema will be automatically created when the SQLite plugin connects
		ds.logger.Info("Using in-memory database, schema will be auto-created")
		return nil
	}

	// For file-based databases, check if we need to create schema
	dbPath := ds.options.DatabaseFile
	if !filepath.IsAbs(dbPath) {
		absPath, err := filepath.Abs(dbPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}
		dbPath = absPath
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

	// Set up SQLite config - based on temporaltest approach
	sqliteConfig := config.SQL{
		PluginName:        "sqlite",
		ConnectAttributes: ds.buildSQLiteAttributes(),
	}

	if ds.options.InMemory {
		// Use shared cache for ephemeral in-memory database - match temporaltest approach
		sqliteConfig.ConnectAttributes["mode"] = "memory"
		sqliteConfig.ConnectAttributes["cache"] = "shared"
		// Use random database name like temporaltest to avoid conflicts
		sqliteConfig.DatabaseName = fmt.Sprintf("%d", rand.Intn(9999999))
	} else {
		// File-based database
		dbPath := ds.options.DatabaseFile
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

		sqliteConfig.DatabaseName = dbPath
		sqliteConfig.ConnectAttributes["mode"] = "rwc"
	}

	// Set up persistence configuration
	cfg.Persistence = config.Persistence{
		DefaultStore:     "sqlite",
		VisibilityStore:  "sqlite",
		NumHistoryShards: 1,
		DataStores: map[string]config.DataStore{
			"sqlite": {SQL: &sqliteConfig},
		},
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

	return cfg, nil
}

func (ds *DevServer) buildSQLiteAttributes() map[string]string {
	attrs := make(map[string]string)
	
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