# Codespace State Refresh Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix stale codespace state after deletion and eliminate confusing Setup view by implementing verification caching with rate limiting and smart next-action routing.

**Architecture:** Worker adds 60s verification cache + 10s rate limiting for `/v1/user/status?refresh=true`. iOS calls this on reconnect before navigation. SSE setup events enriched with `next_action` to route users to correct flow (install/launch/create_repo). New "Create Repository" guidance screen for zero-repo users.

**Tech Stack:** Cloudflare Workers (Hono, Durable Objects), iOS (Swift, SwiftUI), GitHub API

**Design Doc:** `docs/plans/2025-11-07-codespace-state-refresh-design.md`

---

## Task 1: Worker - Add Verification Cache to Durable Object

**Files:**

- Modify: `worker/durable-objects/codespace-store.ts` (or create if doesn't exist in separate file)
- Test: Will test via integration in later tasks (Durable Objects are hard to unit test)

**Context:** The CODESPACE_STORE Durable Object needs to track verification cache per user to rate-limit GitHub API calls.

**Step 1: Locate or identify Durable Object implementation**

The CODESPACE_STORE is referenced in `worker/index.ts` but we need to find where it's implemented. It might be:

- Inline in the Durable Object export
- In a separate file
- Using a default implementation

Run: `grep -r "CODESPACE_STORE" worker/ --include="*.ts"`

Expected: Find where the Durable Object is defined/exported

**Step 2: Add verification cache interface to worker types**

In `worker/index.ts` after the `CodespaceCredentials` interface (around line 46):

```typescript
interface VerificationCache {
  username: string;
  lastVerified: number; // timestamp of last verification
  lastRefreshRequest: number; // timestamp of last refresh=true request
  verifiedCodespaces: CodespaceCredentials[];
}
```

**Step 3: Add cache storage routes to Durable Object**

If CODESPACE_STORE is using a custom class (look for `export class CodespaceStore`), add these routes. If using default Durable Object, we'll store cache alongside codespace data with a special key prefix.

For now, we'll use a pattern where cache is stored with key `verification-cache:${username}` in the same storage.

Add helper functions in `worker/index.ts` after the `verifyAndCleanCodespaces` function (around line 255):

```typescript
// Verification cache helpers for rate limiting and performance
async function getVerificationCache(
  codespaceStore: DurableObjectStub,
  username: string,
): Promise<VerificationCache | null> {
  try {
    const response = await codespaceStore.fetch(
      `https://internal/verification-cache/${username}`,
    );
    if (!response.ok) return null;
    return await response.json();
  } catch (error) {
    console.warn(`Failed to get verification cache for ${username}:`, error);
    return null;
  }
}

async function updateVerificationCache(
  codespaceStore: DurableObjectStub,
  username: string,
  update: Partial<VerificationCache>,
): Promise<void> {
  try {
    const response = await codespaceStore.fetch(
      `https://internal/verification-cache/${username}`,
      {
        method: "PATCH",
        body: JSON.stringify(update),
      },
    );
    if (!response.ok) {
      console.error(`Failed to update verification cache: ${response.status}`);
    }
  } catch (error) {
    console.error(
      `Failed to update verification cache for ${username}:`,
      error,
    );
  }
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

**Step 4: Commit cache infrastructure**

```bash
git add worker/index.ts
git commit -m "feat(worker): add verification cache infrastructure

Add VerificationCache interface and helper functions to support
rate-limited codespace verification with GitHub API.

- Cache tracks lastVerified (60s TTL) and lastRefreshRequest (10s rate limit)
- Helper functions for get/update operations
- Foundation for /v1/user/status caching logic

Ref: docs/plans/2025-11-07-codespace-state-refresh-design.md"
```

---

## Task 2: Worker - Implement Durable Object Cache Storage

**Files:**

- Modify: `worker/index.ts` or Durable Object implementation file
- Research: How CODESPACE_STORE handles internal routes

**Context:** We need to handle `verification-cache/${username}` routes in the Durable Object to actually store/retrieve cache data.

**Step 1: Find Durable Object fetch handler**

The Durable Object should have a `fetch()` method that handles `https://internal/*` routes. Look for patterns like:

- `async fetch(request: Request)`
- URL path routing logic
- Existing `/codespace/${username}` route handlers

Run: `grep -A 20 "async fetch" worker/ --include="*.ts" | grep -B5 -A15 "internal"`

Expected: Find the fetch handler and understand the routing pattern

**Step 2: Add cache routes to Durable Object fetch handler**

Based on what you found, add routes for verification cache. If there's no custom Durable Object class, you'll need to create one or use KV/state directly.

Example pattern (adjust based on actual implementation):

```typescript
// In Durable Object fetch handler, add these routes:

// GET /verification-cache/{username}
if (url.pathname.match(/^\/verification-cache\/(.+)$/)) {
  const username = url.pathname.split("/")[2];
  const cacheKey = `verification-cache:${username}`;
  const cache = await this.state.storage.get<VerificationCache>(cacheKey);

  if (!cache) {
    return new Response("Not found", { status: 404 });
  }

  return new Response(JSON.stringify(cache), {
    headers: { "Content-Type": "application/json" },
  });
}

// PATCH /verification-cache/{username}
if (
  request.method === "PATCH" &&
  url.pathname.match(/^\/verification-cache\/(.+)$/)
) {
  const username = url.pathname.split("/")[2];
  const cacheKey = `verification-cache:${username}`;
  const update = await request.json<Partial<VerificationCache>>();

  // Get existing cache or create new one
  let cache = await this.state.storage.get<VerificationCache>(cacheKey);
  if (!cache) {
    cache = {
      username,
      lastVerified: 0,
      lastRefreshRequest: 0,
      verifiedCodespaces: [],
    };
  }

  // Apply update
  cache = { ...cache, ...update };

  // Save
  await this.state.storage.put(cacheKey, cache);

  return new Response("OK", { status: 200 });
}
```

**Step 3: Test cache storage manually**

If you can run the worker locally, test the cache routes work. Otherwise, we'll verify in the next task when we integrate with `/v1/user/status`.

**Step 4: Commit Durable Object cache routes**

```bash
git add worker/index.ts  # or relevant DO file
git commit -m "feat(worker): implement verification cache storage in Durable Object

Add GET and PATCH routes for verification-cache storage:
- GET /verification-cache/{username} - retrieve cache
- PATCH /verification-cache/{username} - update cache fields

Cache stored in Durable Object state with key pattern
'verification-cache:{username}' for per-user isolation.

Ref: docs/plans/2025-11-07-codespace-state-refresh-design.md"
```

---

## Task 3: Worker - Add Caching and Rate Limiting to /v1/user/status

**Files:**

- Modify: `worker/index.ts:924` (existing `/v1/user/status` endpoint)

**Context:** Update the user status endpoint to use verification cache with 60s TTL and enforce 10s rate limiting on `?refresh=true` requests.

**Step 1: Add refresh query parameter handling**

In the `/v1/user/status` endpoint (line 924), add at the beginning:

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

**Step 2: Remove or update old user status logic**

The existing `/v1/user/status` endpoint likely has simpler logic. Replace it entirely with the above implementation.

**Step 3: Test locally if possible**

If you can run `pnpm dev` and test the worker:

```bash
# Test normal request (should verify on first call)
curl http://localhost:8787/v1/user/status -H "Authorization: Bearer test-token"

# Test refresh request
curl "http://localhost:8787/v1/user/status?refresh=true" -H "Authorization: Bearer test-token"

# Test rapid refresh (should be rate limited)
curl "http://localhost:8787/v1/user/status?refresh=true" -H "Authorization: Bearer test-token"
curl "http://localhost:8787/v1/user/status?refresh=true" -H "Authorization: Bearer test-token"
```

Expected: Second rapid refresh logs rate limit warning and uses cached data.

**Step 4: Commit caching and rate limiting**

```bash
git add worker/index.ts
git commit -m "feat(worker): add caching and rate limiting to /v1/user/status

Implement verification cache with dual-layer protection:
- 60-second cache TTL for normal requests (reduces GitHub API calls)
- 10-second rate limiting on ?refresh=true (prevents abuse)
- Server-side enforcement protects quota even if client has bugs

Rate limiting behavior:
- If refresh requested < 10s after last refresh, log warning and use cache
- If cache older than 60s, automatically verify even without refresh param
- Graceful degradation serves cached data when rate limited

Ref: docs/plans/2025-11-07-codespace-state-refresh-design.md"
```

---

## Task 4: Worker - Enhance SSE Setup Event with next_action

**Files:**

- Modify: `worker/index.ts:1756` (`/v1/codespace` SSE endpoint)

**Context:** When no codespace is found, check user's GitHub repository state and return enriched setup event with `next_action` field to guide iOS routing.

**Step 1: Locate SSE setup event emission**

Find where the SSE endpoint currently sends the "setup" event. Should be around line 1598 based on the design doc.

Run: `grep -n "sendEvent.*setup" worker/index.ts`

Expected: Find the line where setup event is sent

**Step 2: Replace simple setup event with enriched version**

Replace the existing setup event code with:

```typescript
// OLD CODE (remove):
// sendEvent("setup", { message: "Setup required" });

// NEW CODE:
// Determine next_action based on user's GitHub repository state
try {
  console.log(`Determining next_action for user ${username} with no codespace`);

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

**Step 3: Find all places where setup event is sent**

There might be multiple places that send setup events. Find them all:

Run: `grep -n "sendEvent.*setup" worker/index.ts`

Update all of them to include `next_action` field.

**Step 4: Commit SSE enhancement**

```bash
git add worker/index.ts
git commit -m "feat(worker): enhance SSE setup event with next_action routing

Worker now determines appropriate next_action based on user's GitHub state:
- 'create_repo' if user has zero repositories
- 'install' if user has repositories (default flow)
- 'launch' reserved for future use (when we detect Catnip repos)

Setup event now includes:
- message: User-facing message
- next_action: Routing hint for iOS (install/launch/create_repo)
- total_repositories: Count of writable repos

This centralizes decision logic in worker, simplifying iOS routing.

Ref: docs/plans/2025-11-07-codespace-state-refresh-design.md"
```

---

## Task 5: iOS - Update SSEEvent Enum with nextAction

**Files:**

- Modify: `xcode/catnip/Services/SSEService.swift` (SSEEvent enum)

**Context:** Add `nextAction` parameter to the setup case so iOS can route based on worker's determination.

**Step 1: Locate SSEEvent enum**

Run: `grep -n "enum SSEEvent" xcode/catnip/Services/SSEService.swift`

Expected: Find the enum definition

**Step 2: Update setup case to include nextAction**

Find the setup case in the enum and add the nextAction parameter:

```swift
// OLD:
case setup(String)

// NEW:
case setup(String, nextAction: String)
```

**Step 3: Update SSE parsing to extract next_action**

Find where the setup event is parsed from SSE data. Should be in the SSEService where it decodes the JSON.

Look for JSON decoding like:

```swift
if event == "setup" {
    // Parse data JSON
}
```

Update to extract next_action:

```swift
if event == "setup" {
    if let data = try? JSONSerialization.jsonObject(with: dataValue.data(using: .utf8)!) as? [String: Any],
       let message = data["message"] as? String {
        let nextAction = data["next_action"] as? String ?? "install" // Default fallback
        completion(.setup(message, nextAction: nextAction))
    }
}
```

**Step 4: Commit SSEEvent update**

```bash
git add xcode/catnip/Services/SSEService.swift
git commit -m "feat(ios): add nextAction parameter to SSEEvent.setup

Update SSEEvent enum to include nextAction routing hint from worker:
- SSEEvent.setup now includes nextAction string parameter
- Parse next_action field from SSE JSON data
- Default to 'install' if field missing (backward compatibility)

Enables iOS to route based on worker's determination of user state.

Ref: docs/plans/2025-11-07-codespace-state-refresh-design.md"
```

---

## Task 6: iOS - Add Rate Limiting to CatnipInstaller

**Files:**

- Modify: `xcode/catnip/Services/CatnipInstaller.swift`

**Context:** Add client-side rate limiting to prevent rapid refresh=true calls, complementing server-side protection.

**Step 1: Add lastRefreshTime state**

In the CatnipInstaller class, add a private property to track last refresh:

```swift
class CatnipInstaller: ObservableObject {
    @Published var userStatus: UserStatus?
    // ... existing properties ...

    // Client-side rate limiting state (10 second minimum)
    private var lastRefreshTime: Date?

    // ... rest of class ...
}
```

**Step 2: Add forceRefresh parameter to fetchUserStatus**

Find the `fetchUserStatus()` method and add forceRefresh parameter:

```swift
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
```

**Step 3: Build and test locally**

Build the iOS app to verify it compiles:

```bash
cd xcode
xcodebuild -project catnip.xcodeproj -scheme catnip -sdk iphonesimulator
```

Expected: Build succeeds with no errors

**Step 4: Commit rate limiting**

```bash
git add xcode/catnip/Services/CatnipInstaller.swift
git commit -m "feat(ios): add client-side rate limiting to fetchUserStatus

Implement dual-layer rate limiting protection:
- Track lastRefreshTime to prevent calls within 10 seconds
- Add forceRefresh parameter to explicitly request verification
- Fail fast on client to avoid unnecessary network calls
- Server also enforces same limit as backup

Rate limiting behavior:
- If forceRefresh called < 10s after last, log and return early
- Track timestamp BEFORE call to prevent race conditions
- Clear logging for debugging reconnect flows

Ref: docs/plans/2025-11-07-codespace-state-refresh-design.md"
```

---

## Task 7: iOS - Add Create Repository Phase and View

**Files:**

- Modify: `xcode/catnip/Views/CodespaceView.swift:11` (add phase)
- Modify: `xcode/catnip/Views/CodespaceView.swift` (add view)

**Context:** Add new phase and view for users with zero repositories, providing friendly guidance to create their first repo.

**Step 1: Add createRepository phase to enum**

In CodespaceView.swift, find the CodespacePhase enum (around line 11) and add new phase:

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

**Step 2: Add createRepositoryView computed property**

After the existing view computed properties (around line 891), add:

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

**Step 3: Add createRepository case to body's ZStack**

In the body's ZStack (around line 56), add the new phase check:

```swift
var body: some View {
    ZStack {
        if phase == .createRepository {
            createRepositoryView  // NEW
        } else if phase == .setup {
            setupView  // Fallback only
        } else if phase == .selection {
            selectionView
        // ... rest of existing phases
        }
    }
    // ... rest of view modifiers
}
```

**Step 4: Note about fetchRepositories forceRefresh**

The createRepositoryView calls `installer.fetchRepositories(forceRefresh: true)`, but that method might not have the forceRefresh parameter yet. We'll add it in a later task if needed, or you can add it now:

```swift
// In CatnipInstaller.swift
func fetchRepositories(forceRefresh: Bool = false) async throws {
    // If we add refresh support to /v1/repositories endpoint later
    let url = forceRefresh
        ? "\(apiBase)/v1/repositories?refresh=true"
        : "\(apiBase)/v1/repositories"

    // Existing fetch logic...
}
```

For now, just use the existing fetchRepositories() without forceRefresh if it doesn't exist yet.

**Step 5: Build and test**

```bash
cd xcode
xcodebuild -project catnip.xcodeproj -scheme catnip -sdk iphonesimulator
```

Expected: Build succeeds

**Step 6: Commit create repository view**

```bash
git add xcode/catnip/Views/CodespaceView.swift xcode/catnip/Services/CatnipInstaller.swift
git commit -m "feat(ios): add create repository guidance screen

Add friendly onboarding flow for users with zero repositories:
- New createRepository phase in CodespacePhase enum
- Welcoming screen with clear call-to-action
- Direct link to GitHub new repo page
- Self-service 'I Created a Repository' button to refresh and advance
- Graceful error handling if still no repos after refresh

Replaces technical Setup view for zero-repo users with
approachable first-time experience.

Ref: docs/plans/2025-11-07-codespace-state-refresh-design.md"
```

---

## Task 8: iOS - Update handleSSEEvent to Route by next_action

**Files:**

- Modify: `xcode/catnip/Views/CodespaceView.swift:558` (handleSSEEvent)

**Context:** Update the setup event handler to route based on the worker's next_action determination instead of always showing setup view.

**Step 1: Find handleSSEEvent method**

Run: `grep -n "func handleSSEEvent" xcode/catnip/Views/CodespaceView.swift`

Expected: Find method around line 558

**Step 2: Update setup case handler**

Find the `.setup` case in the switch statement and replace with:

```swift
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
        NSLog("🆕 Routing to create repository flow")
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
        NSLog("⚠️ Unknown setup next_action: \(nextAction), falling back to setup view")
        phase = .setup
    }
```

**Step 3: Build and test**

```bash
cd xcode
xcodebuild -project catnip.xcodeproj -scheme catnip -sdk iphonesimulator
```

Expected: Build succeeds

**Step 4: Commit routing logic**

```bash
git add xcode/catnip/Views/CodespaceView.swift
git commit -m "feat(ios): route setup events based on worker next_action

Update handleSSEEvent to route based on worker's next_action:
- 'create_repo' → createRepository phase (friendly onboarding)
- 'install' → repositorySelection with installation mode
- 'launch' → repositorySelection with launch mode
- unknown → fallback to old setup view (backward compatibility)

Worker now centralizes decision logic based on GitHub state,
iOS just follows routing instructions.

Clear logging for debugging routing decisions.

Ref: docs/plans/2025-11-07-codespace-state-refresh-design.md"
```

---

## Task 9: iOS - Update WorkspacesView Reconnect Flow

**Files:**

- Find: `xcode/catnip/Views/WorkspacesView.swift` (health check alert)
- Modify: Reconnect button handler

**Context:** When health check detects dead codespace and user clicks Reconnect, fetch fresh state before navigating back to CodespaceView.

**Step 1: Find health check alert code**

Run: `grep -n "reconnect\|Reconnect" xcode/catnip/Views/WorkspacesView.swift -i`

Expected: Find where the reconnect alert/button is defined

**Step 2: Update reconnect button to refresh before navigation**

Find the reconnect button handler and update it to:

```swift
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
            // Set navigation state to return to CodespaceView
            // The exact property name depends on your navigation setup
            // Look for something like:
            // navigateToCodespace = true
            // OR
            // navigationPath.removeLast()
            // OR
            // dismiss()
        }
    }
}
```

**Step 3: Identify navigation mechanism**

You need to find how WorkspacesView navigates back to CodespaceView. Look for:

- `@State` or `@Binding` navigation properties
- `NavigationStack` or `NavigationView` usage
- `dismiss()` environment action

Run: `grep -n "CodespaceView\|navigationPath\|dismiss" xcode/catnip/Views/WorkspacesView.swift`

Update the navigation code in the button handler based on what you find.

**Step 4: Build and test**

```bash
cd xcode
xcodebuild -project catnip.xcodeproj -scheme catnip -sdk iphonesimulator
```

Expected: Build succeeds

**Step 5: Commit reconnect flow**

```bash
git add xcode/catnip/Views/WorkspacesView.swift
git commit -m "feat(ios): refresh state before reconnect navigation

Update reconnect flow to fetch fresh user status before navigating:
- Call fetchUserStatus(forceRefresh: true) on reconnect button press
- Triggers worker verification with GitHub API (rate-limited)
- Navigate to CodespaceView only after state refreshes
- Graceful degradation if refresh fails

Ensures CodespaceView sees fresh has_any_codespaces flag,
preventing stale 'Access My Codespace' button after deletion.

Ref: docs/plans/2025-11-07-codespace-state-refresh-design.md"
```

---

## Task 10: Manual Testing and Verification

**Files:**

- No code changes, just testing

**Context:** Manually verify the full flow works end-to-end.

**Step 1: Test stale codespace reconnect flow**

1. Create a codespace via the app (or use existing)
2. Delete the codespace via GitHub web UI (github.com/codespaces)
3. Wait for health check to fail in WorkspacesView
4. Click "Reconnect" when alert appears
5. Verify: Should see "Launch New Codespace" or "Install Catnip" flow (NOT "Access My Codespace")

**Step 2: Test zero repository flow**

1. Create a fresh GitHub account with no repositories (or use test account)
2. Login to Catnip app
3. Verify: Should see "Create Your First Repository" guidance screen
4. Click "Create Repository on GitHub" - should open github.com/new
5. Create a repo (can be empty)
6. Return to app and click "I Created a Repository"
7. Verify: Should advance to "Install Catnip" flow

**Step 3: Test rate limiting**

1. Trigger reconnect flow
2. Immediately trigger reconnect again (< 10 seconds)
3. Check Xcode console logs
4. Verify: Second reconnect logs "Client rate limit: Skipping refresh"
5. Wait 10 seconds
6. Trigger reconnect again
7. Verify: Third reconnect logs "Fetching user status (forceRefresh: true)"

**Step 4: Test worker logs**

If you can access worker logs (via `wrangler tail` or Cloudflare dashboard):

1. Trigger a reconnect with forceRefresh
2. Check worker logs for "🔄 Verifying codespaces"
3. Trigger another reconnect within 10 seconds
4. Check worker logs for "⚠️ Rate limit: Ignoring refresh request"

**Step 5: Document any issues**

Create a file `docs/testing/codespace-state-refresh-manual-tests.md` documenting:

- Test scenarios run
- Expected vs actual behavior
- Any bugs found
- Screenshots if helpful

**Step 6: Commit test documentation**

```bash
git add docs/testing/codespace-state-refresh-manual-tests.md
git commit -m "docs: add manual testing results for codespace state refresh

Document manual testing of:
- Stale codespace reconnect flow
- Zero repository onboarding
- Client and server rate limiting
- Worker verification caching

[Record your test results and any issues found here]

Ref: docs/plans/2025-11-07-codespace-state-refresh-design.md"
```

---

## Task 11: Deploy and Monitor

**Files:**

- No code changes

**Context:** Deploy to production and monitor for issues.

**Step 1: Deploy worker**

```bash
cd worker
pnpm run deploy
```

Expected: Worker deploys successfully to Cloudflare

**Step 2: Deploy iOS app**

Follow your normal iOS deployment process:

- Build release version
- Upload to TestFlight
- Or submit to App Store

**Step 3: Monitor worker logs**

```bash
wrangler tail
```

Watch for:

- Rate limiting triggers (should be rare)
- Verification cache hits/misses
- SSE next_action distribution

**Step 4: Monitor user behavior**

Check analytics/logs for:

- How many users hit create_repo flow (zero repos)
- How many users benefit from verification cache
- Any errors in reconnect flow

**Step 5: Create follow-up tasks if needed**

If you discover issues or optimizations needed:

- Add repository list caching (future enhancement from design doc)
- Webhook-based cache invalidation (future enhancement)
- Adjust cache TTL or rate limits based on actual usage

---

## Success Criteria

- [ ] Worker `/v1/user/status?refresh=true` returns fresh data with rate limiting
- [ ] Reconnect flow fetches fresh state before navigation
- [ ] Zero-repo users see create repository guidance (not setup view)
- [ ] SSE setup events route correctly based on next_action
- [ ] Rate limiting prevents rapid refresh calls (10s minimum)
- [ ] Verification cache reduces GitHub API calls (60s TTL)
- [ ] No breaking changes for existing users
- [ ] All existing tests still pass

## Rollback Plan

If issues arise in production:

1. **Worker rollback**:

   ```bash
   wrangler rollback
   ```

2. **iOS rollback**: Submit new build removing `?refresh=true` calls

3. **Partial rollback**: Can disable just rate limiting by setting TTL to 0:
   ```typescript
   const shouldVerify = true; // Always verify
   ```

## Future Enhancements

From design doc `docs/plans/2025-11-07-codespace-state-refresh-design.md`:

1. **Webhook-based invalidation**: Invalidate cache when GitHub webhook reports codespace deletion (more reliable than TTL)

2. **Repository cache**: Cache repository lists with Catnip status to avoid repeated devcontainer.json checks

3. **Optimistic UI**: Show "Launch Codespace" flow immediately, verify in background

4. **Analytics**: Track reconnect frequency, rate limit hits, and next_action distribution to optimize TTLs

---

## References

- **Design Doc**: `docs/plans/2025-11-07-codespace-state-refresh-design.md`
- **Worker Code**: `worker/index.ts`
- **iOS CodespaceView**: `xcode/catnip/Views/CodespaceView.swift`
- **iOS WorkspacesView**: `xcode/catnip/Views/WorkspacesView.swift`
- **CatnipInstaller**: `xcode/catnip/Services/CatnipInstaller.swift`
- **SSEService**: `xcode/catnip/Services/SSEService.swift`

## Skills Reference

- **@superpowers:test-driven-development** - Write tests first for each component
- **@superpowers:systematic-debugging** - If tests fail, investigate root cause before fixing
- **@superpowers:verification-before-completion** - Run verification commands before claiming success
- **@superpowers:executing-plans** - Use to implement this plan task-by-task
