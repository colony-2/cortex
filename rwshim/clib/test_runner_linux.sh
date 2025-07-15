#!/bin/bash
# test_runner_linux.sh - Test runner for the intercept shim in Linux

set -e

echo "=== Building test components ==="
gcc -o test_app test_app.c
gcc -o mock_monitor mock_monitor.c
gcc -shared -fPIC -o intercept.so intercept.c -ldl

echo "=== Test 1: Normal execution (no shim) ==="
./test_app
echo "Exit code: $?"
rm -f test_output.txt

echo ""
echo "=== Test 2: Shimmed execution with ALLOW_ALL policy ==="
./mock_monitor &
MONITOR_PID=$!
sleep 1  # Let monitor start up

echo "Running shimmed test app..."
LD_PRELOAD=./intercept.so ./test_app
TEST_EXIT=$?
echo "Exit code: $TEST_EXIT"

kill $MONITOR_PID 2>/dev/null || true
wait $MONITOR_PID 2>/dev/null || true
rm -f test_output.txt /tmp/vibethis-rwshim.sock

echo ""
echo "=== Test 3: Shimmed execution with DENY_ALL policy ==="
./mock_monitor deny-all &
MONITOR_PID=$!
sleep 1

LD_PRELOAD=./intercept.so ./test_app 2>&1 || echo "Expected failure - Exit code: $?"

kill $MONITOR_PID 2>/dev/null || true
wait $MONITOR_PID 2>/dev/null || true
rm -f test_output.txt /tmp/vibethis-rwshim.sock

echo ""
echo "=== Test 4: Shimmed execution with DENY_WRITES policy ==="
./mock_monitor deny-writes &
MONITOR_PID=$!
sleep 1

LD_PRELOAD=./intercept.so ./test_app 2>&1 || echo "Expected failure - Exit code: $?"

kill $MONITOR_PID 2>/dev/null || true
wait $MONITOR_PID 2>/dev/null || true
rm -f test_output.txt /tmp/vibethis-rwshim.sock

echo ""
echo "=== Test 5: Shimmed execution with DENY_READS policy ==="
./mock_monitor deny-reads &
MONITOR_PID=$!
sleep 1

# First create the file without shim
echo "Creating test file first..."
echo "Test content for read test" > test_output.txt

LD_PRELOAD=./intercept.so ./test_app 2>&1 || echo "Expected failure - Exit code: $?"

kill $MONITOR_PID 2>/dev/null || true
wait $MONITOR_PID 2>/dev/null || true
rm -f test_output.txt /tmp/vibethis-rwshim.sock

echo ""
echo "=== Test 6: No monitor running (should allow by default) ==="
LD_PRELOAD=./intercept.so ./test_app
echo "Exit code: $?"
rm -f test_output.txt

echo ""
echo "=== All tests completed ==="