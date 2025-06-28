#!/bin/bash
# Wrapper script for running Go tests through Moon
# This script filters out the spurious "package test is not in std" error

set -e

# Run go test and capture both stdout and stderr
go test -v ./... 2>&1 | grep -v "package test is not in std"

# Check the actual test result
exit ${PIPESTATUS[0]}