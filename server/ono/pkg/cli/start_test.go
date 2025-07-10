package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStartCommand(t *testing.T) {
	cmd := StartCmd

	assert.NotNil(t, cmd)
	assert.Equal(t, "start", cmd.Use)
	assert.NotEmpty(t, cmd.Short)

	assert.NotNil(t, cmd.Flags().Lookup("port"))
	assert.NotNil(t, cmd.Flags().Lookup("ui-port"))
	assert.NotNil(t, cmd.Flags().Lookup("namespace"))
	assert.NotNil(t, cmd.Flags().Lookup("db-filename"))
	assert.NotNil(t, cmd.Flags().Lookup("log-level"))
	assert.NotNil(t, cmd.Flags().Lookup("ip"))
	assert.NotNil(t, cmd.Flags().Lookup("sqlite-pragma"))
}

func TestStartCommandDefaultFlags(t *testing.T) {
	cmd := StartCmd

	portFlag := cmd.Flags().Lookup("port")
	assert.Equal(t, "7233", portFlag.DefValue)

	uiPortFlag := cmd.Flags().Lookup("ui-port")
	assert.Equal(t, "8233", uiPortFlag.DefValue)

	namespaceFlag := cmd.Flags().Lookup("namespace")
	assert.Equal(t, "default", namespaceFlag.DefValue)

	dbFlag := cmd.Flags().Lookup("db-filename")
	assert.Equal(t, "./ono.db", dbFlag.DefValue)

	logLevelFlag := cmd.Flags().Lookup("log-level")
	assert.Equal(t, "info", logLevelFlag.DefValue)

	ipFlag := cmd.Flags().Lookup("ip")
	assert.Equal(t, "127.0.0.1", ipFlag.DefValue)
}
