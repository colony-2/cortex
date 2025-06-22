#!/bin/bash

echo "Testing position API..."

# Test saving positions
echo "Saving positions..."
curl -X POST http://localhost:8080/api/positions \
  -H "Content-Type: application/json" \
  -d '[{"nodeId":"api","x":100,"y":200},{"nodeId":"frontend","x":300,"y":400}]'

echo -e "\n\nGetting positions..."
# Test getting positions
curl http://localhost:8080/api/positions | jq .

echo -e "\n\nDone!"