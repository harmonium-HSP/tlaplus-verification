#!/bin/sh
set -e

echo "=========================================="
echo "Running TLA+ Model Checker"
echo "=========================================="

# Download TLA+ tools if not present
if [ ! -f "/data/tla2tools.jar" ]; then
    echo "Downloading TLA+ tools..."
    wget --no-check-certificate -O /data/tla2tools.jar https://github.com/tlaplus/tlaplus/releases/download/v1.8.0/tla2tools.jar
fi

# Download JDK if not present
if [ ! -d "/data/jdk-11.0.22+7-jre" ]; then
    echo "Downloading JDK..."
    wget --no-check-certificate -O /data/jdk.tar.gz https://github.com/adoptium/temurin11-binaries/releases/download/jdk-11.0.22%2B7/OpenJDK11U-jre_x64_linux_hotspot_11.0.22_7.tar.gz
    tar -xzf /data/jdk.tar.gz -C /data/
fi

# Run TLC
echo ""
echo "Running model: $1"
echo "Config: $2"
echo ""

/data/jdk-11.0.22+7-jre/bin/java -cp /data/tla2tools.jar tlc.TLC -config "$2" "$1"
