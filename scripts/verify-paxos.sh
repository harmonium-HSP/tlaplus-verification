#!/bin/bash

set -e

echo "=== Paxos Protocol Verification ==="
echo ""

MODEL_FILE="models/paxos/paxos_synod.tla"
CONFIG_FILE="models/paxos/paxos_synod.cfg"
BASELINE_FILE="configs/model-baseline.json"
TLA_TOOLS_JAR="tla2tools.jar"

echo "Step 1: TLA+ Model Checking"
echo "----------------------------"

if [ -f "$TLA_TOOLS_JAR" ] && command -v java &> /dev/null; then
    echo "🔍 Running TLC model checker for Paxos..."
    START_TIME=$(date +%s)

    TLC_OUTPUT=$(java -cp "$TLA_TOOLS_JAR" tlc.TLC -config "$CONFIG_FILE" "$MODEL_FILE" 2>&1)
    EXIT_CODE=$?

    END_TIME=$(date +%s)
    RUNTIME=$((END_TIME - START_TIME))

    if [ $EXIT_CODE -eq 0 ]; then
        echo "✅ TLA+ verification PASSED"
        
        STATES=$(echo "$TLC_OUTPUT" | grep -oP 'States: \K[0-9]+' | tail -1)
        DISTINCT=$(echo "$TLC_OUTPUT" | grep -oP 'Distinct states: \K[0-9]+' | tail -1)
        
        echo "  Total States:      $STATES"
        echo "  Distinct States:   $DISTINCT"
        echo "  Runtime:           ${RUNTIME}s"
        
        if [ -f "$BASELINE_FILE" ] && command -v jq &> /dev/null; then
            CURRENT_DATE=$(date +%Y-%m-%d)
            jq --arg states "$STATES" --arg distinct "$DISTINCT" --arg runtime "$RUNTIME" --arg date "$CURRENT_DATE" \
                '.paxos_synod = {states: ($states | tonumber), distinctStates: ($distinct | tonumber), runtimeSeconds: ($runtime | tonumber), date: $date}' \
                "$BASELINE_FILE" > "$BASELINE_FILE.tmp" && mv "$BASELINE_FILE.tmp" "$BASELINE_FILE"
            
            echo "  ✅ Baseline updated"
        fi
    else
        echo "❌ TLA+ verification FAILED"
        echo "$TLC_OUTPUT" | tail -20
        exit 1
    fi
else
    echo "⚠️ TLA+ verification skipped (Java or tla2tools.jar not available)"
fi

echo ""
echo "Step 2: Go Unit Tests"
echo "---------------------"

if command -v go &> /dev/null; then
    echo "🔍 Running Paxos unit tests..."
    go test -v ./pkg/paxos/ -run "TestBasic|PaxosNode|Majority|Acceptor|ProposalID|Learner"
    
    if [ $? -eq 0 ]; then
        echo "✅ Go unit tests PASSED"
    else
        echo "❌ Go unit tests FAILED"
        exit 1
    fi
else
    echo "⚠️ Go unit tests skipped (go not available)"
fi

echo ""
echo "Step 3: Go Chaos Tests"
echo "----------------------"

if command -v go &> /dev/null; then
    echo "🔍 Running Paxos chaos tests..."
    go test -v -count=5 ./pkg/paxos/ -run "TestChaos"
    
    if [ $? -eq 0 ]; then
        echo "✅ Go chaos tests PASSED"
    else
        echo "❌ Go chaos tests FAILED"
        exit 1
    fi
else
    echo "⚠️ Go chaos tests skipped (go not available)"
fi

echo ""
echo "=== All Paxos verification steps completed ==="
