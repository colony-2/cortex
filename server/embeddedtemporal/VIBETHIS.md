# VIBETHIS

## Project: `embeddedtemporal`

**Description:**

This project, `embeddedtemporal`, provides a self-contained, embedded Temporal server for Go applications. It is designed to simplify local development, testing, and single-node deployments by removing the need to run a separate Temporal service. The server is backed by a single SQLite file for persistence, making it extremely easy to set up, run, and tear down.

**Key Features:**

*   **Embedded Server:** Runs a complete Temporal server (frontend, history, matching, worker) within a single Go process.
*   **SQLite Persistence:** Uses a single SQLite database file for all persistence, which is created automatically.
*   **Zero-Configuration Startup:** Can be started with zero or minimal configuration.
*   **Dynamic Port Allocation:** Capable of automatically finding free ports for its services, preventing port conflicts.
*   **Automatic Schema Management:** Initializes the required database schema on the first run.
*   **Configurable Namespaces:** Allows for the programmatic creation of namespaces on startup.
*   **Optional Web UI:** Can optionally run the Temporal Web UI on a specified port.

**Core APIs:**

The primary interface to this library is the `pkg/temporal` Go package.

*   **`NewServer(opts Options) (*Server, error)`**: Creates a new instance of the embedded Temporal server. The `Options` struct allows for configuration of:
    *   `FrontendIP` and `FrontendPort`
    *   `UIPort` for the Web UI
    *   A list of `Namespaces` to create on startup
    *   The `DatabaseFile` path
    *   `LogLevel`
    *   And other advanced options.

*   **`(*Server) Start() error`**: Starts the embedded server. This is a blocking call that will run until the server is stopped.

*   **`(*Server) StartAsync() error`**: Starts the embedded server in a separate goroutine.

*   **`(*Server) Stop()`**: Gracefully shuts down the embedded server and its services.

*   **`(*Server) FrontendHostPort() string`**: Returns the address of the frontend service (e.g., "127.0.0.1:7233"), which is used by Temporal clients to connect to the server.

*   **`NewClient(server *Server, namespace string) (client.Client, error)`**: A convenience function for creating a Temporal `client.Client` that is pre-configured to connect to the embedded server instance.

**How it Works:**

1.  The `NewServer` function initializes the server configuration, including setting up the necessary service listeners on free ports if not specified.
2.  The `Start` method sets up the persistence layer using the SQLite driver.
3.  It then boots up the full Temporal server stack using `go.temporal.io/server`.
4.  If enabled, it also starts the Temporal Web UI.
5.  The server then runs until `Stop` is called.

**Use Cases:**

*   **Local Development:** Developers can run a full Temporal server directly in their Go application, without needing Docker or a separate server process.
*   **Integration Testing:** Go tests can spin up an embedded server for each test or test suite, providing a clean, isolated environment for testing Temporal workflows and activities.
*   **Simple Deployments:** For small-scale applications, the embedded server can be used as the production Temporal instance, simplifying the deployment architecture.

**Example Usage:**

```go
package main

import (
	"log"
	"time"

	"github.com/vibethis/server/embeddedtemporal/pkg/temporal"
	"go.temporal.io/sdk/client"
)

func main() {
	// Configure the embedded server
	opts := temporal.Options{
		FrontendPort: 7233,
		DatabaseFile: "temporal_test.db",
		LogLevel:     "info",
		Namespaces:   []string{"default"},
	}

	// Create and start the server
	server, err := temporal.NewServer(opts)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	if err := server.StartAsync(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	log.Printf("Server started at: %s", server.FrontendHostPort())

	// Create a client connected to the server
	c, err := temporal.NewClient(server, "default")
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	log.Println("Client connected successfully.")

	// Your application logic here...
	// e.g., start a workflow execution
	// c.ExecuteWorkflow(...)

	time.Sleep(5 * time.Second) // Keep the server running for a bit
}
```