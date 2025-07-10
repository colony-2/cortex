package ono

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"go.temporal.io/server/common/cluster"
	"go.temporal.io/server/common/config"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/metrics"
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
	// Create logger
	zapLogger, err := zap.NewDevelopment()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	ds.logger = log.NewZapLogger(zapLogger)

	// Build server configuration
	cfg, err := ds.buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	// Create server options
	serverOpts := []temporal.ServerOption{
		temporal.WithConfig(cfg),
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

func (ds *DevServer) buildConfig() (*config.Config, error) {
	cfg := &config.Config{}

	// Set up persistence
	if ds.options.InMemory {
		cfg.Persistence = config.Persistence{
			DefaultStore:     "default",
			VisibilityStore:  "visibility",
			NumHistoryShards: 1,
			DataStores: map[string]config.DataStore{
				"default": {
					SQL: &config.SQL{
						PluginName:        "sqlite",
						DatabaseName:      ":memory:",
						ConnectAttributes: ds.buildSQLiteAttributes(),
					},
				},
				"visibility": {
					SQL: &config.SQL{
						PluginName:        "sqlite",
						DatabaseName:      ":memory:",
						ConnectAttributes: ds.buildSQLiteAttributes(),
					},
				},
			},
		}
	} else {
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

		cfg.Persistence = config.Persistence{
			DefaultStore:     "default",
			VisibilityStore:  "visibility",
			NumHistoryShards: 1,
			DataStores: map[string]config.DataStore{
				"default": {
					SQL: &config.SQL{
						PluginName:        "sqlite",
						DatabaseName:      dbPath,
						ConnectAttributes: ds.buildSQLiteAttributes(),
					},
				},
				"visibility": {
					SQL: &config.SQL{
						PluginName:        "sqlite",
						DatabaseName:      dbPath,
						ConnectAttributes: ds.buildSQLiteAttributes(),
					},
				},
			},
		}
	}

	// Set up services
	cfg.Services = map[string]config.Service{
		"frontend": {
			RPC: config.RPC{
				GRPCPort:        ds.options.FrontendPort,
				MembershipPort:  ds.options.FrontendPort + 100,
				BindOnIP:        ds.options.FrontendIP,
			},
		},
		"history": {
			RPC: config.RPC{
				GRPCPort:        ds.options.FrontendPort + 1,
				MembershipPort:  ds.options.FrontendPort + 101,
				BindOnIP:        ds.options.FrontendIP,
			},
		},
		"matching": {
			RPC: config.RPC{
				GRPCPort:        ds.options.FrontendPort + 2,
				MembershipPort:  ds.options.FrontendPort + 102,
				BindOnIP:        ds.options.FrontendIP,
			},
		},
		"worker": {
			RPC: config.RPC{
				GRPCPort:        ds.options.FrontendPort + 3,
				MembershipPort:  ds.options.FrontendPort + 103,
				BindOnIP:        ds.options.FrontendIP,
			},
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
				RPCAddress:             fmt.Sprintf("%s:%d", ds.options.FrontendIP, ds.options.FrontendPort),
			},
		},
	}

	// Set up metrics
	cfg.Global.Metrics = &metrics.Config{
		Prometheus: &metrics.PrometheusConfig{
			ListenAddress: fmt.Sprintf("%s:%d", ds.options.FrontendIP, ds.options.FrontendPort+200),
			HandlerPath:   "/metrics",
		},
	}

	// Set membership
	cfg.Global.Membership = config.Membership{
		MaxJoinDuration:  30,
		BroadcastAddress: ds.options.FrontendIP,
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
	
	// Default pragmas for performance
	attrs["_pragma=journal_mode(WAL)"] = "1"
	attrs["_pragma=synchronous(NORMAL)"] = "1"
	attrs["_pragma=busy_timeout(10000)"] = "1"
	attrs["_pragma=temp_store(MEMORY)"] = "1"
	
	// Add custom pragmas
	for key, value := range ds.options.SQLitePragmas {
		attrs[fmt.Sprintf("_pragma=%s(%s)", key, value)] = "1"
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