#!/bin/bash

echo "=== End-to-End Position Persistence Test ==="

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Base URL
BASE_URL="http://localhost:8080"

# Function to check if server is running
check_server() {
    curl -s -o /dev/null -w "%{http_code}" $BASE_URL/api/graph
}

echo "1. Checking if server is running..."
if [ $(check_server) -ne 200 ]; then
    echo -e "${RED}Server is not running on port 8080. Please start it first.${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Server is running${NC}"

echo -e "\n2. Getting initial positions..."
INITIAL=$(curl -s $BASE_URL/api/positions)
echo "Initial positions: $INITIAL"
INITIAL_COUNT=$(echo $INITIAL | jq 'length')
echo "Count: $INITIAL_COUNT"

echo -e "\n3. Saving test positions..."
TEST_DATA='[
  {"nodeId": "api", "x": 100.5, "y": 200.5},
  {"nodeId": "frontend", "x": 300, "y": 400},
  {"nodeId": "database", "x": 500.5, "y": 600.5}
]'

SAVE_RESPONSE=$(curl -s -X POST $BASE_URL/api/positions \
  -H "Content-Type: application/json" \
  -d "$TEST_DATA")

if [ "$(echo $SAVE_RESPONSE | jq -r '.success')" = "true" ]; then
    echo -e "${GREEN}✓ Positions saved successfully${NC}"
else
    echo -e "${RED}✗ Failed to save positions${NC}"
    echo "Response: $SAVE_RESPONSE"
    exit 1
fi

echo -e "\n4. Retrieving saved positions..."
SAVED=$(curl -s $BASE_URL/api/positions)
echo "Saved positions: $SAVED"
SAVED_COUNT=$(echo $SAVED | jq 'length')
echo "Count: $SAVED_COUNT"

echo -e "\n5. Verifying positions..."
# Check if we have the expected nodes
for node_id in "api" "frontend" "database"; do
    NODE_DATA=$(echo $SAVED | jq -r ".[\"$node_id\"]")
    if [ "$NODE_DATA" != "null" ]; then
        X=$(echo $NODE_DATA | jq -r '.x')
        Y=$(echo $NODE_DATA | jq -r '.y')
        echo -e "${GREEN}✓ Found $node_id at position ($X, $Y)${NC}"
    else
        echo -e "${RED}✗ Missing position for $node_id${NC}"
    fi
done

echo -e "\n6. Testing position update..."
UPDATE_DATA='[
  {"nodeId": "api", "x": 999.99, "y": 888.88}
]'

UPDATE_RESPONSE=$(curl -s -X POST $BASE_URL/api/positions \
  -H "Content-Type: application/json" \
  -d "$UPDATE_DATA")

if [ "$(echo $UPDATE_RESPONSE | jq -r '.success')" = "true" ]; then
    echo -e "${GREEN}✓ Position updated successfully${NC}"
else
    echo -e "${RED}✗ Failed to update position${NC}"
    exit 1
fi

# Verify update
UPDATED=$(curl -s $BASE_URL/api/positions)
API_POS=$(echo $UPDATED | jq -r '.api')
if [ "$API_POS" != "null" ]; then
    X=$(echo $API_POS | jq -r '.x')
    Y=$(echo $API_POS | jq -r '.y')
    if [ "$X" = "999.99" ] && [ "$Y" = "888.88" ]; then
        echo -e "${GREEN}✓ API position updated correctly to ($X, $Y)${NC}"
    else
        echo -e "${RED}✗ API position not updated correctly. Got ($X, $Y)${NC}"
    fi
fi

# Check that other positions remain unchanged
FRONTEND_POS=$(echo $UPDATED | jq -r '.frontend')
if [ "$FRONTEND_POS" != "null" ]; then
    X=$(echo $FRONTEND_POS | jq -r '.x')
    Y=$(echo $FRONTEND_POS | jq -r '.y')
    if [ "$X" = "300" ] && [ "$Y" = "400" ]; then
        echo -e "${GREEN}✓ Frontend position unchanged at ($X, $Y)${NC}"
    else
        echo -e "${RED}✗ Frontend position changed unexpectedly to ($X, $Y)${NC}"
    fi
fi

echo -e "\n${GREEN}=== Test Complete ===${NC}"