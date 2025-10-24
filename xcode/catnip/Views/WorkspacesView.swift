//
//  WorkspacesView.swift
//  catnip
//
//  Workspaces list screen
//

import SwiftUI

struct WorkspacesView: View {
    @State private var workspaces: [WorkspaceInfo] = []
    @State private var isLoading = true
    @State private var isRefreshing = false
    @State private var error: String?
    @State private var showCreateSheet = false
    @State private var createPrompt = ""
    @State private var selectedRepository = ""
    @State private var selectedBranch: String? = nil
    @State private var isCreating = false
    @State private var availableBranches: [String] = []
    @State private var branchesLoading = false
    @State private var createSheetError: String? // Separate error for create sheet
    @State private var deleteConfirmation: WorkspaceInfo? // Workspace to delete
    @State private var navigationWorkspace: WorkspaceInfo? // Workspace to navigate to
    @State private var pendingPromptForNavigation: String? // Prompt to pass to detail view
    @State private var createdWorkspaceForRetry: WorkspaceInfo? // Track created workspace for retry on 408 timeout

    // Claude authentication
    @State private var showClaudeAuthSheet = false
    @State private var hasCheckedClaudeAuth = false

    private var availableRepositories: [String] {
        Array(Set(workspaces.map { $0.repoId })).sorted()
    }

    var body: some View {
        ZStack {
            Color(uiColor: .systemGroupedBackground)
                .ignoresSafeArea()

            if isLoading {
                loadingView
            } else if let error = error {
                errorView(error)
            } else if workspaces.isEmpty {
                emptyView
            } else {
                listView
            }
        }
        .navigationTitle("Workspaces")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .navigationBarTrailing) {
                Button(action: { showCreateSheet = true }) {
                    Image(systemName: "plus")
                }
            }
        }
        .task {
            await loadWorkspaces()

            // Check Claude authentication status
            if !hasCheckedClaudeAuth {
                await checkClaudeAuth()
            }
        }
        .refreshable {
            await loadWorkspaces()
        }
        .sheet(isPresented: $showCreateSheet) {
            CreateWorkspaceSheet(
                isPresented: $showCreateSheet,
                prompt: $createPrompt,
                selectedRepository: $selectedRepository,
                selectedBranch: $selectedBranch,
                availableRepositories: availableRepositories,
                availableBranches: availableBranches,
                branchesLoading: branchesLoading,
                isCreating: isCreating,
                error: createSheetError,
                onCreate: { Task { await createWorkspace() } }
            )
        }
        .onChange(of: showCreateSheet) {
            if showCreateSheet {
                // Pre-select most recently used repository
                if let mostRecent = workspaces.first {
                    selectedRepository = mostRecent.repoId
                    // Don't pre-select branch - let fetchBranches set the default
                    selectedBranch = nil
                    // Fetch branches for the selected repository
                    Task {
                        await fetchBranches()
                    }
                }
            } else {
                // Reset state when sheet closes
                createPrompt = ""
                selectedRepository = ""
                selectedBranch = nil
                availableBranches = []
                createSheetError = nil
                createdWorkspaceForRetry = nil // Clear retry state
            }
        }
        .onChange(of: selectedRepository) {
            // When repository changes, fetch its branches
            if showCreateSheet && !selectedRepository.isEmpty {
                selectedBranch = nil // Reset branch selection
                Task {
                    await fetchBranches()
                }
            }
        }
        .navigationDestination(item: $navigationWorkspace) { workspace in
            WorkspaceDetailView(
                workspaceId: workspace.id,
                initialWorkspace: workspace,
                pendingPrompt: pendingPromptForNavigation
            )
        }
        .onChange(of: navigationWorkspace) {
            // Clear pending prompt after navigation completes
            if navigationWorkspace == nil && pendingPromptForNavigation != nil {
                pendingPromptForNavigation = nil
                NSLog("🐱 [WorkspacesView] Cleared pendingPromptForNavigation after navigation")
            }
        }
        .sheet(isPresented: $showClaudeAuthSheet) {
            let codespaceName = UserDefaults.standard.string(forKey: "codespace_name") ?? "unknown"
            ClaudeAuthSheet(isPresented: $showClaudeAuthSheet, codespaceName: codespaceName) {
                NSLog("🐱 [WorkspacesView] Claude authentication completed")
                // Optionally refresh workspaces or perform other actions
            }
        }
    }

    private var loadingView: some View {
        VStack(spacing: 16) {
            ProgressView()
                .scaleEffect(1.5)
            Text("Loading workspaces...")
                .font(.body)
                .foregroundStyle(.secondary)
        }
    }

    private func errorView(_ message: String) -> some View {
        ScrollView {
            VStack(spacing: 20) {
                Text("Error loading workspaces")
                    .font(.title2)
                    .foregroundStyle(.primary)

                Text(message)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.leading)
                    .textSelection(.enabled)
                    .padding()
                    .background(Color(uiColor: .secondarySystemBackground))
                    .cornerRadius(8)

                Button {
                    Task { await loadWorkspaces() }
                } label: {
                    Text("Retry")
                }
                .buttonStyle(ProminentButtonStyle())
                .padding(.horizontal, 20)
            }
            .padding()
        }
    }

    private var emptyView: some View {
        VStack(spacing: 20) {
            Image(systemName: "folder")
                .font(.system(size: 56))
                .foregroundStyle(.secondary)

            Text("No workspaces")
                .font(.title2.weight(.semibold))
                .foregroundStyle(.primary)

            Text("Create a workspace to get started")
                .font(.body)
                .foregroundStyle(.secondary)

            Button {
                showCreateSheet = true
            } label: {
                Text("Create Workspace")
            }
            .buttonStyle(ProminentButtonStyle())
            .padding(.horizontal, 20)
        }
        .padding()
    }

    private var listView: some View {
        List {
            ForEach(workspaces) { workspace in
                Button {
                    navigationWorkspace = workspace
                } label: {
                    WorkspaceCard(workspace: workspace)
                }
                .buttonStyle(.plain)
                .listRowInsets(EdgeInsets(top: 0, leading: 0, bottom: 0, trailing: 8))
                .listRowSeparator(.visible)
                .listRowBackground(Color(uiColor: .secondarySystemBackground))
                .accessibilityIdentifier("workspace-\(workspace.id)")
                .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                    Button(role: .destructive) {
                        deleteConfirmation = workspace
                    } label: {
                        Label("Delete", systemImage: "trash")
                    }
                }
            }
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .accessibilityIdentifier("workspacesList")
        .alert("Delete Workspace", isPresented: Binding(
            get: { deleteConfirmation != nil },
            set: { if !$0 { deleteConfirmation = nil } }
        )) {
            Button("Cancel", role: .cancel) {
                deleteConfirmation = nil
            }
            Button("Delete", role: .destructive) {
                if let workspace = deleteConfirmation {
                    Task {
                        await deleteWorkspace(workspace)
                    }
                }
            }
        } message: {
            if let workspace = deleteConfirmation {
                let changesList = [
                    workspace.isDirty == true ? "uncommitted changes" : nil,
                    (workspace.commitCount ?? 0) > 0 ? "\(workspace.commitCount ?? 0) commits" : nil
                ].compactMap { $0 }

                if !changesList.isEmpty {
                    Text("Delete workspace \"\(workspace.displayName)\"? This workspace has \(changesList.joined(separator: " and ")). This action cannot be undone.")
                } else {
                    Text("Delete workspace \"\(workspace.displayName)\"? This action cannot be undone.")
                }
            }
        }
    }

    private func loadWorkspaces() async {
        // Only show loading spinner if we have no data yet (initial load)
        // This allows refreshes to happen in the background without disrupting the UI
        await MainActor.run {
            if workspaces.isEmpty {
                isLoading = true
            }
            error = nil
        }

        // Use mock data in UI testing mode
        if UITestingHelper.shouldUseMockData {
            await MainActor.run {
                workspaces = UITestingHelper.getMockWorkspaces().sorted { w1, w2 in
                    let time1 = w1.lastAccessed ?? w1.createdAt ?? ""
                    let time2 = w2.lastAccessed ?? w2.createdAt ?? ""
                    return time1 > time2
                }
                isLoading = false
            }
            return
        }

        do {
            guard let result = try await CatnipAPI.shared.getWorkspaces() else {
                // 304 Not Modified - unlikely on initial load, but handle it
                await MainActor.run {
                    isLoading = false
                }
                return
            }

            await MainActor.run {
                workspaces = result.workspaces.sorted { w1, w2 in
                    let time1 = w1.lastAccessed ?? w1.createdAt ?? ""
                    let time2 = w2.lastAccessed ?? w2.createdAt ?? ""
                    return time1 > time2
                }
                isLoading = false
            }
        } catch {
            await MainActor.run {
                // Only show error if we have no data (initial load failed)
                // Otherwise, keep showing existing data and silently fail the refresh
                if workspaces.isEmpty {
                    if let apiError = error as? APIError {
                        self.error = apiError.errorDescription ?? "Unknown error"
                    } else {
                        self.error = "\(error)"
                    }
                }
                isLoading = false
            }
        }
    }

    private func fetchBranches() async {
        guard !selectedRepository.isEmpty else { return }

        branchesLoading = true
        createSheetError = nil // Clear any previous errors

        do {
            let branches = try await CatnipAPI.shared.fetchBranches(repoId: selectedRepository)

            // Filter out workspace-specific branches (refs/catnip/*)
            let filteredBranches = branches.filter { branch in
                !branch.hasPrefix("refs/catnip/") && !branch.hasPrefix("catnip/")
            }

            await MainActor.run {
                // Sort branches: default branch first, then alphabetical
                let sortedBranches = filteredBranches.sorted { branch1, branch2 in
                    let isDefault1 = (branch1 == "main" || branch1 == "master")
                    let isDefault2 = (branch2 == "main" || branch2 == "master")

                    if isDefault1 && !isDefault2 {
                        return true  // branch1 (default) comes first
                    } else if !isDefault1 && isDefault2 {
                        return false // branch2 (default) comes first
                    } else {
                        return branch1 < branch2 // alphabetical for non-defaults
                    }
                }

                availableBranches = sortedBranches

                // Set default branch if no branch is currently selected
                if selectedBranch == nil || selectedBranch?.isEmpty == true {
                    // Look for common default branch names
                    if let defaultBranch = sortedBranches.first(where: { $0 == "main" || $0 == "master" }) {
                        selectedBranch = defaultBranch
                    } else if !sortedBranches.isEmpty {
                        selectedBranch = sortedBranches[0]
                    }
                }
                branchesLoading = false
            }
        } catch let fetchError {
            await MainActor.run {
                // Show error to user in create sheet only
                if let apiError = fetchError as? APIError {
                    self.createSheetError = "Failed to fetch branches: \(apiError.errorDescription ?? "Unknown error")"
                } else {
                    self.createSheetError = "Failed to fetch branches: \(fetchError.localizedDescription)"
                }
                availableBranches = []
                branchesLoading = false
            }
        }
    }

    private func createWorkspace() async {
        guard !selectedRepository.isEmpty else { return }

        isCreating = true
        createSheetError = nil // Clear any previous errors

        do {
            // Check if we're retrying with an existing workspace
            let workspace: WorkspaceInfo
            let isRetry = createdWorkspaceForRetry != nil

            if let existingWorkspace = createdWorkspaceForRetry {
                // Retry case: reuse the already-created workspace
                workspace = existingWorkspace
                NSLog("🐱 [WorkspacesView] Retrying with existing workspace: \(workspace.id)")
            } else {
                // First attempt: create new workspace
                workspace = try await CatnipAPI.shared.createWorkspace(
                    orgRepo: selectedRepository,
                    branch: selectedBranch
                )
                NSLog("🐱 [WorkspacesView] Created new workspace: \(workspace.id)")

                // Save for potential retry
                await MainActor.run {
                    createdWorkspaceForRetry = workspace
                }

                // HACKY: Wait for workspace directory to be fully created on disk
                // TODO: Fix the backend checkout endpoint to not return 200 until directory is ready
                NSLog("🐱 [WorkspacesView] ⏰ Waiting 2 seconds for workspace directory to be ready...")
                try await Task.sleep(nanoseconds: 2_000_000_000) // 2 seconds
            }

            // Store the prompt to send
            let promptToSend = createPrompt.trimmingCharacters(in: .whitespaces)
            let workspaceName = workspace.name

            // Start PTY session BEFORE sending prompt (skip if retrying - already started)
            if !promptToSend.isEmpty && !isRetry {
                NSLog("🐱 [WorkspacesView] Starting PTY session for workspace: \(workspace.id)")
                do {
                    try await CatnipAPI.shared.startPTY(workspacePath: workspaceName, agent: "claude")
                    NSLog("🐱 [WorkspacesView] ✅ PTY session started successfully")
                } catch {
                    NSLog("🐱 [WorkspacesView] ❌ Failed to start PTY session: \(error)")
                    // Show error to user and keep sheet open for retry
                    await MainActor.run {
                        if !workspaces.contains(where: { $0.id == workspace.id }) {
                            workspaces.insert(workspace, at: 0)
                        }
                        if let apiError = error as? APIError {
                            self.createSheetError = "Workspace created, but failed to start PTY session: \(apiError.errorDescription ?? "Unknown error"). Try submitting again or navigate to the workspace."
                        } else {
                            self.createSheetError = "Workspace created, but failed to start PTY session: \(error.localizedDescription). Try submitting again or navigate to the workspace."
                        }
                        isCreating = false
                    }
                    return
                }

                // Wait a bit for PTY to be ready (backend now waits up to 15s, so 2s here is reasonable)
                NSLog("🐱 [WorkspacesView] ⏰ Waiting 2 seconds for PTY to initialize...")
                try await Task.sleep(nanoseconds: 2_000_000_000) // 2 seconds
            }

            // Send prompt BEFORE navigating (so we can handle errors properly)
            if !promptToSend.isEmpty {
                NSLog("🐱 [WorkspacesView] Sending initial prompt to workspace: \(workspace.id)")
                NSLog("🐱 [WorkspacesView] Prompt length: \(promptToSend.count) chars")

                do {
                    NSLog("🐱 [WorkspacesView] About to call sendPromptToPTY API...")
                    try await CatnipAPI.shared.sendPromptToPTY(
                        workspacePath: workspaceName,
                        prompt: promptToSend,
                        agent: "claude"
                    )
                    NSLog("🐱 [WorkspacesView] ✅ Successfully sent initial prompt")

                    // Set pendingPrompt for detail view
                    await MainActor.run {
                        pendingPromptForNavigation = promptToSend
                        NSLog("🐱 [WorkspacesView] Set pendingPromptForNavigation: \(promptToSend.prefix(50))...")
                    }
                } catch {
                    NSLog("🐱 [WorkspacesView] ❌ Failed to send initial prompt: \(error)")
                    // Show error to user and keep sheet open for retry
                    await MainActor.run {
                        if !workspaces.contains(where: { $0.id == workspace.id }) {
                            workspaces.insert(workspace, at: 0)
                        }
                        if let apiError = error as? APIError {
                            self.createSheetError = "Workspace created, but failed to send prompt: \(apiError.errorDescription ?? "Unknown error"). Try submitting again or navigate to the workspace."
                        } else {
                            self.createSheetError = "Workspace created, but failed to send prompt: \(error.localizedDescription). Try submitting again or navigate to the workspace."
                        }
                        isCreating = false
                    }
                    return
                }
            } else {
                NSLog("🐱 [WorkspacesView] No prompt to send (empty)")
            }

            // Success! Close sheet and navigate
            await MainActor.run {
                if !workspaces.contains(where: { $0.id == workspace.id }) {
                    workspaces.insert(workspace, at: 0)
                }
                showCreateSheet = false
                isCreating = false
                createdWorkspaceForRetry = nil // Clear retry state on success
                navigationWorkspace = workspace
                NSLog("🚀 Navigating to newly created workspace: \(workspace.id)")
            }
        } catch {
            await MainActor.run {
                // Show error to user
                if let apiError = error as? APIError {
                    self.createSheetError = "Failed to create workspace: \(apiError.errorDescription ?? "Unknown error")"
                } else {
                    self.createSheetError = "Failed to create workspace: \(error.localizedDescription)"
                }
                isCreating = false
            }
        }
    }

    private func deleteWorkspace(_ workspace: WorkspaceInfo) async {
        do {
            try await CatnipAPI.shared.deleteWorkspace(id: workspace.id)

            // Remove from local list
            await MainActor.run {
                workspaces.removeAll { $0.id == workspace.id }
                deleteConfirmation = nil
            }
        } catch {
            await MainActor.run {
                // Show error to user
                if let apiError = error as? APIError {
                    self.error = "Failed to delete workspace: \(apiError.errorDescription ?? "Unknown error")"
                } else {
                    self.error = "Failed to delete workspace: \(error.localizedDescription)"
                }
                deleteConfirmation = nil
            }
        }
    }

    private func checkClaudeAuth() async {
        await MainActor.run {
            hasCheckedClaudeAuth = true
        }

        // Skip during UI testing
        if UITestingHelper.isUITesting {
            return
        }

        // Get the codespace name to scope the dismissal check
        let codespaceName = UserDefaults.standard.string(forKey: "codespace_name") ?? "unknown"

        // Check if user has dismissed the auth prompt for this codespace in this session
        let dismissed: Bool = SessionStorage.shared.get(forKey: "claude-auth-dismissed", scope: codespaceName) ?? false
        if dismissed {
            NSLog("🐱 [WorkspacesView] Claude auth was previously dismissed for codespace '\(codespaceName)', skipping check")
            return
        }

        do {
            let settings = try await CatnipAPI.shared.getClaudeSettings()

            // Show auth sheet if Claude is not authenticated OR hasn't completed onboarding
            // This mirrors the web app logic in claude-auth-context.tsx
            if !settings.authenticated || !settings.hasCompletedOnboarding {
                NSLog("🐱 [WorkspacesView] Claude needs onboarding (authenticated: \(settings.authenticated), completed: \(settings.hasCompletedOnboarding)), showing auth sheet")
                await MainActor.run {
                    showClaudeAuthSheet = true
                }
            } else {
                NSLog("🐱 [WorkspacesView] Claude is authenticated and onboarding complete")
            }
        } catch {
            NSLog("🐱 [WorkspacesView] Failed to check Claude auth status: \(error)")
            // Don't show error to user - auth check is optional
        }
    }
}

struct WorkspaceCard: View {
    let workspace: WorkspaceInfo

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            // Title row - session title
            HStack(alignment: .center, spacing: 8) {
                Text(workspace.activityDescription ?? "New Workspace")
                    .font(.headline)
                    .foregroundStyle(.primary)
                    .lineLimit(1)

                Spacer()

                Text(workspace.timeDisplay)
                    .font(.callout)
                    .foregroundStyle(.tertiary)
            }

            // Workspace name subtitle with status indicator
            HStack(spacing: 8) {
                StatusIndicator(status: workspace.claudeActivityState)

                Text(workspace.displayName)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }

            // Branch info with badges
            HStack(spacing: 8) {
                // Badges inline with branch
                if workspace.isDirty == true || (workspace.commitCount ?? 0) > 0 {
                    HStack(spacing: 6) {
                        if workspace.isDirty == true {
                            Text("MODIFIED")
                                .font(.caption2.weight(.semibold))
                                .foregroundStyle(.secondary)
                                .padding(.horizontal, 8)
                                .padding(.vertical, 3)
                                .background(Color(uiColor: .tertiarySystemBackground))
                                .clipShape(RoundedRectangle(cornerRadius: 4))
                        }

                        if let count = workspace.commitCount, count > 0 {
                            Text("+\(count)")
                                .font(.caption2.weight(.semibold))
                                .foregroundStyle(Color.accentColor)
                                .padding(.horizontal, 8)
                                .padding(.vertical, 3)
                                .background(Color.accentColor.opacity(0.15))
                                .clipShape(RoundedRectangle(cornerRadius: 4))
                        }
                    }
                }

                Text(workspace.cleanBranch)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)

                Spacer()
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
    }
}

struct CreateWorkspaceSheet: View {
    @Binding var isPresented: Bool
    @Binding var prompt: String
    @Binding var selectedRepository: String
    @Binding var selectedBranch: String?
    let availableRepositories: [String]
    let availableBranches: [String]
    let branchesLoading: Bool
    let isCreating: Bool
    let error: String?
    let onCreate: () -> Void

    @FocusState private var isTextFieldFocused: Bool

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                // Error display
                if let error = error {
                    HStack(spacing: 8) {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .foregroundColor(.orange)
                        Text(error)
                            .font(.caption)
                            .foregroundColor(.secondary)
                    }
                    .padding(.horizontal, 20)
                    .padding(.vertical, 8)
                    .background(Color.orange.opacity(0.1))
                    .cornerRadius(8)
                    .padding(.horizontal, 20)
                    .padding(.top, 16)
                }

                // Repository selector
                if !availableRepositories.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Repository")
                            .font(.subheadline.weight(.medium))
                            .foregroundStyle(.secondary)
                            .padding(.horizontal, 20)

                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack(spacing: 8) {
                                ForEach(availableRepositories, id: \.self) { repo in
                                    Button(action: { selectedRepository = repo }) {
                                        Text(repo)
                                            .font(.subheadline)
                                            .padding(.horizontal, 14)
                                            .padding(.vertical, 8)
                                            .background(
                                                RoundedRectangle(cornerRadius: 20)
                                                    .fill(selectedRepository == repo ? Color.accentColor.opacity(0.15) : Color(uiColor: .secondarySystemBackground))
                                            )
                                            .overlay(
                                                RoundedRectangle(cornerRadius: 20)
                                                    .strokeBorder(
                                                        selectedRepository == repo ? Color.accentColor : Color.clear,
                                                        lineWidth: 1.5
                                                    )
                                            )
                                            .foregroundStyle(selectedRepository == repo ? Color.accentColor : .secondary)
                                    }
                                }
                            }
                            .padding(.horizontal, 20)
                        }
                    }
                    .padding(.top, 16)
                }

                // Prompt input area
                ZStack(alignment: .topLeading) {
                    if prompt.isEmpty {
                        Text(placeholderText)
                            .foregroundStyle(.tertiary)
                            .padding(.horizontal, 4)
                            .padding(.top, 8)
                    }

                    TextEditor(text: $prompt)
                        .focused($isTextFieldFocused)
                        .scrollContentBackground(.hidden)
                        .frame(minHeight: 100, maxHeight: 250)
                }
                .padding(.horizontal, 20)
                .padding(.top, 16)

                // Branch selector
                if !selectedRepository.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Branch (optional)")
                            .font(.subheadline.weight(.medium))
                            .foregroundStyle(.secondary)
                            .padding(.horizontal, 20)

                        if branchesLoading {
                            HStack(spacing: 8) {
                                ProgressView()
                                    .scaleEffect(0.8)
                                Text("Loading branches...")
                                    .font(.subheadline)
                                    .foregroundStyle(.secondary)
                            }
                            .padding(.horizontal, 20)
                            .padding(.vertical, 8)
                        } else if !availableBranches.isEmpty {
                            ScrollView(.horizontal, showsIndicators: false) {
                                HStack(spacing: 8) {
                                    ForEach(availableBranches, id: \.self) { branch in
                                        Button(action: { selectedBranch = branch }) {
                                            HStack(spacing: 6) {
                                                Image(systemName: "arrow.branch")
                                                    .font(.caption)
                                                Text(branch)
                                                    .font(.subheadline)
                                            }
                                            .padding(.horizontal, 14)
                                            .padding(.vertical, 8)
                                            .background(
                                                RoundedRectangle(cornerRadius: 20)
                                                    .fill(selectedBranch == branch ? Color.accentColor.opacity(0.15) : Color(uiColor: .secondarySystemBackground))
                                            )
                                            .overlay(
                                                RoundedRectangle(cornerRadius: 20)
                                                    .strokeBorder(
                                                        selectedBranch == branch ? Color.accentColor : Color.clear,
                                                        lineWidth: 1.5
                                                    )
                                            )
                                            .foregroundStyle(selectedBranch == branch ? Color.accentColor : .secondary)
                                        }
                                    }
                                }
                                .padding(.horizontal, 20)
                            }
                        }
                    }
                    .padding(.top, 12)
                }

                Spacer()

                // Submit button
                HStack {
                    Spacer()

                    Button {
                        if canSubmit {
                            onCreate()
                        }
                    } label: {
                        Group {
                            if isCreating {
                                ProgressView()
                                    .progressViewStyle(CircularProgressViewStyle(tint: .white))
                                    .scaleEffect(0.8)
                            } else {
                                Image(systemName: "arrow.up")
                                    .font(.body.weight(.semibold))
                            }
                        }
                        .frame(width: 32, height: 32)
                        .foregroundStyle(.white)
                        .background(
                            Circle()
                                .fill(canSubmit ? Color.accentColor : Color.gray.opacity(0.3))
                        )
                    }
                    .disabled(!canSubmit)
                }
                .padding(.horizontal, 20)
                .padding(.bottom, 20)
            }
            .background(Color(uiColor: .systemBackground))
            .navigationTitle("New Workspace")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .navigationBarLeading) {
                    Button("Cancel") {
                        isPresented = false
                    }
                    .foregroundStyle(.primary)
                }
            }
            .onAppear {
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) {
                    isTextFieldFocused = true
                }
            }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }

    private var placeholderText: String {
        if !selectedRepository.isEmpty {
            let branchText = selectedBranch ?? "default branch"
            return "Describe a coding task in \(selectedRepository) @ \(branchText)"
        }
        return "Select a repository and describe your coding task"
    }

    private var canSubmit: Bool {
        !selectedRepository.isEmpty && !isCreating
    }
}

#Preview("Workspaces List") {
    NavigationStack {
        WorkspacesListPreview()
    }
}

#Preview("Empty State") {
    NavigationStack {
        EmptyStatePreview()
    }
}

#Preview("Workspace Cards") {
    List {
        WorkspaceCard(workspace: .preview1)
        WorkspaceCard(workspace: .preview2)
        WorkspaceCard(workspace: .preview3)
    }
    .listStyle(.plain)
}

// Preview helpers
private struct WorkspacesListPreview: View {
    @StateObject private var authManager = MockAuthManager() as AuthManager
    @State private var showCreateSheet = false

    var body: some View {
        ZStack {
            Color(uiColor: .systemGroupedBackground)
                .ignoresSafeArea()

            List {
                ForEach(WorkspaceInfo.previewList) { workspace in
                    WorkspaceCard(workspace: workspace)
                        .listRowInsets(EdgeInsets())
                        .listRowSeparator(.visible)
                        .listRowBackground(Color(uiColor: .secondarySystemBackground))
                }
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
        }
        .navigationTitle("Workspaces")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .navigationBarTrailing) {
                Button(action: { showCreateSheet = true }) {
                    Image(systemName: "plus")
                }
            }
        }
        .sheet(isPresented: $showCreateSheet) {
            CreateWorkspaceSheet(
                isPresented: $showCreateSheet,
                prompt: .constant(""),
                selectedRepository: .constant("wandb/catnip"),
                selectedBranch: .constant(nil),
                availableRepositories: ["wandb/catnip", "acme/project"],
                availableBranches: ["main", "feature/api-docs", "bugfix/auth-token"],
                branchesLoading: false,
                isCreating: false,
                error: nil,
                onCreate: { showCreateSheet = false }
            )
        }
    }
}

private struct EmptyStatePreview: View {
    @State private var showSheet = false

    var body: some View {
        ZStack {
            Color(uiColor: .systemGroupedBackground)
                .ignoresSafeArea()

            VStack(spacing: 20) {
                Image(systemName: "folder")
                    .font(.system(size: 56))
                    .foregroundStyle(.secondary)

                Text("No workspaces")
                    .font(.title2.weight(.semibold))
                    .foregroundStyle(.primary)

                Text("Create a workspace to get started")
                    .font(.body)
                    .foregroundStyle(.secondary)

                Button {
                    showSheet = true
                } label: {
                    Text("Create Workspace")
                }
                .buttonStyle(ProminentButtonStyle())
                .padding(.horizontal, 20)
            }
            .padding()
        }
        .navigationTitle("Workspaces")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .navigationBarTrailing) {
                Button(action: { showSheet = true }) {
                    Image(systemName: "plus")
                }
            }
        }
        .sheet(isPresented: $showSheet) {
            CreateWorkspaceSheet(
                isPresented: $showSheet,
                prompt: .constant(""),
                selectedRepository: .constant("wandb/catnip"),
                selectedBranch: .constant(nil),
                availableRepositories: ["wandb/catnip", "acme/project"],
                availableBranches: ["main", "feature/docs"],
                branchesLoading: false,
                isCreating: false,
                error: nil,
                onCreate: { showSheet = false }
            )
        }
    }
}
