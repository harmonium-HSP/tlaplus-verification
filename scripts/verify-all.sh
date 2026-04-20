#!/bin/bash

set -e

echo "=== Running all verifications ==="

echo ""
echo "1. Running unit tests..."
go test -v ./pkg/...

echo ""
echo "2. Running chaos tests..."
go test -v -run TestChaos -count=10 ./pkg/...

echo ""
echo "3. Running race detection..."
go test -race ./pkg/...

echo ""
echo "4. Checking code format..."
gofmt -l ./pkg/

echo ""
echo "=== All verifications completed ==="
