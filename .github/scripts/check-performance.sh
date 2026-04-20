#!/bin/bash
# Performance Regression Detection Script for TLA+ Models
# Usage: check-performance.sh <model_file> <config_file> <baseline_file> <thresholds_file>

set -e

MODEL_FILE="$1"
CONFIG_FILE="$2"
BASELINE_FILE="${3:-configs/model-baseline.json}"
THRESHOLDS_FILE="${4:-configs/performance-thresholds.json}"

# Extract model name (without path and extension)
MODEL_NAME=$(basename "$MODEL_FILE" .tla)

# Check if baseline exists
if [ ! -f "$BASELINE_FILE" ]; then
    echo "⚠️ Baseline file not found: $BASELINE_FILE"
    echo "STATES=0"
    echo "DISTINCT_STATES=0"
    echo "RUNTIME=0"
    echo "STATES_GROWTH=0"
    echo "WARNING=false"
    echo "BLOCK=false"
    echo "MESSAGE=Baseline not found, skipping performance check"
    exit 0
fi

# Get baseline values
BASELINE_STATES=$(jq -r ".${MODEL_NAME}.states // 0" "$BASELINE_FILE")
BASELINE_DISTINCT=$(jq -r ".${MODEL_NAME}.distinctStates // 0" "$BASELINE_FILE")
BASELINE_RUNTIME=$(jq -r ".${MODEL_NAME}.runtimeSeconds // 0" "$BASELINE_FILE")

# Get thresholds
DEFAULT_WARNING=$(jq -r ".warningThresholdPercent // 20" "$THRESHOLDS_FILE")
DEFAULT_BLOCK=$(jq -r ".blockThresholdPercent // 50" "$THRESHOLDS_FILE")
DEFAULT_MAX_RUNTIME=$(jq -r ".maxRuntimeSeconds // 60" "$THRESHOLDS_FILE")
DEFAULT_MAX_STATES=$(jq -r ".maxStates // 500000" "$THRESHOLDS_FILE")

# Model-specific thresholds (override defaults)
MODEL_WARNING=$(jq -r ".models.${MODEL_NAME}.warningThresholdPercent // $DEFAULT_WARNING" "$THRESHOLDS_FILE")
MODEL_BLOCK=$(jq -r ".models.${MODEL_NAME}.blockThresholdPercent // $DEFAULT_BLOCK" "$THRESHOLDS_FILE")
MODEL_MAX_RUNTIME=$(jq -r ".models.${MODEL_NAME}.maxRuntimeSeconds // $DEFAULT_MAX_RUNTIME" "$THRESHOLDS_FILE")

# Run TLC and capture output
echo "🔍 Running performance check for $MODEL_NAME..."
START_TIME=$(date +%s)

if [ -f "$CONFIG_FILE" ]; then
    TLC_OUTPUT=$(java -cp "$TLA_TOOLS_PATH" tlc.TLC -config "$CONFIG_FILE" "$MODEL_FILE" 2>&1)
else
    TLC_OUTPUT=$(java -cp "$TLA_TOOLS_PATH" tlc.TLC "$MODEL_FILE" 2>&1)
fi

END_TIME=$(date +%s)
RUNTIME=$((END_TIME - START_TIME))

# Parse TLC output
STATES=$(echo "$TLC_OUTPUT" | grep -oP 'States: \K[0-9]+' | tail -1)
DISTINCT_STATES=$(echo "$TLC_OUTPUT" | grep -oP 'Distinct states: \K[0-9]+' | tail -1)
QUEUED=$(echo "$TLC_OUTPUT" | grep -oP 'Queued: \K[0-9]+' | tail -1)

# Default to 0 if not found
STATES=${STATES:-0}
DISTINCT_STATES=${DISTINCT_STATES:-0}

# Calculate growth percentage
if [ "$BASELINE_STATES" -gt 0 ]; then
    STATES_GROWTH=$(( (STATES - BASELINE_STATES) * 100 / BASELINE_STATES ))
else
    STATES_GROWTH=0
fi

if [ "$BASELINE_RUNTIME" -gt 0 ]; then
    RUNTIME_GROWTH=$(( (RUNTIME - BASELINE_RUNTIME) * 100 / BASELINE_RUNTIME ))
else
    RUNTIME_GROWTH=0
fi

# Determine warning/block status
WARNING="false"
BLOCK="false"
MESSAGE=""

# Check state count thresholds
if [ "$STATES" -gt "$DEFAULT_MAX_STATES" ]; then
    BLOCK="true"
    MESSAGE="$MESSAGE State count ($STATES) exceeds maximum ($DEFAULT_MAX_STATES). "
elif [ "$STATES_GROWTH" -ge "$MODEL_BLOCK" ]; then
    BLOCK="true"
    MESSAGE="$MESSAGE State count increased by ${STATES_GROWTH}% (threshold: ${MODEL_BLOCK}%). "
elif [ "$STATES_GROWTH" -ge "$MODEL_WARNING" ]; then
    WARNING="true"
    MESSAGE="$MESSAGE State count increased by ${STATES_GROWTH}% (warning threshold: ${MODEL_WARNING}%). "
fi

# Check runtime thresholds
if [ "$RUNTIME" -gt "$MODEL_MAX_RUNTIME" ]; then
    BLOCK="true"
    MESSAGE="$MESSAGE Runtime ($RUNTIMEs) exceeds maximum (${MODEL_MAX_RUNTIME}s). "
elif [ "$RUNTIME_GROWTH" -ge "$MODEL_BLOCK" ]; then
    BLOCK="true"
    MESSAGE="$MESSAGE Runtime increased by ${RUNTIME_GROWTH}% (threshold: ${MODEL_BLOCK}%). "
elif [ "$RUNTIME_GROWTH" -ge "$MODEL_WARNING" ]; then
    WARNING="true"
    MESSAGE="$MESSAGE Runtime increased by ${RUNTIME_GROWTH}% (warning threshold: ${MODEL_WARNING}%). "
fi

# Clean up message
MESSAGE=$(echo "$MESSAGE" | sed 's/^ //' | sed 's/ $//')

# Output results as environment variables
echo "STATES=$STATES"
echo "DISTINCT_STATES=$DISTINCT_STATES"
echo "RUNTIME=$RUNTIME"
echo "STATES_GROWTH=$STATES_GROWTH"
echo "RUNTIME_GROWTH=$RUNTIME_GROWTH"
echo "WARNING=$WARNING"
echo "BLOCK=$BLOCK"
echo "MESSAGE=$MESSAGE"

# Output human-readable summary
echo ""
echo "📊 Performance Summary for $MODEL_NAME:"
echo "┌─────────────────┬──────────────┬──────────────┬──────────────┐"
echo "│ Metric          │ Current      │ Baseline     │ Growth       │"
echo "├─────────────────┼──────────────┼──────────────┼──────────────┤"
echo "│ States          │ $STATES       │ $BASELINE_STATES │ ${STATES_GROWTH}%      │"
echo "│ Distinct States │ $DISTINCT_STATES │ $BASELINE_DISTINCT │ -            │"
echo "│ Runtime (s)     │ $RUNTIME      │ $BASELINE_RUNTIME │ ${RUNTIME_GROWTH}%      │"
echo "└─────────────────┴──────────────┴──────────────┴──────────────┘"

if [ "$BLOCK" = "true" ]; then
    echo "❌ PERFORMANCE REGRESSION DETECTED - BLOCKING"
    echo "   $MESSAGE"
    exit 1
elif [ "$WARNING" = "true" ]; then
    echo "⚠️ PERFORMANCE WARNING"
    echo "   $MESSAGE"
else
    echo "✅ Performance check passed"
fi
