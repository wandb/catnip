//
//  WorkspaceDetailView.swift
//  catnip
//
//  Workspace detail screen with polling for updates
//

import SwiftUI
import MarkdownUI

enum WorkspacePhase: Equatable {
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
    @State private var showTerminalOnly = false  // Show terminal only (iPhone toggle)

    // Codespace shutdown detection
    @State private var showShutdownAlert = false
    @State private var shutdownMessage: String?
    @Environment(\.dismiss) private var dismiss
    @Environment(\.adaptiveTheme) private var adaptiveTheme

    // CatnipInstaller for status refresh
    @StateObject private var installer = CatnipInstaller.shared

    @EnvironmentObject var authManager: AuthManager

    init(workspaceId: String, initialWorkspace: WorkspaceInfo? = nil, pendingPrompt: String? = nil) {
        self.workspaceId = workspaceId
        _poller = StateObject(wrappedValue: WorkspacePoller(workspaceId: workspaceId, initialWorkspace: initialWorkspace))

        // Set initial pending prompt if provided (e.g., from workspace creation flow)
        if let pendingPrompt = pendingPrompt, !pendingPrompt.isEmpty {
            _pendingUserPrompt = State(initialValue: pendingPrompt)
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

    var body: some View {
        // Determine phase during body evaluation if we have workspace data but haven't updated phase yet
        if let workspace = poller.workspace, phase == .loading {
            DispatchQueue.main.async {
                self.determinePhase(for: workspace)
            }
        }

        return ZStack {
            Color(uiColor: .systemBackground)
                .ignoresSafeArea()

            Group {
                if phase == .loading {
                    loadingView
                } else if phase == .error || workspace == nil {
                    errorView
                } else {
                    contentView
                }
            }
        }
        .task {
                await loadWorkspace()
                poller.start()

                // Start PTY after workspace is loaded
                if let workspace = workspace {
                    Task {
                        do {
                            try await CatnipAPI.shared.startPTY(workspacePath: workspace.name, agent: "claude")
                        } catch {
                            // Non-fatal - PTY will be created on-demand if needed
                        }
                    }
                }
            }
            .onDisappear {
                poller.stop()
            }
            .onChange(of: poller.workspace) {
                if let newWorkspace = poller.workspace {
                    determinePhase(for: newWorkspace)
                }
            }
            .onChange(of: poller.error) {
                if let newError = poller.error {
                    // Filter out "cancelled" errors - normal when requests are cancelled
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
        Group {
            if adaptiveTheme.prefersSideBySideTerminal {
                // iPad/Mac: Vertical split - terminal on top, chat on bottom
                AdaptiveSplitView(
                    defaultMode: .split,
                    allowModeToggle: true,
                    contextTokens: sessionStats?.lastContextSizeTokens,
                    leading: { terminalView },
                    trailing: { chatInterfaceView }
                )
            } else {
                // iPhone: Single view with toggle
                ZStack {
                    Color(uiColor: .systemBackground).ignoresSafeArea()

                    if showTerminalOnly {
                        terminalView
                    } else {
                        chatInterfaceView
                    }
                }
                .toolbar {
                    ToolbarItem(placement: .topBarTrailing) {
                        Button { showTerminalOnly.toggle() } label: {
                            ContextProgressRing(contextTokens: sessionStats?.lastContextSizeTokens) {
                                Image(systemName: showTerminalOnly ? "text.bubble" : "terminal")
                                    .font(.system(size: 11, weight: .medium))
                            }
                        }
                    }
                }
            }
        }
        .navigationTitle(navigationTitle)
        .navigationBarTitleDisplayMode(.inline)
    }

    private var chatInterfaceView: some View {
        ScrollView {
            VStack(spacing: adaptiveTheme.cardPadding) {
                if phase == .input {
                    emptyStateView
                        .padding(.horizontal, adaptiveTheme.containerPadding)
                } else if phase == .working {
                    workingSection
                } else if phase == .completed {
                    completedSection
                }

                if !error.isEmpty {
                    errorBox
                        .padding(.horizontal, adaptiveTheme.containerPadding)
                }
            }
            .padding(.top, adaptiveTheme.containerPadding)
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
        VStack(spacing: 0) {
            if phase == .completed && hasSessionContent {
                // Footer buttons should fill the same horizontal space as scrollable content
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
                .padding(.vertical, 16)
                .padding(.horizontal, adaptiveTheme.containerPadding)  // Match scrollable content padding
                .background(.ultraThinMaterial)
            }
        }
    }

    @MainActor
    private func loadWorkspace() async {
        // If poller already has workspace data (from initialWorkspace), skip fetch
        if let workspace = poller.workspace {
            determinePhase(for: workspace)
            await fetchSessionData()
            return
        }

        phase = .loading
        error = ""

        do {
            // On initial load, don't pass etag - we need the workspace data
            guard let result = try await CatnipAPI.shared.getWorkspace(id: workspaceId, ifNoneMatch: nil) else {
                await MainActor.run {
                    self.error = "Workspace not found"
                    phase = .error
                }
                return
            }

            let workspace = result.workspace
            determinePhase(for: workspace)
            await fetchSessionData()
        } catch let apiError as APIError {
            self.error = apiError.errorDescription ?? "Unknown error"
            phase = .error
        } catch {
            self.error = error.localizedDescription
            phase = .error
        }
    }

    @MainActor
    private func determinePhase(for workspace: WorkspaceInfo) {
        // Use effective values that prefer session data over workspace data
        let currentTitle = effectiveSessionTitle
        let currentTodos = effectiveTodos
        let previousPhase = phase

        // Clear pendingUserPrompt if backend has started processing, completed, or timed out
        if pendingUserPrompt != nil {
            if workspace.claudeActivityState == .running {
                pendingUserPrompt = nil
                pendingUserPromptTimestamp = nil
            } else if currentTitle != nil {
                pendingUserPrompt = nil
                pendingUserPromptTimestamp = nil
            } else if let timestamp = pendingUserPromptTimestamp,
                    Date().timeIntervalSince(timestamp) > 30 {
                pendingUserPrompt = nil
                pendingUserPromptTimestamp = nil
            }
        }

        // Determine new phase based on Claude activity state and session content
        let newPhase: WorkspacePhase
        if workspace.claudeActivityState == .active || pendingUserPrompt != nil {
            newPhase = .working
        } else if currentTitle != nil || currentTodos?.isEmpty == false {
            newPhase = .completed
        } else if workspace.isDirty == true || (workspace.commitCount ?? 0) > 0 {
            newPhase = .completed
        } else {
            newPhase = .input
        }

        // Only update if changed
        if newPhase != previousPhase {
            phase = newPhase
        }

        // Fetch data based on phase
        if newPhase == .working || newPhase == .completed {
            Task {
                await fetchLatestMessage(for: workspace)
                await fetchDiffIfNeeded(for: workspace)
            }
        }
    }

    private func sendPrompt() async {
        guard let workspace = workspace, !prompt.trimmingCharacters(in: .whitespaces).isEmpty else {
            return
        }

        let promptToSend = prompt.trimmingCharacters(in: .whitespaces)
        isSubmitting = true
        error = ""

        do {
            try await CatnipAPI.shared.sendPromptToPTY(
                workspacePath: workspace.name,
                prompt: promptToSend,
                agent: "claude"
            )

            await MainActor.run {
                pendingUserPrompt = promptToSend
                pendingUserPromptTimestamp = Date()
                prompt = ""
                showPromptSheet = false
                phase = .working
                isSubmitting = false
                poller.refresh()
            }
        } catch APIError.timeout {
            await MainActor.run {
                self.error = "Claude is still starting up. Please try again in a moment."
                isSubmitting = false
            }
        } catch {
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
            // Silently fail - message fetch is best effort
        }
    }

    /// Fetch session data to hydrate context stats and other session info
    private func fetchSessionData() async {
        do {
            let result = try await CatnipAPI.shared.getSessionData(workspaceId: workspaceId, ifNoneMatch: nil)
            await MainActor.run {
                if let result = result {
                    poller.updateSessionData(result.sessionData)
                }
            }
        } catch {
            // Silently fail - session data fetch is best effort
        }
    }

    private func fetchDiffIfNeeded(for workspace: WorkspaceInfo) async {
        // Only fetch if workspace has changes
        guard (workspace.isDirty == true || (workspace.commitCount ?? 0) > 0) else {
            return
        }

        // Skip if we already have a cached diff and Claude is still actively working
        let isActivelyWorking = workspace.claudeActivityState == .active
        if cachedDiff != nil && isActivelyWorking {
            return
        }

        do {
            let diff = try await CatnipAPI.shared.getWorkspaceDiff(id: workspace.id)
            await MainActor.run {
                self.cachedDiff = diff
            }
        } catch {
            // Silently fail - diff fetch is best effort
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

        isUpdatingPR = true
        error = ""

        do {
            let prUrl = try await CatnipAPI.shared.updatePullRequest(workspaceId: workspace.id)

            await MainActor.run {
                isUpdatingPR = false

                if let url = URL(string: prUrl) {
                    UIApplication.shared.open(url)
                }

                poller.refresh()
            }
        } catch {
            await MainActor.run {
                self.error = "Failed to update PR: \(error.localizedDescription)"
                isUpdatingPR = false
            }
        }
    }

    // MARK: - Terminal View

    private var terminalView: some View {
        let worktreeName = workspace?.name ?? "unknown"
        let shouldConnect = adaptiveTheme.prefersSideBySideTerminal || showTerminalOnly

        return TerminalView(
            workspaceId: worktreeName,
            baseURL: websocketBaseURL,
            codespaceName: UserDefaults.standard.string(forKey: "codespace_name"),
            authToken: authManager.sessionToken,
            shouldConnect: shouldConnect,
            showExitButton: false
        )
    }

    private var websocketBaseURL: String {
        // Convert https://catnip.run to wss://catnip.run
        return "wss://catnip.run"
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
    @Environment(\.colorScheme) private var colorScheme

    init(_ markdown: String) {
        self.markdown = markdown
    }

    var body: some View {
        Markdown(markdown)
            .markdownTextStyle(\.text) {
                FontSize(15)
            }
            .markdownBlockStyle(\.codeBlock) { configuration in
                ScrollView(.horizontal, showsIndicators: true) {
                    configuration.label
                        .relativeLineSpacing(.em(0.225))
                        .markdownTextStyle {
                            FontFamilyVariant(.monospaced)
                            FontSize(.em(0.85))
                        }
                        .textSelection(.enabled)
                        .padding(AppTheme.Spacing.md)
                }
                .background(AppTheme.Colors.SyntaxHighlighting.background(for: colorScheme))
                .clipShape(RoundedRectangle(cornerRadius: AppTheme.Spacing.Radius.md))
                .overlay(
                    RoundedRectangle(cornerRadius: AppTheme.Spacing.Radius.md)
                        .strokeBorder(AppTheme.Colors.Separator.primary, lineWidth: 0.5)
                )
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

// MARK: - MarkdownText Previews

#Preview("Workspace Detail with Code Blocks - Light") {
    NavigationStack {
        WorkspaceDetailCodeBlockPreview(workspace: .previewWithCodeBlocks, phase: .completed)
    }
    .preferredColorScheme(.light)
}

#Preview("Workspace Detail with Code Blocks - Dark") {
    NavigationStack {
        WorkspaceDetailCodeBlockPreview(workspace: .previewWithCodeBlocks, phase: .completed)
    }
    .preferredColorScheme(.dark)
}

// Helper preview for workspace with code blocks
private struct WorkspaceDetailCodeBlockPreview: View {
    let workspace: WorkspaceInfo
    let phase: WorkspacePhase

    @State private var currentPhase: WorkspacePhase
    @State private var showSheet = false
    @State private var previewPrompt = ""

    init(workspace: WorkspaceInfo, phase: WorkspacePhase) {
        self.workspace = workspace
        self.phase = phase
        _currentPhase = State(initialValue: phase)
    }

    var body: some View {
        ZStack {
            Color(uiColor: .systemGroupedBackground)
                .ignoresSafeArea()

            ScrollView {
                VStack(spacing: 20) {
                    if currentPhase == .completed {
                        completedSectionPreview
                    }
                }
                .padding(16)
            }
        }
        .navigationTitle(workspace.latestSessionTitle ?? workspace.displayName)
        .navigationBarTitleDisplayMode(.inline)
    }

    private var completedSectionPreview: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Session content with padding
            VStack(alignment: .leading, spacing: 16) {
                // User prompt
                if let userPrompt = workspace.latestUserPrompt, !userPrompt.isEmpty {
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

                // Claude's response with code blocks
                if let claudeMessage = workspace.latestClaudeMessage, !claudeMessage.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Claude responded:")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(Color.accentColor)

                        EnhancedMarkdownText(claudeMessage)
                    }
                    .padding(12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color(uiColor: .tertiarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 10))
                }

                if let todos = workspace.todos {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Tasks:")
                            .font(.callout.weight(.semibold))
                            .foregroundStyle(.secondary)

                        TodoListView(todos: todos)
                    }
                }
            }
        }
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }
}
#endif
