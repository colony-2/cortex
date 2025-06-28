#!/bin/bash
# Wrapper script for installing vibethis with proper ldflags

set -e

cd "${MOON_WORKSPACE_ROOT}/server-workspace/cmd/vibethis"
go install -tags=prod -ldflags "-X main.defaultPort=8081" .

echo "Installation complete. You can now run 'vibethis' from anywhere."
echo "Default port: 8081 (override with -p or --port flag)"