#!/bin/bash
set -e

# Arrays of OS and ARCH to test
OS_LIST=("linux" "darwin" "windows")
ARCH_LIST=("amd64" "arm64")

echo "======================================"
echo "Running tests on native environment..."
echo "======================================"
go test -v -run TestStderrRedirectCapturesPanic .

echo ""
echo "======================================================"
echo "Cross-compiling test binary for all target environments"
echo "======================================================"

for OS in "${OS_LIST[@]}"; do
    for ARCH in "${ARCH_LIST[@]}"; do
        echo "Testing cross-compilation for GOOS=$OS GOARCH=$ARCH"
        # Compile the test binary without running it
        GOOS=$OS GOARCH=$ARCH go test -c -o /dev/null .
        echo "✅ OK: $OS/$ARCH"
    done
done

echo ""
echo "🎉 All crashlog package environments compiled successfully!"
