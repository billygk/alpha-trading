#!/bin/bash

# --- Configuration ---
# Set these variables or pass them as arguments
GH_TOKEN=`cat github_token`
OWNER="billygk"
REPO="alpha-trading"
ASSET_NAME="alpha_watcher_linux_amd64"     # The exact filename of the release asset
OUTPUT_FILE="alpha_watcher"                # What to name the file locally

TAG=""

# Parse arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        -tag) TAG="$2"; shift ;;
        *) echo "Unknown parameter passed: $1"; exit 1 ;;
    esac
    shift
done

# --- Logic ---

# Determine the API endpoint
if [ -z "$TAG" ]; then
    echo "No tag specified, looking for LATEST release..."
    API_URL="https://api.github.com/repos/$OWNER/$REPO/releases/latest"
else
    echo "Looking for release tag: $TAG..."
    API_URL="https://api.github.com/repos/$OWNER/$REPO/releases/tags/$TAG"
fi

# 1. Fetch the Asset ID
# We query the Release API to find the internal 'id' GitHub assigned to the binary.
echo "Searching for Asset ID for $ASSET_NAME in $OWNER/$REPO..."

ASSET_ID=$(curl -s -H "Authorization: Bearer $GH_TOKEN" \
  -H "Accept: application/vnd.github+json" \
  "$API_URL" | \
  jq -r ".assets[] | select(.name == \"$ASSET_NAME\") | .id")

# 2. Validation
if [ "$ASSET_ID" == "null" ] || [ -z "$ASSET_ID" ]; then
  echo "Error: Could not find asset '$ASSET_NAME'."
  exit 1
fi

echo "Found Asset ID: $ASSET_ID. Starting download..."

# 3. Download the Asset
# Note: GitHub requires the 'application/octet-stream' header to return the binary data 
# instead of the JSON metadata when hitting the /assets/ endpoint.
curl -L -H "Authorization: Bearer $GH_TOKEN" \
  -H "Accept: application/octet-stream" \
  -o "$OUTPUT_FILE" \
  "https://api.github.com/repos/$OWNER/$REPO/releases/assets/$ASSET_ID"

if [ $? -eq 0 ]; then
  echo "Success: $OUTPUT_FILE downloaded."
  chmod +x "$OUTPUT_FILE"
  echo "Made $OUTPUT_FILE executable."
else
  echo "Error: Download failed."
  exit 1
fi
