//
//  WorkspaceDetailView.swift
//  catnip
//
//  Workspace detail screen with polling for updates
//

import SwiftUI
import MarkdownUI

enum WorkspacePhase {
    case loading
    case input
    case working
    case completed
    case error
}

struct WorkspaceDetailView: View {
    let workspaceId: String

    @StateObject private var poller: WorkspacePoller
    @State private var phase: WorkspacePhase = .loading
    @State private var prompt = ""
    @State private var showPromptSheet = false
    @State private var isSubmitting = false
    @State private var error = ""
    @State private var showFullDiff = false
    @State private var latestMessage: String?
    @State private var cachedDiff: WorktreeDiffResponse?
    @State private var pendingUserPrompt: String? // Store prompt we just sent before backend updates
    @State private var pendingUserPromptTimestamp: Date? // Track when prompt was sent for timeout
    @State private var isCreatingPR = false
    @State private var isUpdatingPR = false
    @State private var showingPRCreationSheet = false
    @State private var isLandscape = false
    @State private var showPortraitTerminal = false  // Show terminal in portrait mode

    // Codespace shutdown detection
    @State private var showShutdownAlert = false
    @State private var shutdownMessage: String?
    @Environment(\.dismiss) private var dismiss

    // CatnipInstaller for status refresh
    @StateObject private var installer = CatnipInstaller.shared

    @Environment(\.horizontalSizeClass) var horizontalSizeClass
    @Environment(\.verticalSizeClass) var verticalSizeClass
    @EnvironmentObject var authManager: AuthManager

    init(workspaceId: String, initialWorkspace: WorkspaceInfo? = nil, pendingPrompt: String? = nil) {
        self.workspaceId = workspaceId
        _poller = StateObject(wrappedValue: WorkspacePoller(workspaceId: workspaceId, initialWorkspace: initialWorkspace))

        // Set initial pending prompt if provided (e.g., from workspace creation flow)
        if let pendingPrompt = pendingPrompt, !pendingPrompt.isEmpty {
            _pendingUserPrompt = State(initialValue: pendingPrompt)
            NSLog("🔵 WorkspaceDetailView init with pending prompt: \(pendingPrompt.prefix(50))...")
        }

        if let initialWorkspace = initialWorkspace {
            NSLog("🔵 WorkspaceDetailView init with pre-loaded workspace: \(workspaceId), activity: \(initialWorkspace.claudeActivityState?.rawValue ?? "nil")")
        } else {
            NSLog("🟡 WorkspaceDetailView init WITHOUT pre-loaded workspace: \(workspaceId) - will fetch")
        }
    }

    private var workspace: WorkspaceInfo? {
        poller.workspace
    }

    /// Get the latest user prompt, preferring session data over workspace data
    private var effectiveLatestUserPrompt: String? {
        // First check session data (more up-to-date during active polling)
        if let prompt = poller.sessionData?.latestUserPrompt, !prompt.isEmpty {
            return prompt
        }
        // Fall back to workspace data
        return workspace?.latestUserPrompt
    }

    /// Get the latest Claude message, preferring session data over workspace data
    private var effectiveLatestMessage: String? {
        // First check session data (more up-to-date during active polling)
        if let msg = poller.sessionData?.latestMessage, !msg.isEmpty {
            return msg
        }
        // Fall back to workspace's latestClaudeMessage (from worktrees endpoint)
        if let msg = workspace?.latestClaudeMessage, !msg.isEmpty {
            return msg
        }
        // Final fallback to latestMessage state
        return latestMessage
    }

    /// Get session stats from session data
    private var sessionStats: SessionStats? {
        poller.sessionData?.stats
    }

    /// Get the effective session title, preferring session data over workspace data
    private var effectiveSessionTitle: String? {
        // First check session data (more up-to-date during active polling)
        if let title = poller.sessionData?.latestSessionTitle, !title.isEmpty {
            return title
        }
        // Fall back to workspace data
        return workspace?.latestSessionTitle
    }

    /// Get effective todos, preferring session data over workspace data
    private var effectiveTodos: [Todo]? {
        // First check session data (more up-to-date during active polling)
        if let todos = poller.sessionData?.todos, !todos.isEmpty {
            return todos
        }
        // Fall back to workspace data
        return workspace?.todos
    }

    /// Check if we have any session content to display
    /// Used to determine if we should show empty state vs completed state
    private var hasSessionContent: Bool {
        // Has user prompt
        if let prompt = effectiveLatestUserPrompt, !prompt.isEmpty {
            return true
        }
        // Has Claude message
        if let msg = effectiveLatestMessage, !msg.isEmpty {
            return true
        }
        // Has session title
        if let title = effectiveSessionTitle, !title.isEmpty {
            return true
        }
        // Has todos
        if let todos = effectiveTodos, !todos.isEmpty {
            return true
        }
        return false
    }

    private var navigationTitle: String {
        // Show session title if available (in both working and completed phases)
        if let title = effectiveSessionTitle, !title.isEmpty {
            // Truncate to first line or 50 chars
            let firstLine = title.components(separatedBy: .newlines).first ?? title
            return firstLine.count > 50 ? String(firstLine.prefix(50)) + "..." : firstLine
        }
        return workspace?.displayName ?? "Workspace"
    }

    private var mainContentView: some View {
        Group {
            // Show terminal in landscape or portrait terminal mode, normal UI otherwise
            if isLandscape {
                terminalView
            } else if showPortraitTerminal {
                portraitTerminalView
            } else {
                if phase == .loading {
                    loadingView
                } else if phase == .error || workspace == nil {
                    errorView
                } else {
                    contentView
                }
            }
        }
    }

    @ToolbarContentBuilder
    private var toolbarContent: some ToolbarContent {
        // Show terminal button when in portrait mode (not showing terminal)
        // Wrapped in context progress ring to show Claude's token usage
        if !isLandscape && !showPortraitTerminal {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    showPortraitTerminal = true
                } label: {
                    ContextProgressRing(contextTokens: sessionStats?.lastContextSizeTokens) {
                        Image(systemName: "terminal")
                            .font(.system(size: 11, weight: .medium))
                            .foregroundStyle(.primary)
                    }
                }
                .buttonStyle(.plain)
            }
        }
    }

    var body: some View {
        mainView
            .task {
                await loadWorkspace()
                poller.start()

                // Note: HealthCheckService is already running - WorkspacesView manages it as a singleton

                // Start PTY after workspace is loaded
                if let workspace = workspace {
                    Task {
                        do {
                            try await CatnipAPI.shared.startPTY(workspacePath: workspace.name, agent: "claude")
                            NSLog("✅ Started PTY for workspace: \(workspace.name)")
                        } catch {
                            NSLog("⚠️ Failed to start PTY: \(error)")
                            // Non-fatal - PTY will be created on-demand if needed
                        }
                    }
                }
            }
            .onDisappear {
                poller.stop()
                // Note: We don't stop HealthCheckService here because WorkspacesView manages it.
                // WorkspacesView is still in the navigation stack when we're viewing a workspace detail.
            }
            .onChange(of: horizontalSizeClass) {
                updateOrientation()
            }
            .onChange(of: verticalSizeClass) {
                updateOrientation()
            }
            .onAppear {
                updateOrientation()
            }
            .onChange(of: poller.workspace) {
                if let newWorkspace = poller.workspace {
                    NSLog("🔄 Workspace updated - activity: \(newWorkspace.claudeActivityState?.rawValue ?? "nil"), title: \(newWorkspace.latestSessionTitle?.prefix(30) ?? "nil")")
                    determinePhase(for: newWorkspace)
                } else {
                    NSLog("⚠️ Workspace updated to nil")
                }
            }
            .onChange(of: poller.error) {
                if let newError = poller.error {
                    // Filter out "cancelled" errors (Code -999) - these are normal when requests are cancelled
                    // to make new ones and are not actionable for users
                    if !newError.lowercased().contains("cancelled") {
                        error = newError
                    }
                }
            }
            .sheet(isPresented: $showPromptSheet) {
                PromptSheet(
                    isPresented: $showPromptSheet,
                    prompt: $prompt,
                    mode: .askForChanges,
                    isSubmitting: isSubmitting,
                    onSubmit: {
                        Task { await sendPrompt() }
                    }
                )
            }
            .sheet(isPresented: $showFullDiff) {
                NavigationStack {
                    WorkspaceDiffViewer(
                        workspaceId: workspaceId,
                        selectedFile: nil,
                        onClose: {
                            showFullDiff = false
                        },
                        onExpand: nil,
                        preloadedDiff: cachedDiff,
                        onDiffLoaded: { diff in
                            cachedDiff = diff
                        }
                    )
                    .navigationTitle("Diff")
                    .navigationBarTitleDisplayMode(.inline)
                    .toolbar {
                        ToolbarItem(placement: .topBarTrailing) {
                            Button {
                                showFullDiff = false
                            } label: {
                                Text("Done")
                            }
                        }
                    }
                }
            }
            .sheet(isPresented: $showingPRCreationSheet) {
                if let workspace = workspace {
                    PRCreationSheet(isPresented: $showingPRCreationSheet, workspace: workspace, isCreating: $isCreatingPR)
                }
            }
            .onAppear {
                // Auto-show sheet if no history
                if phase == .input {
                    showPromptSheet = true
                }
            }
            .onReceive(NotificationCenter.default.publisher(for: .codespaceShutdownDetected)) { notification in
                // Handle codespace shutdown notification
                if let message = notification.userInfo?["message"] as? String {
                    shutdownMessage = message
                    showShutdownAlert = true
                }
            }
            .alert("Codespace Unavailable", isPresented: $showShutdownAlert) {
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

                        // Reset health check state
                        await MainActor.run {
                            HealthCheckService.shared.resetShutdownState()

                            // Post notification to trigger reconnection flow
                            // This will dismiss all views and auto-reconnect in CodespaceView
                            NotificationCenter.default.post(name: .shouldReconnectToCodespace, object: nil)

                            // Also dismiss this view
                            dismiss()
                        }
                    }
                }
            } message: {
                Text(shutdownMessage ?? "Your codespace has shut down. Tap 'Reconnect' to restart it.")
            }
    }

    private var mainView: some View {
        ZStack {
            Color(uiColor: .systemBackground)
                .ignoresSafeArea()

            mainContentView
        }
        .navigationTitle(navigationTitle)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            toolbarContent
        }
    }

    private var loadingView: some View {
        VStack(spacing: 16) {
            ProgressView()
                .scaleEffect(1.5)
            Text("Loading workspace...")
                .font(.body)
                .foregroundStyle(.secondary)
        }
    }

    private var errorView: some View {
        VStack(spacing: 20) {
            Text("Error")
                .font(.title2)
                .foregroundStyle(.primary)

            Text(error.isEmpty ? "Workspace not found" : error)
                .font(.body)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)

            Button {
                Task { await loadWorkspace() }
            } label: {
                Text("Retry")
            }
            .buttonStyle(ProminentButtonStyle())
            .padding(.horizontal, 20)
        }
        .padding()
    }

    private var contentView: some View {
        ScrollView {
            VStack(spacing: 20) {
                if phase == .input || (phase == .completed && !hasSessionContent) {
                    // Show empty state for input phase OR completed phase with no content
                    // (e.g., new workspace with commits but no Claude session)
                    emptyStateView
                        .padding(.horizontal, 16)
                } else if phase == .working {
                    workingSection
                } else if phase == .completed {
                    completedSection
                }

                if !error.isEmpty {
                    errorBox
                        .padding(.horizontal, 16)
                }
            }
            .padding(.top, 16)
        }
        .safeAreaInset(edge: .bottom) {
            footerView
        }
    }

    private var emptyStateView: some View {
        VStack(spacing: 16) {
            Image(systemName: "sparkles")
                .font(.system(size: 48))
                .foregroundStyle(.secondary)
                .padding(.top, 40)

            Text("Start Working")
                .font(.title2.weight(.semibold))

            Text("Describe what you'd like to work on")
                .font(.body)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)

            Button {
                showPromptSheet = true
            } label: {
                Text("Start Working")
            }
            .buttonStyle(ProminentButtonStyle())
            .padding(.horizontal, 20)
        }
        .frame(maxWidth: .infinity)
        .padding(20)
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }

    private var workingSection: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Session and todos section with padding
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 12) {
                    ProgressView()
                    Text("Claude is working...")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }

                // Show the user's prompt (either pending or from session/workspace)
                if let userPrompt = pendingUserPrompt ?? effectiveLatestUserPrompt, !userPrompt.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("You asked:")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(.secondary)

                        Text(userPrompt)
                            .font(.body)
                            .foregroundStyle(.primary)
                    }
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 10))
                }

                // Show Claude's latest message while working
                if let claudeMessage = effectiveLatestMessage, !claudeMessage.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Claude is saying:")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(Color.accentColor)

                        MarkdownText(claudeMessage)
                    }
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 10))
                } else if effectiveSessionTitle != nil {
                    // Show loading state while fetching message
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Claude is saying:")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(Color.accentColor)

                        HStack(spacing: 8) {
                            ProgressView()
                                .scaleEffect(0.8)
                            Text("Loading response...")
                                .font(.body)
                                .foregroundStyle(.secondary)
                        }
                    }
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 10))
                }

                if let todos = effectiveTodos, !todos.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Progress:")
                            .font(.callout.weight(.semibold))
                            .foregroundStyle(.secondary)

                        TodoListView(todos: todos)
                    }
                }
            }
            .padding(8)
            .padding(.horizontal, 8)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 12))

            // Diff viewer edge-to-edge - only show if we have actual diff data with files
            if let diff = cachedDiff, !diff.fileDiffs.isEmpty {
                WorkspaceDiffViewer(
                    workspaceId: workspaceId,
                    selectedFile: nil,
                    onClose: nil,
                    onExpand: {
                        showFullDiff = true
                    },
                    preloadedDiff: cachedDiff,
                    onDiffLoaded: { diff in
                        cachedDiff = diff
                    }
                )
                .frame(height: 400)
                .padding(.top, 16)
            }
        }
    }

    private var completedSection: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Session content with padding
            VStack(alignment: .leading, spacing: 8) {
                // User prompt
                if let userPrompt = effectiveLatestUserPrompt, !userPrompt.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("You asked:")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(.secondary)

                        Text(userPrompt)
                            .font(.body)
                            .foregroundStyle(.primary)
                    }
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 10))
                }

                // Claude's response
                if let claudeMessage = effectiveLatestMessage, !claudeMessage.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Claude responded:")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(Color.accentColor)

                        MarkdownText(claudeMessage)
                    }
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 10))
                } else if effectiveSessionTitle != nil {
                    // Show loading state while fetching message
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Claude responded:")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(Color.accentColor)

                        HStack(spacing: 8) {
                            ProgressView()
                                .scaleEffect(0.8)
                            Text("Loading response...")
                                .font(.body)
                                .foregroundStyle(.secondary)
                        }
                    }
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 10))
                }

                if let todos = effectiveTodos, !todos.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Tasks:")
                            .font(.callout.weight(.semibold))
                            .foregroundStyle(.secondary)

                        TodoListView(todos: todos)
                    }
                }
            }
            .padding(8)
            .padding(.horizontal, 8)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 12))

            // Diff viewer edge-to-edge - only show if we have actual diff data with files
            if let diff = cachedDiff, !diff.fileDiffs.isEmpty {
                WorkspaceDiffViewer(
                    workspaceId: workspaceId,
                    selectedFile: nil,
                    onClose: nil,
                    onExpand: {
                        showFullDiff = true
                    },
                    preloadedDiff: cachedDiff,
                    onDiffLoaded: { diff in
                        cachedDiff = diff
                    }
                )
                .frame(height: 400)
                .padding(.top, 16)
            }
        }
    }

    private var errorBox: some View {
        HStack(spacing: 10) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(Color.red)
            Text(error)
                .font(.subheadline)
            Spacer()
        }
        .foregroundStyle(Color.red)
        .padding(12)
        .background(Color.red.opacity(0.08))
        .clipShape(RoundedRectangle(cornerRadius: 10))
    }

    private var footerView: some View {
        Group {
            if phase == .completed && hasSessionContent {
                // Only show footer buttons if we have actual session content to show
                HStack(spacing: 12) {
                    Button {
                        showPromptSheet = true
                    } label: {
                        Text("Ask for changes")
                    }
                    .buttonStyle(ProminentButtonStyle())

                    Button {
                        handlePRAction()
                    } label: {
                // Show "Update PR" if we have a PR URL and commits ahead
            // AND the PR is not closed or merged
            // For backward compatibility: if hasCommitsAheadOfRemote is nil (older backend),
            // assume true if we have commits (existing behavior)
            if let _ = workspace?.pullRequestUrl,
               workspace?.pullRequestState != "CLOSED",
               workspace?.pullRequestState != "MERGED" {
                
                let hasCommitsAhead = workspace?.hasCommitsAheadOfRemote ?? ((workspace?.commitCount ?? 0) > 0)
                
                if hasCommitsAhead {
                    Button(action: {
                        Task {
                            await updatePR()
                        }
                    }) {
                        HStack(spacing: 6) {
                            if isUpdatingPR {
                                ProgressView()
                                    .scaleEffect(0.8)
                                Text("Updating...")
                            } else {
                                Image(systemName: "arrow.triangle.2.circlepath")
                                Text("Update PR")
                            }
                        }
                    }
                } else {
                    Button(action: {
                        handlePRAction()
                    }) {
                        HStack(spacing: 6) {
                            Image(systemName: "arrow.up.right.square")
                            Text("View PR")
                        }
                    }
                }
            } else if let _ = workspace?.pullRequestUrl {
                Button(action: {
                    handlePRAction()
                }) {
                    HStack(spacing: 6) {
                        Image(systemName: "arrow.up.right.square")
                        Text("View PR")
                    }
                }
            } else {
                Button(action: {
                    handlePRAction()
                }) {
                    HStack(spacing: 6) {
                        Image(systemName: "arrow.triangle.merge")
                        Text("Create PR")
                    }
                }
            }
                    }
                    .buttonStyle(ProminentButtonStyle())
                    .disabled((workspace?.commitCount ?? 0) == 0 || isUpdatingPR)
                    .opacity(((workspace?.commitCount ?? 0) == 0 || isUpdatingPR) ? 0.5 : 1.0)
                }
                .padding(16)
                .background(.ultraThinMaterial)
            }
        }
    }

    private func loadWorkspace() async {
        // If poller already has workspace data (from initialWorkspace), skip fetch
        if let workspace = poller.workspace {
            await MainActor.run {
                NSLog("✅ Using pre-loaded workspace data, skipping initial fetch for: \(workspaceId)")
                determinePhase(for: workspace)
            }

            // Hydrate session data if workspace is in .running or .active state
            // This ensures context stats and other session info are available immediately
            if workspace.claudeActivityState == .running || workspace.claudeActivityState == .active {
                NSLog("📊 Initial hydration: fetching session data for workspace in \(workspace.claudeActivityState?.rawValue ?? "unknown") state")
                await fetchSessionData()
            }
            return
        }

        NSLog("🔍 No pre-loaded data, fetching workspace: \(workspaceId)")
        phase = .loading
        error = ""

        do {
            // On initial load, don't pass etag - we need the workspace data
            guard let result = try await CatnipAPI.shared.getWorkspace(id: workspaceId, ifNoneMatch: nil) else {
                // This shouldn't happen on initial load without etag
                await MainActor.run {
                    NSLog("❌ getWorkspace returned nil (304 Not Modified?) for: \(workspaceId)")
                    self.error = "Workspace not found"
                    phase = .error
                }
                return
            }

            let workspace = result.workspace
            NSLog("✅ Successfully fetched workspace: \(workspaceId)")

            await MainActor.run {
                // Poller will manage workspace state
                determinePhase(for: workspace)
            }

            // Hydrate session data if workspace is in .running or .active state
            if workspace.claudeActivityState == .running || workspace.claudeActivityState == .active {
                NSLog("📊 Initial hydration: fetching session data for workspace in \(workspace.claudeActivityState?.rawValue ?? "unknown") state")
                await fetchSessionData()
            }
        } catch let apiError as APIError {
            await MainActor.run {
                NSLog("❌ API error fetching workspace \(workspaceId): \(apiError.errorDescription ?? "unknown")")
                self.error = apiError.errorDescription ?? "Unknown error"
                phase = .error
            }
        } catch {
            await MainActor.run {
                NSLog("❌ Error fetching workspace \(workspaceId): \(error.localizedDescription)")
                self.error = error.localizedDescription
                phase = .error
            }
        }
    }

    private func determinePhase(for workspace: WorkspaceInfo) {
        // Use effective values that prefer session data over workspace data
        let currentTitle = effectiveSessionTitle
        let currentTodos = effectiveTodos

        NSLog("📊 determinePhase - claudeActivityState: %@, latestSessionTitle: %@, todos: %d, isDirty: %@, commits: %d, pendingPrompt: %@",
              workspace.claudeActivityState.map { "\($0)" } ?? "nil",
              currentTitle ?? "nil",
              currentTodos?.count ?? 0,
              workspace.isDirty.map { "\($0)" } ?? "nil",
              workspace.commitCount ?? 0,
              pendingUserPrompt != nil ? "yes" : "no")

        let previousPhase = phase

        // Clear pendingUserPrompt if backend has started processing, completed, or timed out
        // This prevents getting stuck in "working" phase
        if pendingUserPrompt != nil {
            // Backend received and started processing our prompt
            if workspace.claudeActivityState == .running {
                NSLog("📊 Backend started processing - clearing pending prompt")
                pendingUserPrompt = nil
                pendingUserPromptTimestamp = nil
            }
            // Backend completed the session
            else if currentTitle != nil {
                NSLog("📊 Session created - clearing pending prompt")
                pendingUserPrompt = nil
                pendingUserPromptTimestamp = nil
            }
            // Timeout: clear stale pending prompt after 30 seconds
            else if let timestamp = pendingUserPromptTimestamp,
                    Date().timeIntervalSince(timestamp) > 30 {
                NSLog("⚠️ Pending prompt timed out after 30s - clearing")
                pendingUserPrompt = nil
                pendingUserPromptTimestamp = nil
            }
        }

        // Show "working" phase when:
        // 1. Claude is .active (actively processing), OR
        // 2. We have a pending prompt (just sent a prompt but backend hasn't updated yet)
        // State meanings:
        //   - .inactive: no PTY running
        //   - .running: PTY up and running, waiting for user action (Claude NOT working)
        //   - .active: PTY up and Claude is actively working
        if workspace.claudeActivityState == .active || pendingUserPrompt != nil {
            phase = .working

            // Fetch latest message and diff while working
            Task {
                await fetchLatestMessage(for: workspace)
                await fetchDiffIfNeeded(for: workspace)
            }
        } else if currentTitle != nil || currentTodos?.isEmpty == false {
            // Has a session title or todos - definitely completed
            phase = .completed

            // Fetch the latest message for completed sessions
            Task {
                await fetchLatestMessage(for: workspace)
                await fetchDiffIfNeeded(for: workspace)
            }
        } else if workspace.isDirty == true || (workspace.commitCount ?? 0) > 0 {
            // Workspace has modifications or commits but no session title
            // This can happen with old /messages endpoint usage
            // Treat as completed to show the changes
            phase = .completed
            NSLog("📊 Workspace has changes but no session - treating as completed")

            // Try to fetch latest message in case there is one
            Task {
                await fetchLatestMessage(for: workspace)
                await fetchDiffIfNeeded(for: workspace)
            }
        } else {
            phase = .input
        }

        NSLog("📊 determinePhase - final phase: %@ (was: %@)", "\(phase)", "\(previousPhase)")
    }

    private func sendPrompt() async {
        guard let workspace = workspace, !prompt.trimmingCharacters(in: .whitespaces).isEmpty else {
            NSLog("🐱 [WorkspaceDetailView] Cannot send prompt - workspace or prompt is empty")
            return
        }

        let promptToSend = prompt.trimmingCharacters(in: .whitespaces)
        NSLog("🐱 [WorkspaceDetailView] Sending prompt to workspace: \(workspace.id)")
        NSLog("🐱 [WorkspaceDetailView] Prompt length: \(promptToSend.count) chars")
        NSLog("🐱 [WorkspaceDetailView] Workspace name (session ID): \(workspace.name)")

        isSubmitting = true
        error = ""

        do {
            NSLog("🐱 [WorkspaceDetailView] About to call sendPromptToPTY API...")
            try await CatnipAPI.shared.sendPromptToPTY(
                workspacePath: workspace.name,
                prompt: promptToSend,
                agent: "claude"
            )
            NSLog("🐱 [WorkspaceDetailView] ✅ Successfully sent prompt")

            await MainActor.run {
                // Store the prompt we just sent for immediate display
                pendingUserPrompt = promptToSend
                pendingUserPromptTimestamp = Date()
                NSLog("🐱 [WorkspaceDetailView] Stored pending prompt: \(promptToSend.prefix(50))...")

                prompt = ""
                showPromptSheet = false
                phase = .working
                isSubmitting = false

                // Trigger immediate refresh after sending prompt
                NSLog("🐱 [WorkspaceDetailView] Triggering poller refresh")
                poller.refresh()
            }
        } catch APIError.timeout {
            NSLog("🐱 [WorkspaceDetailView] ⏰ PTY not ready (timeout)")
            await MainActor.run {
                self.error = "Claude is still starting up. Please try again in a moment."
                isSubmitting = false
            }
        } catch {
            NSLog("🐱 [WorkspaceDetailView] ❌ Failed to send prompt: \(error)")
            await MainActor.run {
                self.error = error.localizedDescription
                isSubmitting = false
                showPromptSheet = false
            }
        }
    }

    private func fetchLatestMessage(for workspace: WorkspaceInfo) async {
        do {
            let result = try await CatnipAPI.shared.getLatestMessage(worktreePath: workspace.path)
            await MainActor.run {
                if !result.isError {
                    self.latestMessage = result.content
                }
            }
        } catch {
            NSLog("❌ Failed to fetch latest message: %@", error.localizedDescription)
        }
    }

    /// Fetch session data to hydrate context stats and other session info
    private func fetchSessionData() async {
        do {
            NSLog("📊 Fetching session data for workspace: \(workspaceId)")
            let sessionData = try await CatnipAPI.shared.getSessionData(workspaceId: workspaceId)

            await MainActor.run {
                if let sessionData = sessionData {
                    poller.updateSessionData(sessionData)
                    NSLog("✅ Hydrated session data - context tokens: \(sessionData.stats?.lastContextSizeTokens ?? 0)")
                } else {
                    NSLog("⚠️ Session data fetch returned nil")
                }
            }
        } catch {
            NSLog("❌ Failed to fetch session data: \(error.localizedDescription)")
        }
    }

    private func fetchDiffIfNeeded(for workspace: WorkspaceInfo) async {
        // Only fetch if workspace has changes
        guard (workspace.isDirty == true || (workspace.commitCount ?? 0) > 0) else {
            NSLog("📊 No changes to fetch diff for")
            return
        }

        // Skip if we already have a cached diff and Claude is still actively working
        // We want to refetch periodically during active work, but avoid spamming requests
        // When work completes, we'll refetch one final time from the completed phase
        let isActivelyWorking = workspace.claudeActivityState == .active
        if cachedDiff != nil && isActivelyWorking {
            NSLog("📊 Diff already cached and Claude still actively working, skipping fetch to avoid spam")
            return
        }

        NSLog("📊 Fetching diff for workspace with changes (dirty: %@, commits: %d, activelyWorking: %@)",
              workspace.isDirty.map { "\($0)" } ?? "nil",
              workspace.commitCount ?? 0,
              isActivelyWorking ? "yes" : "no")

        do {
            let diff = try await CatnipAPI.shared.getWorkspaceDiff(id: workspace.id)
            await MainActor.run {
                NSLog("📊 Successfully fetched diff: %d files changed", diff.fileDiffs.count)
                self.cachedDiff = diff
            }
        } catch {
            NSLog("❌ Failed to fetch diff: %@", error.localizedDescription)
        }
    }

    private func handlePRAction() {
        guard let workspace = workspace else { return }
        
        if let urlString = workspace.pullRequestUrl, let url = URL(string: urlString) {
            // If we have commits ahead and PR is open, show update confirmation
            // Otherwise just open the URL
            if let hasCommitsAhead = workspace.hasCommitsAheadOfRemote, 
               hasCommitsAhead,
               workspace.pullRequestState != "CLOSED",
               workspace.pullRequestState != "MERGED" {
                Task {
                    await updatePR()
                }
            } else {
                #if os(macOS)
                NSWorkspace.shared.open(url)
                #else
                UIApplication.shared.open(url)
                #endif
            }
        } else if (workspace.commitCount ?? 0) > 0 {
            showingPRCreationSheet = true
        }
    }

    private func updatePR() async {
        guard let workspace = workspace else { return }
        
        NSLog("🔄 Updating PR for workspace: \(workspace.id)")
        isUpdatingPR = true
        error = ""
        
        do {
            let prUrl = try await CatnipAPI.shared.updatePullRequest(workspaceId: workspace.id)
            
            await MainActor.run {
                NSLog("✅ Successfully updated PR: \(prUrl)")
                isUpdatingPR = false
                
                // Open the updated PR
                if let url = URL(string: prUrl) {
                    UIApplication.shared.open(url)
                }
                
                // Trigger refresh to update state (clear dirty flag etc if backend handles it)
                poller.refresh()
            }
        } catch {
            NSLog("❌ Failed to update PR: \(error)")
            await MainActor.run {
                self.error = "Failed to update PR: \(error.localizedDescription)"
                isUpdatingPR = false
            }
        }
    }

    // MARK: - Terminal View

    private var terminalView: some View {
        let codespaceName = UserDefaults.standard.string(forKey: "codespace_name") ?? "nil"
        let token = authManager.sessionToken ?? "nil"
        let worktreeName = workspace?.name ?? "unknown"

        // 🔐 DEBUG: WebSocket connection info for testing
        NSLog("🔐🔐🔐 WEBSOCKET_DEBUG 🔐🔐🔐")
        NSLog("🔐 Codespace: \(codespaceName)")
        NSLog("🔐 Session Token: \(token)")
        NSLog("🔐 WebSocket Base URL: \(websocketBaseURL)")
        NSLog("🔐 Workspace ID (UUID): \(workspaceId)")
        NSLog("🔐 Worktree Name (session): \(worktreeName)")
        NSLog("🔐🔐🔐 END WEBSOCKET_DEBUG 🔐🔐🔐")

        // Terminal view with navigation bar
        // Use worktree name (not UUID) as the session parameter
        // Only connect when in landscape mode to prevent premature connections
        // Let keyboard naturally push content up by not ignoring safe area
        return TerminalView(
            workspaceId: worktreeName,
            baseURL: websocketBaseURL,
            codespaceName: UserDefaults.standard.string(forKey: "codespace_name"),
            authToken: authManager.sessionToken,
            shouldConnect: isLandscape
        )
    }

    private var websocketBaseURL: String {
        // Convert https://catnip.run to wss://catnip.run
        return "wss://catnip.run"
    }

    // MARK: - Portrait Terminal View

    private var portraitTerminalView: some View {
        let codespaceName = UserDefaults.standard.string(forKey: "codespace_name") ?? "nil"
        let worktreeName = workspace?.name ?? "unknown"

        NSLog("🐱 Portrait terminal - Codespace: \(codespaceName), Worktree: \(worktreeName)")

        // Terminal fills available space - glass accessory overlays it
        // Add ~60 points width for approximately 7-8 extra columns at 12pt mono font
        return GeometryReader { geometry in
            ScrollView(.horizontal, showsIndicators: false) {
                TerminalView(
                    workspaceId: worktreeName,
                    baseURL: websocketBaseURL,
                    codespaceName: UserDefaults.standard.string(forKey: "codespace_name"),
                    authToken: authManager.sessionToken,
                    shouldConnect: showPortraitTerminal,
                    showExitButton: false,
                    showDismissButton: false
                )
                .frame(width: geometry.size.width + 60)
            }
        }
        .background(Color.black)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    showPortraitTerminal = false
                } label: {
                    ContextProgressRing(contextTokens: sessionStats?.lastContextSizeTokens) {
                        Image(systemName: "square.grid.2x2")
                            .font(.system(size: 11, weight: .medium))
                            .foregroundStyle(.primary)
                    }
                }
                .buttonStyle(.plain)
            }
        }
    }

    private func updateOrientation() {
        // Detect landscape: compact height OR regular width + compact height
        // This works for both iPhone landscape and iPad landscape
        let newIsLandscape = verticalSizeClass == .compact ||
            (horizontalSizeClass == .regular && verticalSizeClass == .compact)

        if newIsLandscape != isLandscape {
            isLandscape = newIsLandscape
            NSLog("📱 Orientation changed - isLandscape: \(isLandscape)")

            // Close portrait terminal when rotating to landscape
            if newIsLandscape && showPortraitTerminal {
                showPortraitTerminal = false
            }
        }
    }

    private func rotateToLandscape() {
        // Request landscape orientation
        if let windowScene = UIApplication.shared.connectedScenes.first as? UIWindowScene {
            windowScene.requestGeometryUpdate(.iOS(interfaceOrientations: .landscape)) { error in
                NSLog("⚠️ Failed to rotate to landscape: \(error.localizedDescription)")
            }
        }
    }
}

struct TodoListView: View {
    let todos: [Todo]

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(todos) { todo in
                HStack(alignment: .top, spacing: 12) {
                    Circle()
                        .fill(todoColor(for: todo.status))
                        .frame(width: 8, height: 8)
                        .padding(.top, 6)

                    Text(todo.content)
                        .font(.body)
                        .foregroundStyle(.primary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        }
    }

    private func todoColor(for status: TodoStatus) -> Color {
        switch status {
        case .completed:
            return Color.green
        case .inProgress:
            return Color.orange
        case .pending:
            return Color.gray.opacity(0.5)
        }
    }
}

#if DEBUG
#Preview("Input Phase") {
    NavigationStack {
        WorkspaceDetailPreview(phase: .input)
    }
}

#Preview("Working Phase") {
    NavigationStack {
        WorkspaceDetailPreview(phase: .working)
    }
}

#Preview("Completed Phase") {
    NavigationStack {
        WorkspaceDetailPreview(phase: .completed)
    }
}

// Preview wrapper for testing different phases
private struct WorkspaceDetailPreview: View {
    let phase: WorkspacePhase
    @State private var mockWorkspace: WorkspaceInfo
    @State private var currentPhase: WorkspacePhase
    @State private var showSheet = false
    @State private var previewPrompt = ""

    init(phase: WorkspacePhase) {
        self.phase = phase

        // Configure workspace based on phase
        var workspace = WorkspaceInfo.preview1
        switch phase {
        case .input:
            workspace = WorkspaceInfo(
                id: workspace.id,
                name: workspace.name,
                branch: workspace.branch,
                repoId: workspace.repoId,
                claudeActivityState: .inactive,
                commitCount: 0,
                isDirty: false,
                lastAccessed: workspace.lastAccessed,
                createdAt: workspace.createdAt,
                todos: nil,
                latestSessionTitle: nil,
                latestUserPrompt: nil,
                latestClaudeMessage: nil,
                pullRequestUrl: nil,
                pullRequestState: nil,
                hasCommitsAheadOfRemote: nil,
                path: workspace.path,
                cacheStatus: workspace.cacheStatus
            )
        case .working:
            // .active means Claude is actively working
            workspace = WorkspaceInfo(
                id: workspace.id,
                name: workspace.name,
                branch: workspace.branch,
                repoId: workspace.repoId,
                claudeActivityState: .active,  // Claude is actively working
                commitCount: workspace.commitCount,
                isDirty: workspace.isDirty,
                lastAccessed: workspace.lastAccessed,
                createdAt: workspace.createdAt,
                todos: Todo.previewList,
                latestSessionTitle: "Implementing new feature",
                latestUserPrompt: nil,
                latestClaudeMessage: "Working on the new feature...",
                pullRequestUrl: nil,
                pullRequestState: nil,
                hasCommitsAheadOfRemote: workspace.hasCommitsAheadOfRemote,
                path: workspace.path,
                cacheStatus: workspace.cacheStatus
            )
        case .completed:
            workspace = WorkspaceInfo.preview1
        default:
            workspace = WorkspaceInfo.preview1
        }

        _mockWorkspace = State(initialValue: workspace)
        _currentPhase = State(initialValue: phase)
    }

    var body: some View {
        ZStack {
            Color(uiColor: .systemGroupedBackground)
                .ignoresSafeArea()

            ScrollView {
                VStack(spacing: 20) {
                    if currentPhase == .input {
                        inputSectionPreview
                    } else if currentPhase == .working {
                        workingSectionPreview
                    } else if currentPhase == .completed {
                        completedSectionPreview
                    }
                }
                .padding(16)
            }
        }
        .navigationTitle(mockWorkspace.displayName)
        .navigationBarTitleDisplayMode(.inline)
    }

    private var inputSectionPreview: some View {
        VStack(spacing: 16) {
            Image(systemName: "sparkles")
                .font(.system(size: 48))
                .foregroundStyle(.secondary)
                .padding(.top, 40)

            Text("Start Working")
                .font(.title2.weight(.semibold))

            Text("Describe what you'd like to work on")
                .font(.body)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)

            Button {
                showSheet = true
            } label: {
                Text("Start Working")
            }
            .buttonStyle(ProminentButtonStyle())
            .padding(.horizontal, 20)
        }
        .frame(maxWidth: .infinity)
        .padding(20)
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12))
        .sheet(isPresented: $showSheet) {
            PromptSheet(
                isPresented: $showSheet,
                prompt: $previewPrompt,
                mode: .askForChanges,
                onSubmit: {
                    showSheet = false
                }
            )
        }
    }

    private var workingSectionPreview: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Session and todos section with padding
            VStack(alignment: .leading, spacing: 16) {
                HStack(spacing: 12) {
                    ProgressView()
                    Text("Claude is working...")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }

                if let latestSession = mockWorkspace.latestSessionTitle {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Session:")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(Color.accentColor)

                        Text(latestSession)
                            .font(.body)
                            .foregroundStyle(.primary)
                    }
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 10))
                }

                if let todos = mockWorkspace.todos {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Progress:")
                            .font(.callout.weight(.semibold))
                            .foregroundStyle(.secondary)

                        TodoListView(todos: todos)
                    }
                }
            }
            .padding(16)

            // Diff viewer edge-to-edge
            Divider()

            // Use mock preview data
            WorkspaceDiffViewerPreviewContent()
                .frame(height: 400)
        }
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }

    private var completedSectionPreview: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Session content with padding
            VStack(alignment: .leading, spacing: 16) {
                // User prompt
                if let userPrompt = mockWorkspace.latestUserPrompt, !userPrompt.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("You asked:")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(.secondary)

                        Text(userPrompt)
                            .font(.body)
                            .foregroundStyle(.primary)
                    }
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 10))
                }

                // Claude's response
                if let claudeMessage = mockWorkspace.latestSessionTitle {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Claude responded:")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(Color.accentColor)

                        Text(claudeMessage)
                            .font(.body)
                            .foregroundStyle(.primary)
                    }
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 10))
                }

                if let todos = mockWorkspace.todos {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Tasks:")
                            .font(.callout.weight(.semibold))
                            .foregroundStyle(.secondary)

                        TodoListView(todos: todos)
                    }
                }
            }
            .padding(16)

            // Diff viewer edge-to-edge
            Divider()

            // Use mock preview data
            WorkspaceDiffViewerPreviewContent()
                .frame(height: 400)
        }
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }
}
#endif

// MARK: - Markdown Text Component

struct MarkdownText: View {
    let markdown: String

    init(_ markdown: String) {
        self.markdown = markdown
    }

    var body: some View {
        Markdown(markdown)
            .markdownTextStyle(\.text) {
                FontSize(15)
            }
            .textSelection(.enabled)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}

// MARK: - Context Progress Ring

/// A circular progress indicator showing Claude's context usage
/// Changes color based on token count thresholds:
/// - Gray: < 40K tokens
/// - Green: 40K - 80K tokens
/// - Orange: 80K - 120K tokens
/// - Red: > 120K tokens (approaching 155K limit)
struct ContextProgressRing<Content: View>: View {
    let contextTokens: Int64?
    let content: Content

    private let maxTokens: Int64 = 155_000
    private let lineWidth: CGFloat = 2.5
    private let buttonSize: CGFloat = 36
    // Inset for the ring - positions it just inside the button edge
    private let ringInset: CGFloat = 1.0

    init(contextTokens: Int64?, @ViewBuilder content: () -> Content) {
        self.contextTokens = contextTokens
        self.content = content()
    }

    private var progress: Double {
        guard let tokens = contextTokens, tokens > 0 else { return 0 }
        return min(Double(tokens) / Double(maxTokens), 1.0)
    }

    private var ringColor: Color {
        guard let tokens = contextTokens else { return .gray.opacity(0.3) }

        switch tokens {
        case ..<40_000:
            return .gray.opacity(0.5)
        case 40_000..<80_000:
            return .green
        case 80_000..<120_000:
            return .orange
        default:
            return .red
        }
    }

    var body: some View {
        Circle()
            .fill(.ultraThinMaterial)
            .overlay {
                // Background ring (always visible, subtle)
                Circle()
                    .strokeBorder(Color.gray.opacity(0.3), lineWidth: lineWidth)
                    .padding(ringInset)
            }
            .overlay {
                // Progress ring - uses trim for animation
                Circle()
                    .trim(from: 0, to: progress)
                    .stroke(ringColor, style: StrokeStyle(lineWidth: lineWidth, lineCap: .round))
                    .rotationEffect(.degrees(-90))
                    .padding(ringInset + lineWidth / 2)
                    .animation(.easeInOut(duration: 0.3), value: progress)
            }
            .overlay {
                // Icon content centered
                content
            }
            .frame(width: buttonSize, height: buttonSize)
    }
}

// MARK: - Preview Helper for Diff Viewer

#if DEBUG
private struct WorkspaceDiffViewerPreviewContent: View {
    var body: some View {
        VStack(spacing: 0) {
            // Header
            HStack(spacing: 12) {
                Image(systemName: "doc.text")
                    .font(.caption)
                    .foregroundStyle(.secondary)

                VStack(alignment: .leading, spacing: 2) {
                    Text("Diff")
                        .font(.caption.weight(.medium))
                    Text("3 files changed, 25 insertions(+), 8 deletions(-)")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }

                Spacer()

                Button {} label: {
                    Image(systemName: "xmark")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .frame(width: 24, height: 24)
                }
                .buttonStyle(.plain)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
            .background(Color(uiColor: .systemBackground).opacity(0.95))
            .overlay(
                Rectangle()
                    .fill(Color(uiColor: .separator))
                    .frame(height: 0.5),
                alignment: .bottom
            )

            // Content
            ScrollView {
                LazyVStack(spacing: 8) {
                    DiffFileView(fileDiff: .preview1, initiallyExpanded: true)
                    DiffFileView(fileDiff: .preview2, initiallyExpanded: false)
                }
                .padding(.vertical, 8)
            }
        }
        .background(Color(uiColor: .systemGroupedBackground))
    }
}
#endif
