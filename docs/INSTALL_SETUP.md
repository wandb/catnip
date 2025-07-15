# Install Script Setup

This document describes the setup for the Catnip installation script that allows users to install the CLI with `curl -sSfL install.catnip.sh | sh`.

## Overview

The install script (`public/install.sh`) has been modified to use a proxy through our Cloudflare worker instead of directly accessing GitHub's API and release assets. This allows us to serve the installation script from a private repository using GitHub App authentication.

## Architecture

```
User -> install.catnip.sh -> Cloudflare Worker -> GitHub API (with App token) -> Private Repository
```

## Setup Steps

### 1. GitHub App Configuration

The system uses your existing GitHub App (configured via `GITHUB_APP_ID` and `GITHUB_APP_PRIVATE_KEY`) to authenticate with GitHub. No additional setup is required if you already have the GitHub App configured.

### 2. Environment Variables

Ensure these variables are set in your `.dev.vars` file:

```bash
GITHUB_APP_ID=your_app_id
GITHUB_APP_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----"
```

### 3. Upload Secrets to Cloudflare

Use the existing secrets upload script:

```bash
# For QA environment
npx tsx worker/scripts/upload-secrets.ts --env qa

# For production environment
npx tsx worker/scripts/upload-secrets.ts --env production
```

## Worker Endpoints

The worker now includes these new endpoints:

- `GET /v1/github/releases/latest` - Proxies GitHub releases API
- `GET /v1/github/releases/download/:version/:filename` - Proxies GitHub release asset downloads

## Install Script Changes

The install script has been updated to:

1. Use `CATNIP_PROXY_URL` environment variable (defaults to `https://install.catnip.sh`)
2. Query `/v1/github/releases/latest` instead of GitHub API directly
3. Download assets from `/v1/github/releases/download/:version/:filename`

## Usage

Once deployed, users can install catnip with:

```bash
curl -sSfL install.catnip.sh | sh
```

## Testing

To test the installation flow:

1. Ensure `GITHUB_RELEASE_TOKEN` is set in your environment
2. Start the dev server: `pnpm dev`
3. Test the proxy endpoints:
   ```bash
   curl http://localhost:8787/v1/github/releases/latest
   ```
4. Test the full installation (requires actual releases):
   ```bash
   curl -sSfL http://localhost:8787/install.sh | sh
   ```

## Security Considerations

- The system uses GitHub App authentication with installation tokens
- Installation tokens are cached for 50 minutes to reduce API calls
- The GitHub App should have minimal permissions (only `contents: read` for releases)
- The proxy endpoints don't require authentication from end users
- Assets are cached for 1 hour to reduce GitHub API calls

## Troubleshooting

- If the proxy returns 500 errors, check that `GITHUB_APP_ID` and `GITHUB_APP_PRIVATE_KEY` are properly set
- If assets fail to download, ensure the GitHub App has `contents: read` permissions for the repository
- Check Cloudflare Worker logs for authentication errors
- Verify that the GitHub App is installed in the `wandb` organization