# GitHub Authentication Troubleshooting

## Common Issues

### "Resource not accessible by integration" Error

This error occurs when the GitHub OAuth token doesn't have the necessary permissions. Common causes:

1. **Using GitHub App without proper permissions**
   - Solution: Ensure your GitHub App has "Email addresses" permission set to "Read"
   - Users need to re-approve the app after permission changes

2. **OAuth scope mismatch**
   - The Hono OAuth middleware tries to fetch user emails by default
   - Ensure the `user:email` scope is included in your OAuth request

## Quick Fixes

### Switch to OAuth App Mode
Comment out GitHub App credentials in `.dev.vars`:
```bash
#GITHUB_APP_ID=...
#GITHUB_APP_PRIVATE_KEY=...
```

### Debug OAuth Flow
The worker logs OAuth completion details:
- Check `grantedScopes` to see what permissions were actually granted
- Check `oauthAppMode` to confirm which mode is being used
- Check `userEmail` to see if email was successfully fetched

### Restart Wrangler with Debug Logging
```bash
pnpm wrangler dev --log-level debug
```

## Configuration Notes

- `oauthApp: false` = GitHub App mode (requires app installation)
- `oauthApp: true` = OAuth App mode (simpler, no installation needed)
- The code automatically uses OAuth App mode if `GITHUB_APP_ID` is not set