# Worker Scripts

Utility scripts for managing CloudFlare worker configuration.

## generate-key.ts

Generates a secure encryption key for token encryption and updates `.env.local`:

```bash
tsx worker/scripts/generate-key.ts
```

This will:
- Generate a 256-bit encryption key
- Automatically add or update `CATNIP_ENCRYPTION_KEY` in `.env.local`
- Create `.env.local` if it doesn't exist

## import-pem.ts

Imports a GitHub App private key PEM file and converts it for use in environment variables:

```bash
# Using default filename
tsx worker/scripts/import-pem.ts

# Or specify a custom PEM file
tsx worker/scripts/import-pem.ts path/to/your-private-key.pem
```

This will:
- Read the PEM file
- Convert newlines to `\n` for single-line storage
- Add or update `GITHUB_APP_PRIVATE_KEY` in `.env.local`
- The key will be automatically converted back to multi-line format when used

## Environment Variables

After running these scripts, your `.env.local` should contain:

```env
GITHUB_APP_ID=1594285
GITHUB_CLIENT_ID=Iv23liz5Ry8r5913ZAhz
GITHUB_CLIENT_SECRET=002595a4a5c9aa51017a4bd0cc8c198a7dfba640
GITHUB_WEBHOOK_SECRET=9fc1400bd624e081ffc73cb6492d77f979f970621ce336af7aee0434014682fb
CATNIP_ENCRYPTION_KEY=<generated-base64-key>
GITHUB_APP_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----"
```

## Deployment

When deploying to CloudFlare, add these as secrets:

```bash
wrangler secret put GITHUB_CLIENT_ID
wrangler secret put GITHUB_CLIENT_SECRET
wrangler secret put GITHUB_WEBHOOK_SECRET
wrangler secret put CATNIP_ENCRYPTION_KEY
wrangler secret put GITHUB_APP_PRIVATE_KEY
```