#!/bin/bash
# Baseline Update Script for TLA+ Model Performance
# Usage: update-baseline.sh <baseline_file>

set -e

BASELINE_FILE="${1:-model-baseline.json}"

# Create baseline file if it doesn't exist
if [ ! -f "$BASELINE_FILE" ]; then
    echo "{}" > "$BASELINE_FILE"
fi

# Get current date
DATE=$(date +%Y-%m-%d)

# Find all TLA+ models
MODELS=$(find . -name "*.tla" -type f | grep -v ".git" | sort)

echo "🔄 Updating performance baselines..."

for MODEL_FILE in $MODELS; do
    MODEL_NAME=$(basename "$MODEL_FILE" .tla)
    CONFIG_FILE="${MODEL_FILE%.tla}.cfg"
    
    echo "Processing $MODEL_NAME..."
    
    # Run TLC to get metrics
    START_TIME=$(date +%s)
    
    if [ -f "$CONFIG_FILE" ]; then
        TLC_OUTPUT=$(java -cp "$TLA_TOOLS_PATH" tlc.TLC -config "$CONFIG_FILE" "$MODEL_FILE" 2>&1)
    else
        TLC_OUTPUT=$(java -cp "$TLA_TOOLS_PATH" tlc.TLC "$MODEL_FILE" 2>&1)
    fi
    
    END_TIME=$(date +%s)
    RUNTIME=$((END_TIME - START_TIME))
    
    # Parse metrics
    STATES=$(echo "$TLC_OUTPUT" | grep -oP 'States: \K[0-9]+' | tail -1)
    DISTINCT_STATES=$(echo "$TLC_OUTPUT" | grep -oP 'Distinct states: \K[0-9]+' | tail -1)
    
    STATES=${STATES:-0}
    DISTINCT_STATES=${DISTINCT_STATES:-0}
    
    # Update baseline using jq
    echo "  - States: $STATES"
    echo "  - Distinct States: $DISTINCT_STATES"
    echo "  - Runtime: ${RUNTIME}s"
    
    # Update or add model entry
    TEMP_FILE=$(mktemp)
    jq \
        --arg name "$MODEL_NAME" \
        --argjson states "$STATES" \
        --argjson distinct "$DISTINCT_STATES" \
        --argjson runtime "$RUNTIME" \
        --arg date "$DATE" \
        '.[$name] = {states: $states, distinctStates: $distinct, runtimeSeconds: $runtime, date: $date}' \
        "$BASELINE_FILE" > "$TEMP_FILE"
    
    mv "$TEMP_FILE" "$BASELINE_FILE"
    
    echo "  ✓ Updated baseline for $MODEL_NAME"
done

echo ""
echo "✅ Baseline update completed successfully"
echo "Updated baseline file: $BASELINE_FILE"
