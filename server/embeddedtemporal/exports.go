package embeddedtemporal

import (
	"go.temporal.io/sdk/client"
	"github.com/vibethis/server/embeddedtemporal/pkg/temporal"
)

// Re-export key types
type Server = temporal.Server
type Options = temporal.Options
type ClientOptions = temporal.ClientOptions

// Re-export functions
func NewServer(opts Options) (*Server, error) {
	return temporal.NewServer(opts)
}

func NewClient(opts ClientOptions) (client.Client, error) {
	return temporal.NewClient(opts)
}

func NewNamespaceClient(hostPort string) (client.NamespaceClient, error) {
	return temporal.NewNamespaceClient(hostPort)
}

func IsPortAvailable(host string, port int) bool {
	return temporal.IsPortAvailable(host, port)
}

func FindFreePort() int {
	return temporal.FindFreePort()
}