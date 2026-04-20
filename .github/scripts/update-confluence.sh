#!/bin/bash
# Confluence Update Script for TLA+ Verification Status with History
# Usage: update-confluence.sh <space_key> <status> <failed_models> <failed_details>

set -e

# Parameters
SPACE_KEY="$1"
STATUS="$2"
FAILED_MODELS="$3"
FAILED_DETAILS="$4"

# Environment variables
BASE_URL="${CONFLUENCE_BASE_URL}"
USERNAME="${CONFLUENCE_USERNAME}"
API_TOKEN="${CONFLUENCE_API_TOKEN}"

# Page configuration
PAGE_TITLE="TLA+ Verification Status"
MAX_HISTORY=10

# Basic auth header
AUTH=$(echo -n "${USERNAME}:${API_TOKEN}" | base64)

# Colors for status
if [ "$STATUS" = "PASS" ]; then
    STATUS_COLOR="green"
    STATUS_BADGE="✅ All Passed"
    STATUS_ICON="check-circle"
else
    STATUS_COLOR="red"
    STATUS_BADGE="❌ Verification Failed"
    STATUS_ICON="alert-circle"
fi

# Get current timestamp and commit info
TIMESTAMP=$(date -u "+%Y-%m-%d %H:%M:%S UTC")
COMMIT_SHA="${GITHUB_SHA:-unknown}"
BRANCH="${GITHUB_REF_NAME:-unknown}"

# Build history entry
build_history_entry() {
    local entry_status="$1"
    local entry_time="$2"
    local entry_commit="$3"
    local entry_branch="$4"
    local entry_models="$5"
    local entry_details="$6"
    
    if [ "$entry_status" = "PASS" ]; then
        entry_badge="✅"
    else
        entry_badge="❌"
    fi
    
    # Truncate commit hash for display
    short_commit=$(echo "$entry_commit" | cut -c1-7)
    
    echo "<tr>"
    echo "<td>$entry_time</td>"
    echo "<td>$entry_badge</td>"
    echo "<td><code>$short_commit</code></td>"
    echo "<td>$entry_branch</td>"
    if [ -n "$entry_models" ]; then
        # Show first failed model only
        first_model=$(echo "$entry_models" | head -n 1 | sed 's/^- //')
        echo "<td><ac:link><ri:page ri:content-title=\"$first_model\"/></ac:link></td>"
        # Truncate details
        short_details=$(echo "$entry_details" | head -c 100 | sed 's/\n/ /g')
        echo "<td>$short_details...</td>"
    else
        echo "<td>-</td>"
        echo "<td>-</td>"
    fi
    echo "</tr>"
}

# Extract existing history from page content
extract_history() {
    local content="$1"
    # Find table rows between <tbody> and </tbody> in history section
    echo "$content" | sed -n '/<h2>Verification History<\/h2>/,/<\/table>/p' | \
        grep -E '<tr>.*<\/tr>' | head -n $MAX_HISTORY
}

# Build page content
build_content() {
    local existing_history="$1"
    
    cat <<EOF
<ac:structured-macro ac:name="status" ac:schema-version="1">
  <ac:parameter ac:name="colour">$STATUS_COLOR</ac:parameter>
  <ac:parameter ac:name="title">$STATUS_BADGE</ac:parameter>
</ac:structured-macro>

<h2>Current Status</h2>

<table>
  <tbody>
    <tr>
      <th>Status</th>
      <td>$STATUS_BADGE</td>
    </tr>
    <tr>
      <th>Last Updated</th>
      <td>$TIMESTAMP</td>
    </tr>
    <tr>
      <th>Commit</th>
      <td><code>$COMMIT_SHA</code></td>
    </tr>
    <tr>
      <th>Branch</th>
      <td>$BRANCH</td>
    </tr>
  </tbody>
</table>

<h2>Failed Models</h2>
EOF

    if [ -n "$FAILED_MODELS" ]; then
        echo "<ul>"
        echo "$FAILED_MODELS" | while read -r line; do
            if [ -n "$line" ]; then
                echo "<li>$line</li>"
            fi
        done
        echo "</ul>"
        
        echo "<h3>Error Details</h3>"
        echo "<pre>$FAILED_DETAILS</pre>"
    else
        echo "<p>No failures - all models passed verification.</p>"
    fi

    echo "<h2>Verification History</h2>"
    echo "<table>"
    echo "<thead>"
    echo "<tr><th>时间</th><th>结果</th><th>提交</th><th>分支</th><th>失败模型</th><th>反例摘要</th></tr>"
    echo "</thead>"
    echo "<tbody>"
    
    # Add new entry first
    echo "$(build_history_entry "$STATUS" "$TIMESTAMP" "$COMMIT_SHA" "$BRANCH" "$FAILED_MODELS" "$FAILED_DETAILS")"
    
    # Add existing history (excluding new entry if it already exists)
    if [ -n "$existing_history" ]; then
        echo "$existing_history"
    fi
    
    echo "</tbody>"
    echo "</table>"
}

# Function to find page ID by title
find_page_id() {
    local response
    response=$(curl -s -X GET \
        -H "Authorization: Basic $AUTH" \
        -H "Content-Type: application/json" \
        "${BASE_URL}/rest/api/content?spaceKey=${SPACE_KEY}&title=${PAGE_TITLE}&limit=1")
    
    echo "$response" | grep -o '"id":"[^"]*"' | cut -d'"' -f4
}

# Function to get existing page content
get_page_content() {
    local page_id="$1"
    curl -s -X GET \
        -H "Authorization: Basic $AUTH" \
        "${BASE_URL}/rest/api/content/${page_id}?expand=body.storage" | \
        grep -o '"value":"[^"]*"' | cut -d'"' -f4 | sed 's/\\n/\n/g' | sed 's/\\r//g'
}

# Function to create new page
create_page() {
    local content
    content=$(build_content "")
    
    local payload
    payload="{
  \"type\": \"page\",
  \"title\": \"$PAGE_TITLE\",
  \"space\": {\"key\": \"$SPACE_KEY\"},
  \"body\": {
    \"storage\": {
      \"value\": \"$(echo "$content" | sed 's/"/\\\\"/g' | sed 's/\n/\\n/g')\",
      \"representation\": \"storage\"
    }
  }
}"
    
    curl -s -X POST \
        -H "Authorization: Basic $AUTH" \
        -H "Content-Type: application/json" \
        -d "$payload" \
        "${BASE_URL}/rest/api/content" | grep -o '"id":"[^"]*"' | cut -d'"' -f4
}

# Function to update existing page
update_page() {
    local page_id="$1"
    
    # Get current content to extract history
    local current_content
    current_content=$(get_page_content "$page_id")
    
    # Extract existing history
    local existing_history
    existing_history=$(extract_history "$current_content")
    
    # Build new content with history
    local content
    content=$(build_content "$existing_history")
    
    # Get current version
    local version_response
    version_response=$(curl -s -X GET \
        -H "Authorization: Basic $AUTH" \
        "${BASE_URL}/rest/api/content/${page_id}?expand=version")
    local current_version
    current_version=$(echo "$version_response" | grep -o '"number":[0-9]*' | cut -d':' -f2)
    local new_version=$((current_version + 1))
    
    local payload
    payload="{
  \"version\": {\"number\": $new_version},
  \"title\": \"$PAGE_TITLE\",
  \"body\": {
    \"storage\": {
      \"value\": \"$(echo "$content" | sed 's/"/\\\\"/g' | sed 's/\n/\\n/g')\",
      \"representation\": \"storage\"
    }
  }
}"
    
    curl -s -X PUT \
        -H "Authorization: Basic $AUTH" \
        -H "Content-Type: application/json" \
        -d "$payload" \
        "${BASE_URL}/rest/api/content/${page_id}"
}

# Main execution
echo "🔄 Updating Confluence page..."

# Check if page exists
PAGE_ID=$(find_page_id)

if [ -n "$PAGE_ID" ]; then
    echo "📝 Page found (ID: $PAGE_ID), updating..."
    update_page "$PAGE_ID"
else
    echo "🆕 Page not found, creating new page..."
    create_page
fi

echo "✅ Confluence update completed successfully"
