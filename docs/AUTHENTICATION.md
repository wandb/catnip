# Authentication Strategy for Catnip

This document outlines the authentication strategy for Catnip when deployed to CloudFlare.

## Overview

Catnip requires authentication to secure user containers and resources. The system needs to support:

1. **Web-based authentication** - For users accessing through browsers
2. **SDK/CLI authentication** - For programmatic access and development tools
3. **Container-level authentication** - For GitHub operations within containers

## Authentication Methods

### 1. GitHub App Integration

Creating a GitHub App provides several advantages:

- **OAuth flow** for web authentication
- **Installation tokens** for repository access
- **Fine-grained permissions** for user resources
- **Webhook support** for real-time updates

#### Implementation Plan:

1. Create a GitHub App with the following permissions:
   - Repository access (read/write)
   - User email (read)
   - Gist creation (write)
   
2. Store app credentials securely in CloudFlare environment variables

3. Implement OAuth callback handler in the worker

### 2. Web Authentication Flow

For browser-based access:

1. **Initial Request**: User visits catnip.dev
2. **Auth Check**: Worker checks for valid session cookie
3. **GitHub OAuth**: If no session, redirect to GitHub OAuth
4. **Callback**: Handle OAuth callback, create session
5. **Cookie**: Set secure HTTP-only cookie with session token
6. **Session Storage**: Store session data in CloudFlare KV or Durable Objects

```typescript
// Example session cookie structure
{
  "sessionId": "uuid",
  "userId": "github-user-id",
  "githubToken": "encrypted-token",
  "expiresAt": "timestamp"
}
```

### 3. SDK/CLI Authentication

For programmatic access, implement GitHub Device Code Flow:

1. **Device Code Request**: CLI requests device code from GitHub
2. **User Authorization**: User authorizes in browser
3. **Token Exchange**: CLI exchanges code for access token
4. **Token Verification**: Worker verifies GitHub token on each request

#### Benefits:
- No need to store long-lived tokens
- Direct GitHub authentication
- Users control token permissions
- Standard OAuth device flow

#### Implementation:

```typescript
// CLI authentication flow
async function authenticateCLI() {
  // 1. Request device code from GitHub
  const deviceCode = await requestDeviceCode();
  
  // 2. Display user code
  console.log(`Please visit: ${deviceCode.verification_uri}`);
  console.log(`Enter code: ${deviceCode.user_code}`);
  
  // 3. Poll for authorization
  const token = await pollForToken(deviceCode);
  
  // 4. Store token locally
  await storeToken(token);
}
```

### 4. Container Authentication

Containers need GitHub access for:
- Git operations
- Claude Code authentication
- GitHub CLI operations

#### Approach:

1. **Token Injection**: Inject user's GitHub token into container environment
2. **Credential Helper**: Configure git credential helper to use injected token
3. **Secure Storage**: Store credentials in container's secure storage
4. **Token Refresh**: Implement token refresh mechanism

## Security Considerations

### Token Storage

- **Web Sessions**: Store in CloudFlare KV with encryption
- **Container Tokens**: Inject as environment variables, never persist to disk
- **SDK Tokens**: Store locally with OS-appropriate secure storage

### Token Scopes

Minimize token scopes based on use case:
- **Web**: Full user access
- **SDK**: Repository and gist access only
- **Container**: Repository access for current project

### Session Management

- **Expiration**: 24-hour web sessions, 90-day SDK tokens
- **Revocation**: Implement token revocation endpoint
- **Rotation**: Automatic token rotation for long-lived sessions

## Implementation Phases

### Phase 1: Basic Authentication
- GitHub OAuth for web
- Session cookie management
- Basic token verification

### Phase 2: SDK Support
- Device code flow implementation
- CLI authentication commands
- Token storage utilities

### Phase 3: Advanced Features
- GitHub App creation
- Fine-grained permissions
- Token refresh mechanism
- Audit logging

## API Endpoints

### Worker Endpoints

```typescript
// Authentication endpoints
GET  /auth/login     // Redirect to GitHub OAuth
GET  /auth/callback  // OAuth callback handler
POST /auth/logout    // Clear session
GET  /auth/status    // Check authentication status

// SDK endpoints  
POST /auth/device    // Request device code
POST /auth/token     // Exchange code for token
POST /auth/verify    // Verify token validity
```

### Container API Authentication

All container API requests require authentication:

```typescript
// Header-based authentication
Authorization: Bearer <github-token>

// Or session cookie for web requests
Cookie: catnip-session=<session-id>
```

## Configuration

### Environment Variables

```bash
# GitHub App Configuration
GITHUB_APP_ID=
GITHUB_APP_PRIVATE_KEY=
GITHUB_CLIENT_ID=
GITHUB_CLIENT_SECRET=

# Session Configuration
SESSION_SECRET=
SESSION_DURATION=86400

# CloudFlare KV Namespaces
KV_SESSIONS=
KV_TOKENS=
```

### Worker Configuration

Update `wrangler.jsonc`:

```jsonc
{
  "kv_namespaces": [
    {
      "binding": "SESSIONS",
      "id": "session-namespace-id"
    },
    {
      "binding": "TOKENS", 
      "id": "token-namespace-id"
    }
  ],
  "vars": {
    "GITHUB_CLIENT_ID": "your-client-id"
  }
}
```

## Testing Strategy

1. **Unit Tests**: Test authentication flows in isolation
2. **Integration Tests**: Test full OAuth flow with GitHub
3. **Security Tests**: Verify token handling and session security
4. **Load Tests**: Ensure session storage scales

## Future Enhancements

1. **Multi-factor Authentication**: Add 2FA support
2. **Team Support**: Organization-level authentication
3. **API Keys**: Generate long-lived API keys for CI/CD
4. **SSO Integration**: Support enterprise SSO providers
5. **Audit Logs**: Track all authentication events