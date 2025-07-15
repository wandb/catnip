# Cloudflare Deployment Guide

This guide walks you through deploying Catnip to Cloudflare Workers with Containers support.

## Prerequisites

1. **Cloudflare Account**: You need a Cloudflare account with access to Workers
2. **Wrangler CLI**: Install the Cloudflare Wrangler CLI
   ```bash
   npm install -g wrangler
   ```
3. **Cloudflare Containers Access**: Ensure your account has access to the Containers beta

## Initial Setup

### 1. Authenticate with Cloudflare

```bash
wrangler login
```

This will open a browser window for authentication.

### 2. Build the Frontend

Before deploying, build the frontend assets:

```bash
pnpm build
```

This creates the `dist/` directory with your compiled frontend assets.

### 3. Configure Account ID

You need to add your Cloudflare account ID to `wrangler.jsonc`. You can find this in your Cloudflare dashboard (right sidebar).

```json
{
  "account_id": "your-account-id-here"
}
```

### 4. Configure Your Domain (Optional)

If you want to use a custom domain like `catnip.run`:

1. Add your domain to Cloudflare (if not already)
2. Configure a custom domain for your Worker in the Cloudflare dashboard
3. Or add to `wrangler.jsonc`:
   ```json
   "routes": [
     { "pattern": "catnip.run/*", "zone_name": "catnip.run" }
   ]
   ```

### 5. Configure HTTPS Redirects

To redirect HTTP to HTTPS, you have several options:

**Option A: Cloudflare Dashboard (Recommended)**
1. Go to **SSL/TLS** → **Edge Certificates**
2. Enable **Always Use HTTPS**

**Option B: Page Rules**
1. Go to **Rules** → **Page Rules**
2. Create rule for `http://catnip.run/*`
3. Set **Always Use HTTPS** action

**Option C: Already Configured in Worker**
The Worker code already includes HTTP to HTTPS redirects (except for curl/wget requests to preserve install script functionality).

## Environment Variables and Secrets

Catnip uses a mix of public environment variables (configured in `wrangler.jsonc`) and secrets (added via `wrangler secret`).

### Environment Structure

The project is configured with explicit environments (no default environment):

- **No default environment**: `wrangler deploy` will fail - you must specify an environment
- **QA environment**: `wrangler deploy --env qa` - uses QA GitHub App IDs
- **Production environment**: `wrangler deploy --env production` - uses production GitHub App IDs

This ensures you always know which environment you're deploying to and prevents accidental production deployments.

### Public Environment Variables

Each environment specifies its own GitHub App configuration in `wrangler.jsonc`:

- `GITHUB_APP_ID`: Your GitHub App ID (public, per environment)
- `GITHUB_CLIENT_ID`: Your GitHub OAuth App Client ID (public, per environment)
- `ENVIRONMENT`: Environment name (qa, production)

### Required Secrets

Add the actual secrets using the `wrangler secret put` command:

```bash
# GitHub OAuth App secret
wrangler secret put GITHUB_CLIENT_SECRET
# Enter value: [your-github-client-secret]

# GitHub App private key
wrangler secret put GITHUB_APP_PRIVATE_KEY
# Enter value: [paste entire private key including BEGIN/END lines]

# GitHub webhook secret
wrangler secret put GITHUB_WEBHOOK_SECRET
# Enter value: [your-webhook-secret]

# Encryption key for session cookies
wrangler secret put CATNIP_ENCRYPTION_KEY
# Enter value: [your-encryption-key]
```

### Automated Secret Upload

Use the provided TypeScript script to upload all secrets from `.dev.vars`:

```bash
# Upload secrets to QA environment
npx tsx worker/scripts/upload-secrets.ts --env qa

# Upload secrets to production environment
npx tsx worker/scripts/upload-secrets.ts --env production

# Force overwrite existing secrets
npx tsx worker/scripts/upload-secrets.ts --env qa --force

# Show help
npx tsx worker/scripts/upload-secrets.ts --help
```

Note: You must specify an environment since there's no default configuration.

Features:

- Automatically skips public variables (GITHUB_APP_ID, GITHUB_CLIENT_ID)
- Checks for existing secrets and errors if they exist (use --force to overwrite)
- Shows preview of secret values (first 4 characters)
- Supports environment-specific uploads
- Handles multi-line values (like private keys) correctly
- Uses temporary files to avoid command-line escaping issues

### Generating Secrets

If you need to generate new secrets:

```bash
# Generate a secure encryption key
openssl rand -base64 32

# Generate a webhook secret
openssl rand -hex 32
```

### GitHub OAuth App Setup

1. Go to GitHub Settings > Developer settings > OAuth Apps
2. Create a new OAuth App with:
   - **Application name**: Catnip
   - **Homepage URL**: https://catnip.run (or your domain)
   - **Authorization callback URL**: https://catnip.run/v1/auth/github
3. Save the Client ID and Client Secret

### GitHub App Setup (Optional)

For enhanced permissions and refresh tokens:

1. Go to GitHub Settings > Developer settings > GitHub Apps
2. Create a new GitHub App
3. Configure webhooks, permissions, and callback URLs
4. Generate and download a private key

## Deployment

### 1. Push the container

```bash
docker pull --platform=linux/amd64 ghcr.io/wandb/catnip:0.1.0
pnpm wrangler containers push ghcr.io/wandb/catnip:0.1.0
```

### 2. Deploy to Cloudflare

```bash
wrangler deploy --env production
```

This will:

- Build and push the container image
- Deploy the Worker code
- Upload static assets
- Create Durable Objects

### 3. Verify Deployment

After deployment, you'll see output like:

```
Published catnip (x.x.x)
  https://catnip.workers.dev
```

Visit the URL to verify your deployment is working.

### 3. Monitor Logs

View real-time logs:

```bash
wrangler tail
```

## Container Configuration

The container is defined in `container/Dockerfile` and is automatically built and deployed by Wrangler.

### Container Limits

- **Max Instances**: 40 (configured in wrangler.jsonc)
- **Sleep After**: 10 minutes of inactivity
- **Default Port**: 8080

### Updating the Container

When you modify the container:

1. Make changes to files in the `container/` directory
2. Run `wrangler deploy` - it will automatically rebuild and deploy the container

## Troubleshooting

### Common Issues

1. **Authentication Errors**

   - Verify all secrets are properly set
   - Check that GitHub OAuth/App URLs match your deployment URL

2. **Container Build Failures**

   - Ensure Docker is running locally
   - Check the Dockerfile syntax
   - Review build logs in Wrangler output

3. **Asset Loading Issues**
   - Ensure `pnpm build` was run before deployment
   - Check that the `dist/` directory exists

### Debug Commands

```bash
# Check your configuration
wrangler config:list

# View secret names (not values)
wrangler secret list

# Test locally with remote bindings
wrangler dev --remote

# View container logs
wrangler tail --format pretty
```

## Production Considerations

### 1. Custom Domain

For production, set up a custom domain:

```bash
# Add route for your domain
wrangler route add catnip.run/*
```

### 2. Environment-Specific Deployments

The project is configured with explicit environments, each with its own GitHub App IDs and routes:

```bash
# Deploy to QA
wrangler deploy --env qa

# Deploy to production
wrangler deploy --env production

# Note: No default environment - you must specify --env
```

Each environment has different:

- GitHub App IDs and Client IDs
- Routes (qa.catnip.run, catnip.run)
- Worker names (catnip-qa, catnip)

#### Environment-Specific Secrets

You'll need to set secrets for each environment separately:

```bash
# Set secrets for QA
wrangler secret put GITHUB_CLIENT_SECRET --env qa
wrangler secret put GITHUB_APP_PRIVATE_KEY --env qa
wrangler secret put GITHUB_WEBHOOK_SECRET --env qa
wrangler secret put CATNIP_ENCRYPTION_KEY --env qa

# Set secrets for production
wrangler secret put GITHUB_CLIENT_SECRET --env production
# ... repeat for other secrets
```

#### GitHub Apps per Environment

You'll need to create separate GitHub OAuth Apps/GitHub Apps for each environment:

1. **QA GitHub App**

   - Homepage URL: https://qa.catnip.run
   - Callback URL: https://qa.catnip.run/v1/auth/github
   - Webhook URL: https://qa.catnip.run/v1/github/webhooks

2. **Production GitHub App**
   - Homepage URL: https://catnip.run
   - Callback URL: https://catnip.run/v1/auth/github
   - Webhook URL: https://catnip.run/v1/github/webhooks

### 3. Monitoring

Enable Cloudflare Analytics and Workers Analytics to monitor:

- Request volume
- Error rates
- Container usage
- Performance metrics

### 4. Backup Secrets

Keep a secure backup of your secrets, especially:

- GitHub App private key
- Encryption keys
- OAuth credentials

## Updating

To update your deployment:

```bash
# Pull latest changes
git pull

# Install dependencies
pnpm install

# Build frontend
pnpm build

# Deploy
wrangler deploy
```

## Rollback

If you need to rollback:

```bash
# List deployments
wrangler deployments list

# Rollback to specific version
wrangler rollback [deployment-id]
```

## Cost Considerations

Monitor your usage for:

- Worker requests
- Container compute time
- Durable Object operations
- Bandwidth

Cloudflare Containers are billed based on:

- vCPU milliseconds
- Memory usage
- Number of instances

## Security Best Practices

1. **Rotate Secrets Regularly**

   - Update GitHub tokens periodically
   - Rotate encryption keys (requires user re-authentication)

2. **Use GitHub App Mode**

   - Provides better security with expiring tokens
   - Allows more granular permissions

3. **Monitor Access Logs**

   - Review authentication attempts
   - Watch for unusual container activity

4. **Keep Dependencies Updated**
   - Regularly update Worker dependencies
   - Keep container base image updated
