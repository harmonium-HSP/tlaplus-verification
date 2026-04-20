#!/bin/bash

set -e

echo "=== Raft Election Protocol Verification ==="

MODEL_FILE="models/raft/raft_election.tla"
CONFIG_FILE="models/raft/raft_election.cfg"
BASELINE_FILE="configs/model-baseline.json"
TLA_TOOLS_JAR="tla2tools.jar"

if [ ! -f "$MODEL_FILE" ]; then
    echo "❌ Model file not found: $MODEL_FILE"
    exit 1
fi

if [ ! -f "$CONFIG_FILE" ]; then
    echo "❌ Config file not found: $CONFIG_FILE"
    exit 1
fi

if [ ! -f "$TLA_TOOLS_JAR" ]; then
    echo "🔄 Downloading TLA+ tools..."
    wget -q https://github.com/tlaplus/tlaplus/releases/download/v1.8.0/tla2tools.jar
fi

echo "🔍 Running TLC model checker..."
START_TIME=$(date +%s)

TLC_OUTPUT=$(java -cp "$TLA_TOOLS_JAR" tlc.TLC -config "$CONFIG_FILE" "$MODEL_FILE" 2>&1)
EXIT_CODE=$?

END_TIME=$(date +%s)
RUNTIME=$((END_TIME - START_TIME))

echo ""

if [ $EXIT_CODE -eq 0 ]; then
    echo "✅ Raft election verification PASSED"
    
    STATES=$(echo "$TLC_OUTPUT" | grep -oP 'States: \K[0-9]+' | tail -1)
    DISTINCT=$(echo "$TLC_OUTPUT" | grep -oP 'Distinct states: \K[0-9]+' | tail -1)
    
    echo ""
    echo "📊 Verification Results:"
    echo "  Total States:      $STATES"
    echo "  Distinct States:   $DISTINCT"
    echo "  Runtime:           ${RUNTIME}s"
    
    if [ -f "$BASELINE_FILE" ]; then
        echo ""
        echo "🔄 Updating baseline..."
        
        if command -v jq &> /dev/null; then
            CURRENT_DATE=$(date +%Y-%m-%d)
            jq --arg states "$STATES" --arg distinct "$DISTINCT" --arg runtime "$RUNTIME" --arg date "$CURRENT_DATE" \
                '.raft_election = {states: ($states | tonumber), distinctStates: ($distinct | tonumber), runtimeSeconds: ($runtime | tonumber), date: $date}' \
                "$BASELINE_FILE" > "$BASELINE_FILE.tmp" && mv "$BASELINE_FILE.tmp" "$BASELINE_FILE"
            
            echo "✅ Baseline updated successfully"
        else
            echo "⚠️ jq not installed, skipping baseline update"
        fi
    fi
    
    exit 0
else
    echo "❌ Raft election verification FAILED"
    echo ""
    echo "📝 Error output:"
    echo "$TLC_OUTPUT" | tail -20
    exit 1
fi
