#!/bin/bash
set -e

echo "Starting R2 upload for release assets..."

# Get the current tag from GitHub ref
TAG_NAME=${GITHUB_REF#refs/tags/}
echo "Processing release: $TAG_NAME"

# Required environment variables
if [ -z "$CLOUDFLARE_ACCOUNT_ID" ] || [ -z "$CLOUDFLARE_API_TOKEN" ] || [ -z "$R2_BUCKET_NAME" ]; then
    echo "Error: Missing required environment variables"
    echo "Required: CLOUDFLARE_ACCOUNT_ID, CLOUDFLARE_API_TOKEN, R2_BUCKET_NAME"
    exit 1
fi

# Function to upload file to R2 using wrangler
upload_to_r2() {
    local file_path="$1"
    local r2_key="$2"
    local content_type="$3"
    
    echo "Uploading $file_path to R2 at $r2_key..."
    
    # Export the API token for wrangler
    export CLOUDFLARE_API_TOKEN
    export CLOUDFLARE_ACCOUNT_ID
    
    # Run the wrangler command - use --remote to ensure we upload to Cloudflare, not local
    wrangler r2 object put "${R2_BUCKET_NAME}/${r2_key}" \
        --file="$file_path" \
        --content-type="$content_type" \
        --remote
    
    WRANGLER_EXIT_CODE=$?
    
    if [ $WRANGLER_EXIT_CODE -eq 0 ]; then
        echo "Successfully uploaded $file_path"
    else
        echo "Failed to upload $file_path"
        echo "Exit code: $WRANGLER_EXIT_CODE"
        exit 1
    fi
}

# Get release information from GitHub API
echo "Fetching release information from GitHub API..."
RELEASE_INFO=$(curl -s \
    -H "Authorization: token $GITHUB_TOKEN" \
    -H "Accept: application/vnd.github.v3+json" \
    "https://api.github.com/repos/$GITHUB_REPOSITORY/releases/tags/$TAG_NAME")

if [ $? -ne 0 ]; then
    echo "Failed to fetch release information"
    exit 1
fi

# Check if the response is an error
if echo "$RELEASE_INFO" | jq -e '.message' >/dev/null 2>&1; then
    echo "Error from GitHub API:"
    echo "$RELEASE_INFO" | jq '.'
    exit 1
fi

# Check if assets exist
if ! echo "$RELEASE_INFO" | jq -e '.assets' >/dev/null 2>&1; then
    echo "Error: No assets found in release response"
    exit 1
fi

# Extract release data and create metadata
echo "Creating release metadata..."
RELEASE_METADATA=$(echo "$RELEASE_INFO" | jq '{
    id: .id,
    name: .name,
    tag_name: .tag_name,
    target_commitish: .target_commitish,
    draft: .draft,
    prerelease: .prerelease,
    created_at: .created_at,
    published_at: .published_at,
    assets: [.assets[] | {
        id: .id,
        name: .name,
        content_type: .content_type,
        size: .size,
        download_count: .download_count,
        created_at: .created_at,
        updated_at: .updated_at
    }],
    body: .body,
    html_url: .html_url,
    zipball_url: .zipball_url,
    tarball_url: .tarball_url
}')

# Save metadata to temporary file
METADATA_FILE=$(mktemp)
echo "$RELEASE_METADATA" > "$METADATA_FILE"

# Upload release metadata
upload_to_r2 "$METADATA_FILE" "releases/$TAG_NAME/metadata.json" "application/json"

# Update latest release pointer
upload_to_r2 "$METADATA_FILE" "releases/latest.json" "application/json"

# Clean up metadata file
rm "$METADATA_FILE"

# Download and upload each asset
echo "Processing release assets..."
echo "$RELEASE_INFO" | jq -r '.assets[] | "\(.id) \(.name) \(.content_type // "application/octet-stream")"' | while read -r asset_id asset_name content_type; do
    echo "Processing asset: $asset_name (ID: $asset_id)"
    
    # Download asset from GitHub
    TEMP_FILE=$(mktemp)
    echo "Downloading $asset_name from GitHub..."
    
    curl -L -o "$TEMP_FILE" \
        -H "Authorization: token $GITHUB_TOKEN" \
        -H "Accept: application/octet-stream" \
        "https://api.github.com/repos/$GITHUB_REPOSITORY/releases/assets/$asset_id"
    
    if [ $? -eq 0 ]; then
        # Upload to R2
        upload_to_r2 "$TEMP_FILE" "releases/$TAG_NAME/$asset_name" "$content_type"
        echo "Successfully processed $asset_name"
    else
        echo "Failed to download $asset_name"
        rm -f "$TEMP_FILE"
        exit 1
    fi
    
    # Clean up temporary file
    rm -f "$TEMP_FILE"
done

echo "R2 upload completed successfully for release $TAG_NAME"