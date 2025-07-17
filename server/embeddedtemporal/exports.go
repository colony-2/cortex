package embeddedtemporal

import (
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	"github.com/vibethis/server/embeddedtemporal/pkg/temporal"
)

// Re-export key types
type Server = temporal.Server
type Options = temporal.Options
type ClientOptions = temporal.ClientOptions

// Re-export functions
func NewServer(opts Options, logger *zap.Logger) (*Server, error) {
	return temporal.NewServer(opts, logger)
}

func CreateClient(opts ClientOptions) (client.Client, error) {
	return temporal.CreateClient(opts)
}

func IsPortAvailable(host string, port int) bool {
	return temporal.IsPortAvailable(host, port)
}

func GetFreePort() (int, error) {
	return temporal.GetFreePort()
}