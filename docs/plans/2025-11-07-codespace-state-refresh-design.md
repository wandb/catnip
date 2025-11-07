# Codespace State Refresh and Setup Flow Design

**Date:** 2025-11-07
**Status:** Approved
**Authors:** Claude (brainstorming with user)

## Problem Statement

Two critical UX issues exist in the current codespace connection flow:

### Issue 1: Stale Codespace State After Deletion

When a user deletes their codespace in GitHub and attempts to reconnect via the iOS app:

1. App shows "Access My Codespace" button (based on stale `has_any_codespaces` flag)
2. User clicks button, triggering SSE connection
3. SSE fails because codespace no longer exists
4. App falls back to confusing "Setup" view with manual devcontainer instructions

**Root Cause:** The `has_any_codespaces` flag in `/v1/user/status` is based on CODESPACE_STORE data, which isn't automatically verified against GitHub when codespaces are deleted outside the app.

### Issue 2: Confusing Setup Flow

The current "Setup" view shows manual devcontainer.json instructions, which is the wrong UX for most scenarios:

- Users with repositories but no Catnip → Should see "Install Catnip" flow
- Users with repositories with Catnip → Should see "Launch New Codespace" flow
- Users with zero repositories → Should see "Create Repository" guidance

## Design Principles

1. **Fresh data on reconnect**: When health check fails and user clicks "Reconnect", fetch fresh state before navigation
2. **Rate limiting**: Protect GitHub API quota with server-side verification caching and rate limits
3. **Smart routing**: Worker determines correct next action based on user's GitHub state
4. **Graceful degradation**: System continues working even if refresh fails or is rate-limited

## Architecture Overview

```
┌─────────────────┐
│  WorkspacesView │  Health check fails
│                 │  ↓
│  [Reconnect]────┼──> fetchUserStatus(forceRefresh: true)
└─────────────────┘        ↓
                           ?refresh=true
                           ↓
                    ┌──────────────────┐
                    │  Cloudflare      │
                    │  Worker          │
                    │  /v1/user/status │
                    └──────────────────┘
                           ↓
                    Rate limit check (10s)
                           ↓
                    Verification cache check (60s)
                           ↓
                    verifyAndCleanCodespaces()
                           ↓
                    Return has_any_codespaces
                           ↓
┌─────────────────┐
│ CodespaceView   │  Renders with fresh state
│                 │
│ - Access/Launch │  Based on actual GitHub state
│ - Install       │
│ - Create Repo   │
└─────────────────┘
```

## Component 1: Worker-Side Verification Caching

### Cache Structure

```typescript
interface VerificationCache {
  username: string;
  lastVerified: number; // timestamp of last verification
  lastRefreshRequest: number; // timestamp of last refresh=true request
  verifiedCodespaces: CodespaceCredentials[];
}
```

### Implementation in `/v1/user/status`

**Location:** `worker/index.ts:924` (existing endpoint)

```typescript
app.get("/v1/user/status", requireAuth, async (c) => {
  const username = c.get("username");
  const accessToken = c.get("accessToken");
  const requestsRefresh = c.req.query("refresh") === "true";
  const now = Date.now();

  try {
    const codespaceStore = c.env.CODESPACE_STORE.get(
      c.env.CODESPACE_STORE.idFromName("global"),
    );

    // Get verification cache from Durable Object state
    const cache = await getVerificationCache(codespaceStore, username);

    // SERVER-SIDE RATE LIMITING
    // Protection: Ignore refresh=true if last refresh was < 10 seconds ago
    // This prevents rapid-fire refresh calls from client bugs or user spam
    let shouldRefresh = requestsRefresh;
    if (requestsRefresh && cache?.lastRefreshRequest) {
      const timeSinceLastRefresh = now - cache.lastRefreshRequest;
      if (timeSinceLastRefresh < 10_000) {
        console.log(
          `⚠️ Rate limit: Ignoring refresh request for ${username} ` +
            `(${timeSinceLastRefresh}ms since last refresh, min 10s required)`,
        );
        shouldRefresh = false; // Override - use cached data instead
      }
    }

    // CACHE LOGIC
    // Verify with GitHub if:
    // 1. Client requested refresh AND rate limit allows, OR
    // 2. No cache exists, OR
    // 3. Cache is older than 60 seconds
    const shouldVerify =
      shouldRefresh || !cache || now - cache.lastVerified > 60_000;

    let hasAnyCodespaces = false;

    if (shouldVerify) {
      console.log(`🔄 Verifying codespaces for ${username} with GitHub API`);

      // Update lastRefreshRequest timestamp if this was explicit refresh
      if (shouldRefresh) {
        await updateRefreshTimestamp(codespaceStore, username, now);
      }

      // Fetch all stored codespaces
      const allResponse = await codespaceStore.fetch(
        `https://internal/codespace/${username}?all=true`,
      );

      if (allResponse.ok) {
        const storedCodespaces =
          (await allResponse.json()) as CodespaceCredentials[];

        // Verify codespaces still exist in GitHub and clean up deleted ones
        const verifiedCodespaces = await verifyAndCleanCodespaces(
          storedCodespaces,
          accessToken,
          username,
          codespaceStore,
        );

        // Update cache
        await updateVerificationCache(codespaceStore, username, {
          lastVerified: now,
          verifiedCodespaces,
        });

        hasAnyCodespaces = verifiedCodespaces.length > 0;
      }
    } else {
      console.log(`📦 Using cached codespace data for ${username}`);
      hasAnyCodespaces = cache.verifiedCodespaces.length > 0;
    }

    return c.json({
      has_any_codespaces: hasAnyCodespaces,
    });
  } catch (error) {
    console.error("User status error:", error);
    return c.json({ error: "Internal server error" }, 500);
  }
});
```

### Durable Object Cache Helpers

**Location:** Add to CODESPACE_STORE Durable Object or as helper functions

```typescript
async function getVerificationCache(
  codespaceStore: DurableObjectStub,
  username: string,
): Promise<VerificationCache | null> {
  const response = await codespaceStore.fetch(
    `https://internal/verification-cache/${username}`,
  );
  if (!response.ok) return null;
  return await response.json();
}

async function updateVerificationCache(
  codespaceStore: DurableObjectStub,
  username: string,
  update: Partial<VerificationCache>,
): Promise<void> {
  await codespaceStore.fetch(
    `https://internal/verification-cache/${username}`,
    {
      method: "PATCH",
      body: JSON.stringify(update),
    },
  );
}

async function updateRefreshTimestamp(
  codespaceStore: DurableObjectStub,
  username: string,
  timestamp: number,
): Promise<void> {
  await updateVerificationCache(codespaceStore, username, {
    lastRefreshRequest: timestamp,
  });
}
```

### Key Benefits

- **60-second cache** reduces GitHub API calls for normal app usage
- **10-second rate limit** prevents abuse from client bugs or rapid reconnects
- **Server-side enforcement** protects quota even if iOS has bugs
- **Graceful degradation** serves cached data when rate limited
- **Audit trail** via console.log for debugging rate limit triggers

## Component 2: iOS Reconnect Flow

### Current Flow (Problematic)

```
Health check fails → Alert → Navigate to CodespaceView →
.task runs with stale data → Shows "Access My Codespace" →
SSE fails → Setup view
```

### New Flow

```
Health check fails → Alert with "Reconnect" button →
User clicks Reconnect →
fetchUserStatus(forceRefresh: true) →
Worker verifies with GitHub (rate-limited) →
Update installer state →
Navigate to CodespaceView →
View renders with fresh data
```

### WorkspacesView Implementation

**Location:** `xcode/catnip/Views/WorkspacesView.swift` (health check alert handler)

```swift
// In the health check failure alert
.alert("Connection Lost", isPresented: $showReconnectAlert) {
    Button("Reconnect") {
        Task {
            // CRITICAL: Refresh user status BEFORE navigation
            // This triggers worker verification with ?refresh=true
            // Rate-limited to prevent abuse (10s server, 10s client)
            do {
                try await installer.fetchUserStatus(forceRefresh: true)
                NSLog("✅ Refreshed user status before reconnect")
            } catch {
                NSLog("⚠️ Failed to refresh status: \(error)")
                // Continue anyway - user will see current state
                // Graceful degradation if network fails
            }

            // Navigate back to CodespaceView with fresh data
            await MainActor.run {
                navigateToCodespace = true
            }
        }
    }

    Button("Cancel", role: .cancel) { }
}
```

### CatnipInstaller Enhancement

**Location:** `xcode/catnip/Services/CatnipInstaller.swift`

```swift
class CatnipInstaller: ObservableObject {
    @Published var userStatus: UserStatus?

    // Client-side rate limiting state
    private var lastRefreshTime: Date?

    func fetchUserStatus(forceRefresh: Bool = false) async throws {
        // CLIENT-SIDE RATE LIMITING (10 second minimum)
        // Note: Server also enforces this limit, but we fail fast here
        // to avoid unnecessary network calls
        if forceRefresh, let lastRefresh = lastRefreshTime {
            let timeSinceRefresh = Date().timeIntervalSince(lastRefresh)
            if timeSinceRefresh < 10.0 {
                NSLog(
                    "⚠️ Client rate limit: Skipping refresh - only " +
                    "\(String(format: "%.1f", timeSinceRefresh))s since last refresh " +
                    "(min 10s required)"
                )
                return // Server would also reject this
            }
        }

        // Build URL with refresh parameter
        let url = forceRefresh
            ? "\(apiBase)/v1/user/status?refresh=true"
            : "\(apiBase)/v1/user/status"

        NSLog("🔄 Fetching user status (forceRefresh: \(forceRefresh))")

        // Track refresh time BEFORE the call to prevent race conditions
        if forceRefresh {
            lastRefreshTime = Date()
        }

        // Existing fetch logic...
        var request = URLRequest(url: URL(string: url)!)
        request.httpMethod = "GET"

        let (data, response) = try await URLSession.shared.data(for: request)

        guard let httpResponse = response as? HTTPURLResponse else {
            throw NSError(domain: "Invalid response", code: 0)
        }

        guard httpResponse.statusCode == 200 else {
            throw NSError(
                domain: "HTTP error",
                code: httpResponse.statusCode
            )
        }

        let status = try JSONDecoder().decode(UserStatus.self, from: data)

        await MainActor.run {
            self.userStatus = status
        }

        NSLog(
            "✅ User status updated: has_any_codespaces=\(status.hasAnyCodespaces)"
        )
    }
}
```

### Key Benefits

- **Refresh before navigation** ensures CodespaceView sees fresh state
- **Dual rate limiting** (client + server) prevents abuse
- **Graceful degradation** if refresh fails or is rate-limited
- **Clear audit logging** for debugging reconnect flow issues

## Component 3: Worker-Side SSE Setup Event Enrichment

### Enhanced SSE Setup Event

When the worker's SSE flow encounters a "setup needed" scenario (no codespace credentials found), it determines the appropriate next action based on the user's GitHub state.

**Location:** `worker/index.ts:1756` (`/v1/codespace` SSE endpoint)

### Event Structure

```typescript
// Old format (ambiguous)
sendEvent("setup", {
  message: "Setup required"
});

// New enriched format (actionable)
sendEvent("setup", {
  message: string,
  next_action: "install" | "launch" | "create_repo",
  total_repositories: number,
  repositories_with_catnip?: number
});
```

### Implementation Logic

```typescript
// In /v1/codespace SSE endpoint
// After determining no codespace credentials exist in CODESPACE_STORE

try {
  // Fetch user's repositories to determine next action
  const reposResponse = await fetch(
    `https://api.github.com/user/repos?per_page=30&sort=pushed&affiliation=owner,collaborator`,
    {
      headers: {
        Authorization: `Bearer ${accessToken}`,
        Accept: "application/vnd.github.v3+json",
        "User-Agent": "Catnip-Worker/1.0",
      },
    },
  );

  if (!reposResponse.ok) {
    // API error - fallback to safe default
    console.error(
      "Failed to fetch repos for setup guidance:",
      reposResponse.status,
    );
    sendEvent("setup", {
      message: "Setup required. Please add Catnip feature to your repository.",
      next_action: "install", // Safe default
      total_repositories: 0,
    });
    void writer.close();
    return;
  }

  const repos = (await reposResponse.json()) as Array<{
    id: number;
    name: string;
    archived: boolean;
    permissions?: { push: boolean };
  }>;

  // Filter to repos user can modify
  const writableRepos = repos.filter((r) => !r.archived && r.permissions?.push);

  if (writableRepos.length === 0) {
    // CASE 1: No repositories (or no writable repositories)
    console.log(`User ${username} has no writable repositories`);
    sendEvent("setup", {
      message: "Create a GitHub repository to get started with Catnip",
      next_action: "create_repo",
      total_repositories: 0,
      repositories_with_catnip: 0,
    });
  } else {
    // CASE 2: Has repositories
    // Default to "install" flow - iOS will fetch detailed repo info
    // and determine if any already have Catnip feature
    console.log(
      `User ${username} has ${writableRepos.length} writable repositories`,
    );
    sendEvent("setup", {
      message: "Add Catnip feature to a repository to continue",
      next_action: "install",
      total_repositories: writableRepos.length,
      // iOS CatnipInstaller will fetch full repo details with Catnip status
    });
  }
} catch (error) {
  console.error("Failed to determine next_action for setup:", error);
  // Fallback to safe default
  sendEvent("setup", {
    message: "Setup required",
    next_action: "install",
    total_repositories: 0,
  });
}

void writer.close();
```

### SSE Event Type Update

**Location:** `xcode/catnip/Services/SSEService.swift`

```swift
enum SSEEvent {
    case status(String)
    case success(String, String?)
    case error(String)
    case setup(String, nextAction: String) // UPDATED: Add nextAction parameter
    case multiple([CodespaceInfo])
}
```

### iOS Handling in CodespaceView

**Location:** `xcode/catnip/Views/CodespaceView.swift:558` (`handleSSEEvent`)

```swift
@MainActor
private func handleSSEEvent(_ event: SSEEvent) {
    switch event {
    // ... existing cases ...

    case .setup(let message, let nextAction):
        statusMessage = ""
        errorMessage = message
        sseService?.disconnect()
        sseService = nil

        NSLog("📋 Setup event received: nextAction=\(nextAction)")

        // Route based on worker's determination of next action
        switch nextAction {
        case "create_repo":
            // User has no repositories - show creation guidance
            phase = .createRepository

        case "launch":
            // User has repos with Catnip - show launch flow
            NSLog("🚀 Routing to launch codespace flow")
            repositoryListMode = .launch
            phase = .repositorySelection
            Task {
                do {
                    try await installer.fetchRepositories()
                } catch {
                    errorMessage = "Failed to load repositories: \(error.localizedDescription)"
                    phase = .connect
                }
            }

        case "install":
            // User has repos but needs to install Catnip
            NSLog("📦 Routing to install Catnip flow")
            repositoryListMode = .installation
            phase = .repositorySelection
            Task {
                do {
                    try await installer.fetchRepositories()
                } catch {
                    errorMessage = "Failed to load repositories: \(error.localizedDescription)"
                    phase = .connect
                }
            }

        default:
            // Unknown action - fallback to old setup view
            NSLog("⚠️ Unknown setup next_action: \(nextAction)")
            phase = .setup
        }
    }
}
```

### Key Benefits

- **Worker centralizes decision logic** based on actual GitHub state
- **iOS just follows instructions** - simpler client logic
- **Correct flow for every scenario** (zero repos, has repos, has Catnip)
- **Fallback to safe defaults** if API calls fail

## Component 4: Create Repository Guidance Screen

When the worker determines the user has zero repositories (`next_action: "create_repo"`), iOS shows a friendly onboarding screen instead of technical setup instructions.

### New Phase

**Location:** `xcode/catnip/Views/CodespaceView.swift:11`

```swift
enum CodespacePhase {
    case connect
    case connecting
    case setup  // Deprecated - rarely used, fallback only
    case createRepository  // NEW - friendly onboarding for zero repos
    case selection
    case repositorySelection
    case installing
    case creatingCodespace
    case error
}
```

### View Implementation

**Location:** `xcode/catnip/Views/CodespaceView.swift` (add new computed property)

```swift
private var createRepositoryView: some View {
    ScrollView {
        VStack(spacing: 24) {
            Spacer()

            // Welcoming icon
            Image(systemName: "plus.rectangle.on.folder")
                .font(.system(size: 60))
                .foregroundStyle(.accentColor)

            VStack(spacing: 12) {
                Text("Create Your First Repository")
                    .font(.title2.weight(.semibold))
                    .multilineTextAlignment(.center)

                Text("Catnip needs a GitHub repository to work with. Create one to get started with agentic coding on your mobile device.")
                    .font(.body)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal)
            }

            Spacer()

            VStack(spacing: 12) {
                // Primary action - Create on GitHub
                Button {
                    if let url = URL(string: "https://github.com/new") {
                        UIApplication.shared.open(url)
                    }
                } label: {
                    HStack {
                        Image(systemName: "plus.circle.fill")
                        Text("Create Repository on GitHub")
                    }
                }
                .buttonStyle(ProminentButtonStyle(isDisabled: false))

                // Secondary action - Refresh to check
                Button {
                    Task {
                        do {
                            // Force refresh both user status and repositories
                            // This will re-check GitHub state after user creates repo
                            try await installer.fetchUserStatus(forceRefresh: true)
                            try await installer.fetchRepositories(forceRefresh: true)

                            // After refresh, determine next flow
                            await MainActor.run {
                                if installer.repositories.isEmpty {
                                    // Still no repos - show error
                                    errorMessage = "No repositories found yet. Create one on GitHub and try again."
                                } else {
                                    // Success! Navigate to install flow
                                    NSLog("✅ User now has \(installer.repositories.count) repositories")
                                    repositoryListMode = .installation
                                    phase = .repositorySelection
                                }
                            }
                        } catch {
                            await MainActor.run {
                                errorMessage = "Failed to check repositories: \(error.localizedDescription)"
                            }
                        }
                    }
                } label: {
                    HStack {
                        Image(systemName: "arrow.clockwise")
                        Text("I Created a Repository")
                    }
                }
                .buttonStyle(SecondaryButtonStyle(isDisabled: false))
            }
            .padding(.horizontal, 20)

            // Show error if refresh found no repos
            if !errorMessage.isEmpty {
                HStack(spacing: 10) {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .foregroundStyle(Color.orange)
                    Text(errorMessage)
                        .font(.subheadline)
                    Spacer()
                }
                .foregroundStyle(Color.orange)
                .padding(12)
                .background(Color.orange.opacity(0.08))
                .clipShape(RoundedRectangle(cornerRadius: 10))
                .padding(.horizontal, 20)
            }
        }
        .padding()
    }
    .scrollBounceBehavior(.basedOnSize)
    .background(Color(uiColor: .systemGroupedBackground))
}
```

### Body Integration

**Location:** `xcode/catnip/Views/CodespaceView.swift:56`

```swift
var body: some View {
    ZStack {
        if phase == .createRepository {
            createRepositoryView  // NEW
        } else if phase == .setup {
            setupView  // Fallback only
        } else if phase == .selection {
            selectionView
        // ... rest of phases
        }
    }
    // ... rest of view modifiers
}
```

### CatnipInstaller forceRefresh Support

**Location:** `xcode/catnip/Services/CatnipInstaller.swift`

```swift
// Add forceRefresh parameter to fetchRepositories
func fetchRepositories(forceRefresh: Bool = false) async throws {
    let url = forceRefresh
        ? "\(apiBase)/v1/repositories?refresh=true"  // If we add refresh support
        : "\(apiBase)/v1/repositories"

    // Existing fetch logic...
}
```

### Key Benefits

- **Friendly, welcoming tone** for new users (vs technical Setup view)
- **Clear call-to-action** to create repository with direct link
- **Self-service flow** - user can verify and advance after creating repo
- **Graceful error handling** if still no repos after refresh

## Testing Strategy

### Unit Tests (Worker)

```typescript
// Test rate limiting
describe("verification cache rate limiting", () => {
  it("ignores refresh=true if < 10 seconds since last refresh", async () => {
    // First refresh succeeds
    await fetch("/v1/user/status?refresh=true");

    // Second refresh within 10s is ignored
    const response = await fetch("/v1/user/status?refresh=true");
    // Should use cached data, not call GitHub API
  });

  it("allows refresh=true after 10 seconds", async () => {
    await fetch("/v1/user/status?refresh=true");
    await sleep(10000);
    const response = await fetch("/v1/user/status?refresh=true");
    // Should verify with GitHub API
  });

  it("verifies if cache is older than 60 seconds", async () => {
    await fetch("/v1/user/status"); // Prime cache
    await sleep(61000);
    const response = await fetch("/v1/user/status"); // No refresh param
    // Should verify with GitHub API due to stale cache
  });
});

// Test next_action determination
describe("SSE setup event next_action", () => {
  it("returns create_repo for users with no repositories", async () => {
    // Mock GitHub API to return empty repo list
    const event = await getSetupEvent(mockUserWithNoRepos);
    expect(event.next_action).toBe("create_repo");
  });

  it("returns install for users with repositories", async () => {
    const event = await getSetupEvent(mockUserWithRepos);
    expect(event.next_action).toBe("install");
  });
});
```

### Integration Tests (iOS)

```swift
// Test reconnect flow
func testReconnectRefreshesState() async throws {
    // Setup: User has a codespace stored
    // Action: Delete codespace in GitHub, trigger reconnect
    // Assert: User status refreshes, shows correct UI
}

// Test create repository flow
func testCreateRepositoryFlowForNewUser() async throws {
    // Setup: Mock user with zero repositories
    // Action: Navigate through create repo screen
    // Assert: Shows create_repo guidance, can refresh and advance
}

// Test rate limiting
func testRapidReconnectsAreRateLimited() async throws {
    // Action: Trigger reconnect multiple times rapidly
    // Assert: Only first call uses refresh=true, rest use cached data
    // Verify via network monitoring
}
```

### Manual Testing Scenarios

1. **Stale codespace reconnect:**
   - Create codespace via app
   - Delete codespace via GitHub web UI
   - Trigger health check failure and click Reconnect
   - Verify: Should show "Launch New Codespace" or "Install Catnip" flow (not "Access My Codespace")

2. **Zero repository new user:**
   - Create fresh GitHub account with no repos
   - Login to Catnip app
   - Verify: Shows "Create Repository" guidance screen
   - Create repo via link
   - Click "I Created a Repository"
   - Verify: Advances to "Install Catnip" flow

3. **Rate limiting:**
   - Trigger reconnect
   - Immediately trigger reconnect again (< 10s)
   - Verify: Second reconnect uses cached data (check logs)
   - Wait 10 seconds
   - Trigger reconnect again
   - Verify: Third reconnect refreshes from GitHub (check logs)

## Migration Plan

### Phase 1: Worker Updates (No Breaking Changes)

1. Add verification cache structure to CODESPACE_STORE Durable Object
2. Implement cache helpers (get/update)
3. Update `/v1/user/status` with caching and rate limiting
4. Add `next_action` field to SSE setup event (iOS ignores until updated)
5. Deploy worker

### Phase 2: iOS Updates

1. Update `SSEEvent` enum with `nextAction` parameter
2. Add `lastRefreshTime` to `CatnipInstaller`
3. Implement `fetchUserStatus(forceRefresh:)` with client-side rate limiting
4. Add `createRepositoryView` to `CodespaceView`
5. Update `handleSSEEvent` to route based on `next_action`
6. Update WorkspacesView reconnect alert to call `fetchUserStatus(forceRefresh: true)`
7. Deploy iOS app

### Rollback Strategy

- Worker changes are backward compatible (existing clients ignore new fields)
- If issues arise, can disable refresh behavior by removing `?refresh=true` calls
- Cache can be cleared by restarting CODESPACE_STORE Durable Object

## Future Enhancements

1. **Webhook-based invalidation:** Instead of verification cache TTL, invalidate cache when GitHub webhook reports codespace deletion
2. **Repository cache:** Cache repository lists with Catnip status to avoid repeated devcontainer.json checks
3. **Optimistic UI:** Show "Launch Codespace" flow immediately, verify in background
4. **Analytics:** Track reconnect frequency, rate limit hits, and next_action distribution

## Open Questions

1. **View lifecycle:** Does CodespaceView reinitialize when navigating back from WorkspacesView, or is it cached? (Affects where to refresh)
2. **Notification integration:** Should reconnect trigger a notification if app is backgrounded?
3. **Repository refresh caching:** Should we also cache repository lists with `?refresh=true` support?

## Appendix: API Contracts

### `/v1/user/status` Query Parameters

| Parameter | Type    | Description                                                                    |
| --------- | ------- | ------------------------------------------------------------------------------ |
| `refresh` | boolean | If `"true"`, verify codespaces with GitHub API (rate-limited to 10s intervals) |

### SSE Setup Event Format

```typescript
{
  event: "setup",
  data: {
    message: string,              // User-facing message
    next_action: "install" | "launch" | "create_repo",  // Routing hint
    total_repositories: number,   // Count of user's repos
    repositories_with_catnip?: number  // Optional, for future use
  }
}
```

### Verification Cache Storage

Stored in CODESPACE_STORE Durable Object under key `verification-cache:${username}`:

```json
{
  "username": "octocat",
  "lastVerified": 1699564800000,
  "lastRefreshRequest": 1699564790000,
  "verifiedCodespaces": [
    {
      "githubToken": "...",
      "githubUser": "octocat",
      "codespaceName": "octocat-repo-abc123",
      "createdAt": 1699564700000,
      "updatedAt": 1699564750000
    }
  ]
}
```
