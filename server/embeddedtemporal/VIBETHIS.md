# VIBETHIS Project: Embedded Temporal Server

This project provides a lightweight, embedded Temporal server for Go applications. It is designed for local development, testing, and simple single-node deployments. The server uses a single SQLite file for persistence, making it easy to set up and manage.

## Key Features

- **Embedded Server:** Runs a complete Temporal server within a Go application.
- **SQLite Backend:** Uses a single SQLite file for persistence.
- **Automatic Schema Initialization:** Creates the necessary database schema on the first run.
- **Configurable Namespaces:** Allows defining custom namespaces upon server startup.
- **Dynamic Port Allocation:** Automatically finds free ports for internal services.

## Core APIs

The primary way to interact with this project is through the `embeddedtemporal` Go package.

### `NewServer(opts Options) (*Server, error)`

Creates a new embedded Temporal server instance.

**`Options` struct:**

- `FrontendIP` (string): The IP address for the frontend service (e.g., "127.0.0.1").
- `FrontendPort` (int): The port for the frontend service (e.g., 7233).
- `UIPort` (int): The port for the Temporal Web UI.
- `Namespaces` ([]string): A list of namespaces to create on startup.
- `DatabaseFile` (string): The path to the SQLite database file.
- `LogLevel` (string): The logging level ("debug", "info", "error").
- `SQLitePragmas` (map[string]string): Custom SQLite pragmas for performance tuning.
- `EnableUI` (bool): Enables the Temporal Web UI.

### `(*Server) Start() error`

Starts the embedded Temporal server. This is a blocking call that initializes the database, configures the services, and starts the server.

### `(*Server) Stop() error`

Gracefully shuts down the Temporal server.

### `(*Server) GetFrontendAddress() string`

Returns the address of the frontend service (e.g., "127.0.0.1:7233"), which can be used by Temporal clients.

### `NewClient(opts ClientOptions) (client.Client, error)`

A convenience function to create a Temporal client that connects to the embedded server.

**`ClientOptions` struct:**

- `HostPort` (string): The address of the frontend service.
- `Namespace` (string): The namespace to connect to.
- `MetricsHandler` (client.MetricsHandler): An optional handler for client-side metrics.

### `NewNamespaceClient(hostPort string) (client.NamespaceClient, error)`

A convenience function to create a Temporal namespace client, which can be used to manage namespaces (e.g., register, describe).

## Example Usage

```go
package main

import (
	"log"
	"github.com/vibethis/embeddedtemporal"
)

func main() {
	opts := embeddedtemporal.Options{
		FrontendIP:   "127.0.0.1",
		FrontendPort: 7233,
		DatabaseFile: "temporal.db",
		LogLevel:     "info",
		Namespaces:   []string{"default"},
	}

	server, err := embeddedtemporal.NewServer(opts)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Printf("Server started. Frontend at: %s", server.GetFrontendAddress())

	// Your application logic here...

	if err := server.Stop(); err != nil {
		log.Fatalf("Failed to stop server: %v", err)
	}
}
```
