//
//  WorkspaceDetailView.swift
//  catnip
//
//  Workspace detail screen with polling for updates
//

import SwiftUI

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

    private var navigationTitle: String {
        // Show session title if available (in both working and completed phases)
        if let title = workspace?.latestSessionTitle, !title.isEmpty {
            // Truncate to first line or 50 chars
            let firstLine = title.components(separatedBy: .newlines).first ?? title
            return firstLine.count > 50 ? String(firstLine.prefix(50)) + "..." : firstLine
        }
        return workspace?.displayName ?? "Workspace"
    }

    var body: some View {
        ZStack {
            Color(uiColor: .systemGroupedBackground)
                .ignoresSafeArea()

            if phase == .loading {
                loadingView
            } else if phase == .error || workspace == nil {
                errorView
            } else {
                contentView
            }
        }
        .navigationTitle(navigationTitle)
        .navigationBarTitleDisplayMode(.inline)
        .task {
            await loadWorkspace()
            poller.start()
        }
        .onDisappear {
            poller.stop()
        }
        .onChange(of: poller.workspace) { oldWorkspace, newWorkspace in
            if let newWorkspace = newWorkspace {
                NSLog("🔄 Workspace updated - activity: \(newWorkspace.claudeActivityState?.rawValue ?? "nil"), title: \(newWorkspace.latestSessionTitle?.prefix(30) ?? "nil")")
                determinePhase(for: newWorkspace)
            } else {
                NSLog("⚠️ Workspace updated to nil")
            }
        }
        .onChange(of: poller.error) { _, newError in
            if let newError = newError {
                error = newError
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
        .onAppear {
            // Auto-show sheet if no history
            if phase == .input {
                showPromptSheet = true
            }
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
                if phase == .input {
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

                // Show the user's prompt (either pending or from workspace)
                if let userPrompt = pendingUserPrompt ?? workspace?.latestUserPrompt, !userPrompt.isEmpty {
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

                if let latestSession = workspace?.latestSessionTitle, !latestSession.isEmpty {
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

                if let todos = workspace?.todos, !todos.isEmpty {
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
            }
        }
    }

    private var completedSection: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Session content with padding
            VStack(alignment: .leading, spacing: 8) {
                // User prompt
                if let userPrompt = workspace?.latestUserPrompt, !userPrompt.isEmpty {
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
                if let claudeMessage = latestMessage, !claudeMessage.isEmpty {
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
                } else if workspace?.latestSessionTitle != nil {
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

                if let todos = workspace?.todos, !todos.isEmpty {
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
            if phase == .completed {
                Button {
                    showPromptSheet = true
                } label: {
                    Text("Ask for changes")
                }
                .buttonStyle(ProminentButtonStyle())
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
        NSLog("📊 determinePhase - claudeActivityState: %@, latestSessionTitle: %@, todos: %d, isDirty: %@, commits: %d, pendingPrompt: %@",
              workspace.claudeActivityState.map { "\($0)" } ?? "nil",
              workspace.latestSessionTitle ?? "nil",
              workspace.todos?.count ?? 0,
              workspace.isDirty.map { "\($0)" } ?? "nil",
              workspace.commitCount ?? 0,
              pendingUserPrompt != nil ? "yes" : "no")

        let previousPhase = phase

        // Show "working" phase when:
        // 1. Claude is ACTIVE (actively working), OR
        // 2. We have a pending prompt (just sent a prompt but backend hasn't updated yet)
        if workspace.claudeActivityState == .active || pendingUserPrompt != nil {
            phase = .working

            // Fetch diff if workspace has changes
            Task {
                await fetchDiffIfNeeded(for: workspace)
            }
        } else if workspace.latestSessionTitle != nil || workspace.todos?.isEmpty == false {
            // Has a session title or todos - definitely completed
            phase = .completed

            // Clear pending prompt when transitioning to completed
            if previousPhase == .working {
                pendingUserPrompt = nil
                NSLog("📊 Cleared pending prompt on transition to completed")
            }

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
            NSLog("🐱 [WorkspaceDetailView] About to call sendPrompt API...")
            try await CatnipAPI.shared.sendPrompt(workspacePath: workspace.name, prompt: promptToSend)
            NSLog("🐱 [WorkspaceDetailView] ✅ Successfully sent prompt")

            await MainActor.run {
                // Store the prompt we just sent for immediate display
                pendingUserPrompt = promptToSend
                NSLog("🐱 [WorkspaceDetailView] Stored pending prompt: \(promptToSend.prefix(50))...")

                prompt = ""
                showPromptSheet = false
                phase = .working
                isSubmitting = false

                // Trigger immediate refresh after sending prompt
                NSLog("🐱 [WorkspaceDetailView] Triggering poller refresh")
                poller.refresh()
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

    private func fetchDiffIfNeeded(for workspace: WorkspaceInfo) async {
        // Only fetch if workspace has changes and we don't already have a diff
        guard (workspace.isDirty == true || (workspace.commitCount ?? 0) > 0) else {
            NSLog("📊 No changes to fetch diff for")
            return
        }

        // Skip if we already have a cached diff, UNLESS Claude is actively working
        // (we want to keep refetching to show live updates)
        if cachedDiff != nil && workspace.claudeActivityState != .active {
            NSLog("📊 Diff already cached and workspace inactive, skipping fetch")
            return
        }

        NSLog("📊 Fetching diff for workspace with changes (dirty: %@, commits: %d, active: %@)",
              workspace.isDirty.map { "\($0)" } ?? "nil",
              workspace.commitCount ?? 0,
              workspace.claudeActivityState == .active ? "yes" : "no")

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
                pullRequestUrl: nil,
                path: workspace.path,
                cacheStatus: workspace.cacheStatus
            )
        case .working:
            workspace = WorkspaceInfo(
                id: workspace.id,
                name: workspace.name,
                branch: workspace.branch,
                repoId: workspace.repoId,
                claudeActivityState: .active,
                commitCount: workspace.commitCount,
                isDirty: workspace.isDirty,
                lastAccessed: workspace.lastAccessed,
                createdAt: workspace.createdAt,
                todos: Todo.previewList,
                latestSessionTitle: "Implementing new feature",
                latestUserPrompt: nil,
                pullRequestUrl: nil,
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

// MARK: - Markdown Text Component

struct MarkdownText: View {
    let markdown: String

    init(_ markdown: String) {
        self.markdown = markdown
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            // Split by double newlines to handle paragraphs
            let paragraphs = markdown.components(separatedBy: "\n\n").filter { !$0.isEmpty }

            ForEach(Array(paragraphs.enumerated()), id: \.offset) { index, paragraph in
                if let attributedString = try? AttributedString(markdown: paragraph, options: .init(interpretedSyntax: .full)) {
                    Text(attributedString)
                        .font(.body)
                        .foregroundStyle(.primary)
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                } else {
                    // Fallback to plain text if markdown parsing fails
                    Text(paragraph)
                        .font(.body)
                        .foregroundStyle(.primary)
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        }
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
            .background(Color(uiColor: .secondarySystemBackground).opacity(0.95))
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
