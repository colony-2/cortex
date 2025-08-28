#!/bin/bash

# Generate the code
oapi-codegen -generate types -templates ./oapi-templates -package oas oas.json > foo_temp.go 2>&1

# Check if it's an error message
if grep -q "error generating" foo_temp.go; then
    # Extract the Go code after the error message and fix it
    (
        echo "package oas"
        echo ""
        sed -n '1,/^error generating/d; p' foo_temp.go | sed 's/: oas.go.*//'
    ) | \
    sed 's/}` json:/} `json:/g' | \
    sed 's/]` json:/] `json:/g' | \
    sed 's/\*RetryPolicy` json:/\*RetryPolicy `json:/g' | \
    sed 's/\*string` json:/\*string `json:/g' | \
    sed 's/\*int` json:/\*int `json:/g' | \
    sed 's/\*bool` json:/\*bool `json:/g' | \
    sed 's/\*float32` json:/\*float32 `json:/g' | \
    sed 's/\*\[\]string` json:/\*\[\]string `json:/g' | \
    sed 's/string` json:/string `json:/g' | \
    sed 's/State` json:/State `json:/g' | \
    sed 's/"github.com\/oapi-codegen\/runtime"/"github.com\/oapi-codegen\/runtime"/' > foo.go
else
    mv foo_temp.go foo.go
fi

rm -f foo_temp.go

echo "Generated foo.go"