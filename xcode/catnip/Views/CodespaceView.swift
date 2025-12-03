//
//  CodespaceView.swift
//  catnip
//
//  Codespace connection screen with SSE support
//

import SwiftUI
import MarkdownUI

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

enum RepositoryListMode {
    case installation  // Show repos without Catnip, action = install
    case launch        // Show repos with Catnip, action = launch
}

struct CodespaceView: View {
    @Environment(\.scenePhase) private var scenePhase
    @EnvironmentObject var authManager: AuthManager
    @StateObject private var installer = CatnipInstaller.shared
    @StateObject private var tracker = CodespaceCreationTracker.shared
    @State private var phase: CodespacePhase = .connect
    @State private var statusMessage: String = ""
    @State private var errorMessage: String = ""
    @State private var codespaces: [CodespaceInfo] = []
    @State private var sseService: SSEService?
    @State private var navigateToWorkspaces = false
    @State private var currentCatFact: String = ""
    @State private var installationResult: InstallationResult?
    @State private var createdCodespace: CodespaceCreationResult.CodespaceInfo?
    @State private var repositoryListMode: RepositoryListMode = .installation
    @State private var pendingRepository: String?
    @State private var pendingCodespaceName: String?
    @State private var wasConnectingBeforeBackground = false
    @State private var shouldAutoReconnect = false

    private let catFacts = [
        "Cats can rotate their ears 180 degrees.",
        "A group of cats is called a 'clowder'.",
        "Cats spend 70% of their lives sleeping.",
        "A cat's purr vibrates at a frequency that promotes bone healing.",
        "Cats have 32 muscles in each ear.",
        "A cat can jump up to six times its length.",
        "Cats have a third eyelid called a 'haw'.",
        "A cat's nose print is unique, like a human fingerprint.",
        "Cats can make over 100 different sounds.",
        "The world's longest cat measured 48.5 inches long."
    ]

    var body: some View {
        ZStack {
            if phase == .createRepository {
                createRepositoryView
            } else if phase == .setup {
                setupView
            } else if phase == .selection {
                selectionView
            } else if phase == .repositorySelection {
                repositorySelectionView
            } else if phase == .installing {
                installingView
            } else if phase == .creatingCodespace {
                creatingCodespaceView
            } else {
                connectView
            }
        }
        .navigationTitle("Catnip")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .navigationBarLeading) {
                if phase == .createRepository || phase == .setup || phase == .selection || phase == .repositorySelection || phase == .creatingCodespace {
                    Button {
                        // If creating, keep it running in background
                        if tracker.isCreating {
                            NSLog("🎯 User navigating away while creation continues")
                        }

                        phase = .connect
                        errorMessage = ""

                        // Clear background reconnection flag when manually navigating back
                        wasConnectingBeforeBackground = false

                        // Only reset installer if there's an error or not creating
                        if installer.error != nil || !tracker.isCreating {
                            installer.reset()
                        }
                    } label: {
                        Image(systemName: "chevron.left")
                    }
                }
            }

            ToolbarItem(placement: .navigationBarTrailing) {
                Menu {
                    if !authManager.isPreviewMode {
                        Button {
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
                        } label: {
                            Label("Install Catnip", systemImage: "plus.rectangle.on.folder")
                        }
                    }

                    if authManager.isPreviewMode {
                        Button(role: .destructive) {
                            authManager.exitPreviewMode()
                        } label: {
                            Label("Exit Preview", systemImage: "xmark.circle")
                        }
                    } else {
                        Button(role: .destructive) {
                            Task { await authManager.logout() }
                        } label: {
                            Label("Logout", systemImage: "rectangle.portrait.and.arrow.right")
                        }
                        .disabled(phase == .connecting)
                    }
                } label: {
                    Image(systemName: "ellipsis")
                        .imageScale(.large)
                        .fontWeight(.bold)
                }
                .disabled(phase == .connecting)
                .accessibilityIdentifier("moreOptionsButton")
            }
        }
        .navigationDestination(isPresented: $navigateToWorkspaces) {
            WorkspacesView()
        }
        .onChange(of: navigateToWorkspaces) {
            // Reset state when returning from workspaces view
            if !navigateToWorkspaces {
                phase = .connect
                statusMessage = ""
                errorMessage = ""

                // Refresh user status and repositories when returning to connect screen (skip in UI testing)
                if !UITestingHelper.isUITesting {
                    Task {
                        do {
                            try await installer.fetchUserStatus()
                            NSLog("🐱 [CodespaceView] Refreshed user status on return to connect")
                        } catch {
                            NSLog("🐱 [CodespaceView] Failed to refresh user status: \(error)")
                        }
                    }
                }
            }
        }
        .onChange(of: phase) {
            // Refresh user status and repositories when returning to connect screen from other flows (skip in UI testing)
            if phase == .connect && !UITestingHelper.isUITesting {
                Task {
                    do {
                        try await installer.fetchUserStatus()
                        NSLog("🐱 [CodespaceView] Refreshed user status on phase change to connect")
                    } catch {
                        NSLog("🐱 [CodespaceView] Failed to refresh user status: \(error)")
                    }
                }

                // Reload repositories to ensure cache is populated after reset()
                Task {
                    do {
                        try await installer.fetchRepositories()
                        NSLog("🐱 [CodespaceView] Refreshed repositories on phase change to connect")
                    } catch {
                        NSLog("🐱 [CodespaceView] Failed to refresh repositories: \(error)")
                    }
                }

                // Auto-connect if we just finished creating a codespace
                if let codespace = createdCodespace {
                    NSLog("🐱 [CodespaceView] Auto-connecting to newly created codespace: \(codespace.name)")
                    handleConnect(codespaceName: codespace.name)
                    // Clear the created codespace so we don't auto-connect again
                    createdCodespace = nil
                }
            }
        }
        .task {
            // Auto-navigate to workspaces in UI testing mode
            if UITestingHelper.shouldAutoNavigateToWorkspaces() {
                UserDefaults.standard.set("mock-codespace", forKey: "codespace_name")
                await MainActor.run {
                    navigateToWorkspaces = true
                }
                return
            }

            // Skip network calls in UI testing mode (unless using mock data)
            // When using mock data, we still need to call fetch methods to populate the installer
            if UITestingHelper.isUITesting && !UITestingHelper.shouldUseMockData {
                return
            }

            // Fetch user status for conditional UI
            Task {
                do {
                    try await installer.fetchUserStatus()
                } catch {
                    NSLog("🐱 [CodespaceView] Failed to fetch user status: \(error)")
                }
            }

            // Preload repositories in the background for faster UX
            Task {
                do {
                    try await installer.fetchRepositories()
                    NSLog("🐱 [CodespaceView] Successfully preloaded \(installer.repositories.count) repositories")
                } catch {
                    // Silently fail - user will see error if they actually navigate to repo list
                    NSLog("🐱 [CodespaceView] Failed to preload repositories: \(error)")
                }
            }

        }
        .onChange(of: scenePhase) { oldPhase, newPhase in
            handleScenePhaseChange(oldPhase: oldPhase, newPhase: newPhase)
        }
        .onReceive(NotificationCenter.default.publisher(for: .shouldReconnectToCodespace)) { _ in
            // Child views have dismissed - trigger auto-reconnect
            NSLog("🔄 Received reconnection request, will auto-connect when navigateToWorkspaces becomes false")
            shouldAutoReconnect = true
        }
        .onChange(of: navigateToWorkspaces) {
            // Auto-reconnect after returning from workspaces view (triggered by shutdown)
            if !navigateToWorkspaces && shouldAutoReconnect {
                NSLog("🔄 Auto-reconnecting after shutdown detection")
                shouldAutoReconnect = false
                // Trigger reconnection with stored codespace name
                let codespaceName = UserDefaults.standard.string(forKey: "codespace_name")
                handleConnect(codespaceName: codespaceName)
            }
        }
    }

    // Computed properties for dynamic primary button text
    private var primaryButtonText: String {
        if installer.userStatus?.hasAnyCodespaces == false {
            // No codespaces - check if they have repos with Catnip
            if installer.hasRepositoriesWithCatnip {
                return "Launch New Codespace"
            } else {
                return "Install Catnip"
            }
        } else {
            // Has codespaces
            return "Access My Codespace"
        }
    }

    // Computed properties for dynamic status icon and color
    private var statusIcon: String {
        let message = statusMessage.lowercased()

        if message.contains("connected") || message.contains("success") {
            return "checkmark.circle.fill"
        } else if message.contains("waiting") || message.contains("hold") {
            return "clock.fill"
        } else if message.contains("finding") || message.contains("searching") {
            return "magnifyingglass"
        } else if message.contains("connecting") || message.contains("establishing") {
            return "arrow.triangle.2.circlepath"
        } else if message.contains("verifying") || message.contains("authenticating") {
            return "checkmark.shield.fill"
        } else if message.contains("setting up") || message.contains("configuring") {
            return "gearshape.2.fill"
        } else if message.contains("ready") {
            return "sparkles"
        } else {
            return "antenna.radiowaves.left.and.right"
        }
    }

    private var statusColor: Color {
        let message = statusMessage.lowercased()

        if message.contains("connected") || message.contains("success") {
            return .green
        } else if message.contains("waiting") || message.contains("hold") {
            return .orange
        } else if message.contains("ready") {
            return .green
        } else {
            return .accentColor
        }
    }

    private var connectView: some View {
        ScrollView {
            VStack(spacing: 20) {
                // Logo / brand
                Image("logo")
                    .resizable()
                    .scaledToFit()
                    .frame(width: 80, height: 80)
                    .clipShape(RoundedRectangle(cornerRadius: 16))
                    .shadow(color: Color.black.opacity(0.1), radius: 8, x: 0, y: 2)
                    .padding(.top, 40)

                Text("Access your GitHub Codespaces")
                    .font(.title2.weight(.semibold))
                    .multilineTextAlignment(.center)
                    .padding(.bottom, 4)

                VStack(spacing: 16) {
                    Button {
                        // Determine action based on user's codespace and repository status
                        if installer.userStatus?.hasAnyCodespaces == false {
                            // No codespaces - check if they have repos with Catnip
                            if installer.hasRepositoriesWithCatnip {
                                // Has repos with Catnip → Launch New Codespace
                                repositoryListMode = .launch
                            } else {
                                // No repos with Catnip → Install Catnip
                                repositoryListMode = .installation
                            }
                            phase = .repositorySelection
                            Task {
                                do {
                                    try await installer.fetchRepositories()
                                } catch {
                                    errorMessage = "Failed to load repositories: \(error.localizedDescription)"
                                    phase = .connect
                                }
                            }
                        } else {
                            // Has codespaces → Access My Codespace
                            handleConnect()
                        }
                    } label: {
                        HStack {
                            if phase == .connecting {
                                ProgressView()
                                    .progressViewStyle(CircularProgressViewStyle(tint: .white))
                                    .padding(.trailing, 6)
                            }
                            Text(phase == .connecting ? "Connecting…" : primaryButtonText)
                        }
                    }
                    .buttonStyle(ProminentButtonStyle(isDisabled: phase == .connecting))
                    .disabled(phase == .connecting)
                    .accessibilityIdentifier("primaryActionButton")
                }

                // Inline status / error
                if !statusMessage.isEmpty {
                    HStack(spacing: 10) {
                        Image(systemName: statusIcon)
                            .foregroundStyle(statusColor)
                        Text(statusMessage)
                            .font(.subheadline)
                            .foregroundStyle(.primary)
                        Spacer()
                    }
                    .padding(12)
                    .background(statusColor.opacity(0.08))
                    .clipShape(RoundedRectangle(cornerRadius: 10))
                }

                if !errorMessage.isEmpty {
                    HStack(spacing: 10) {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .foregroundStyle(Color.red)
                        Text(errorMessage)
                            .font(.subheadline)
                        Spacer()
                    }
                    .foregroundStyle(Color.red)
                    .padding(12)
                    .background(Color.red.opacity(0.08))
                    .clipShape(RoundedRectangle(cornerRadius: 10))
                }

                Spacer(minLength: 12)

                // Fun fact section
                VStack(spacing: 6) {
                    HStack(spacing: 4) {
                        Text("🐾")
                            .font(.footnote)
                        Text(currentCatFact)
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                    .multilineTextAlignment(.center)
                }
            }
            .padding(.horizontal, 20)
        }
        .scrollBounceBehavior(.basedOnSize)
        .background(Color(uiColor: .systemGroupedBackground))
        .onAppear {
            if currentCatFact.isEmpty {
                currentCatFact = catFacts.randomElement() ?? catFacts[0]
            }
        }
    }

    private var setupView: some View {
        Form {
            Section {
                Label("Setup Required", systemImage: "wrench.and.screwdriver")
                    .font(.headline)
            }

            Section("Automatic Setup") {
                Text("Let Catnip automatically add the feature to one of your repositories.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)

                Button {
                    repositoryListMode = .installation
                    phase = .repositorySelection
                    Task {
                        do {
                            try await installer.fetchRepositories()
                        } catch {
                            errorMessage = "Failed to load repositories: \(error.localizedDescription)"
                            phase = .setup
                        }
                    }
                } label: {
                    HStack {
                        Image(systemName: "wand.and.stars")
                        Text("Automatic Setup")
                    }
                }
                .buttonStyle(ProminentButtonStyle(isDisabled: false))
            }

            Section("Manual Setup") {
                Text("Add the feature to **.devcontainer/devcontainer.json**:")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)

                Markdown("""
                ```json
                {
                  "features": {
                    "ghcr.io/wandb/catnip/feature:1": {}
                  },
                  "forwardPorts": [6369]
                }
                ```
                """)
                .padding(8)
                .background(Color(uiColor: .secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 8))

                Text("Create a new Codespace and return here to connect.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
        }
        .scrollContentBackground(.hidden)
        .background(Color(uiColor: .systemGroupedBackground))
    }

    private var selectionView: some View {
        List {
            Section("Select Codespace") {
                ForEach(codespaces) { codespace in
                    Button {
                        handleConnect(codespaceName: codespace.name)
                    } label: {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(codespace.displayName)
                                .font(.body.weight(.semibold))
                                .foregroundStyle(.primary)
                            if let repo = codespace.repository {
                                Text(repo)
                                    .font(.subheadline)
                                    .foregroundStyle(.gray)
                            }
                            Text("Last used: \(codespace.lastUsedDate, style: .date)")
                                .font(.caption)
                                .foregroundStyle(.gray)
                        }
                    }
                }
            }

            Section {
                Button {
                    repositoryListMode = .launch
                    phase = .repositorySelection
                    Task {
                        do {
                            try await installer.fetchRepositories()
                        } catch {
                            errorMessage = "Failed to load repositories: \(error.localizedDescription)"
                            phase = .selection
                        }
                    }
                } label: {
                    HStack {
                        Image(systemName: "plus.circle.fill")
                        Text("Launch New Codespace")
                    }
                }
                .frame(maxWidth: .infinity, alignment: .center)
            }
        }
        .listStyle(.insetGrouped)
        .scrollContentBackground(.hidden)
        .background(Color(uiColor: .systemGroupedBackground))
    }


    private func handleConnect(codespaceName: String? = nil) {
        phase = .connecting
        errorMessage = ""
        statusMessage = ""
        statusMessage = "Finding your codespace..."

        // Store codespace name for potential reconnection after backgrounding
        pendingCodespaceName = codespaceName

        // Mock connection for UI tests
        if UITestingHelper.isUITesting {
            UserDefaults.standard.set("mock-codespace", forKey: "codespace_name")
            phase = .connect
            statusMessage = "Connected."
            navigateToWorkspaces = true
            return
        }

        // Mock connection for preview mode
        if authManager.isPreviewMode {
            UserDefaults.standard.set("preview-codespace", forKey: "codespace_name")
            phase = .connect
            statusMessage = "Connected."
            navigateToWorkspaces = true
            return
        }

        // Save codespace name immediately when selected (non-sensitive app state)
        if let codespaceName = codespaceName, !codespaceName.isEmpty {
            UserDefaults.standard.set(codespaceName, forKey: "codespace_name")
        }

        Task {
            do {
                let token = try await KeychainHelper.load(key: "session_token")
                let headers = [
                    "Content-Type": "application/json",
                    "Authorization": "Bearer \(token)"
                ]

                let service = SSEService()
                sseService = service

                service.connect(codespaceName: codespaceName, org: nil, headers: headers) { event in
                    Task { @MainActor in
                        handleSSEEvent(event)
                    }
                }
            } catch {
                await MainActor.run {
                    errorMessage = "Failed to connect: \(error.localizedDescription)"
                    phase = .error
                }
            }
        }
    }

    @MainActor
    private func handleSSEEvent(_ event: SSEEvent) {
        switch event {
        case .status(let message):
            statusMessage = message

        case .success(let message, let codespaceUrl):
            errorMessage = ""
            statusMessage = message.isEmpty ? "Connected." : message

            // Extract and store codespace name if not already set
            if let urlString = codespaceUrl,
               let url = URL(string: urlString),
               let host = url.host {
                // Extract from format: codespace-name-6369.app.github.dev
                // The regex pattern matches everything before "-6369.app.github.dev"
                let pattern = #"-6369\.app\.github\.dev$"#
                if let range = host.range(of: pattern, options: .regularExpression) {
                    let codespaceName = String(host[..<range.lowerBound])
                    UserDefaults.standard.set(codespaceName, forKey: "codespace_name")
                }
            }

            sseService?.disconnect()
            sseService = nil
            // Clear background reconnection flag since we successfully connected
            wasConnectingBeforeBackground = false
            // Keep phase as .connecting until after navigation to maintain loading state

            // Navigate to workspaces after a short delay
            DispatchQueue.main.asyncAfter(deadline: .now() + 1.0) {
                navigateToWorkspaces = true
            }

        case .error(let message):
            statusMessage = ""
            errorMessage = message
            phase = .error
            sseService?.disconnect()
            sseService = nil
            // Clear background reconnection flag since we hit an error
            wasConnectingBeforeBackground = false

        case .setup(let message, let nextAction):
            statusMessage = ""
            errorMessage = message
            sseService?.disconnect()
            sseService = nil
            // Clear background reconnection flag since we're moving to setup flow
            wasConnectingBeforeBackground = false

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

        case .multiple(let foundCodespaces):
            codespaces = foundCodespaces
            phase = .selection
            sseService?.disconnect()
            sseService = nil
            // Clear background reconnection flag since we're moving to selection flow
            wasConnectingBeforeBackground = false
        }
    }

    // MARK: - Repository Selection View

    private var repositorySelectionView: some View {
        List {
            Section {
                // Only show loading spinner if we have no cached data
                if installer.isLoading && installer.repositories.isEmpty {
                    HStack {
                        Spacer()
                        ProgressView()
                        Spacer()
                    }
                    .padding()
                } else if filteredRepositories.isEmpty {
                    Text(repositoryListMode == .launch ? "No repositories with Catnip found" : "No repositories found")
                        .foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, alignment: .center)
                        .padding()
                } else {
                    ForEach(filteredRepositories, id: \.id) { repo in
                        Button {
                            if repositoryListMode == .launch {
                                handleLaunchCodespace(repository: repo)
                            } else {
                                handleInstallCatnip(repository: repo)
                            }
                        } label: {
                            HStack(spacing: 12) {
                                Image(systemName: repo.statusIcon)
                                    .foregroundStyle(repo.statusColor)
                                    .frame(width: 24)

                                VStack(alignment: .leading, spacing: 4) {
                                    Text(repo.displayName)
                                        .font(.body.weight(.medium))
                                        .foregroundStyle(.primary)

                                    Text(repo.statusText)
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }

                                Spacer()

                                if repositoryListMode == .launch {
                                    Image(systemName: "arrow.right.circle")
                                        .foregroundStyle(Color.accentColor)
                                }
                            }
                            .padding(.vertical, 4)
                        }
                    }
                }
            } header: {
                Text(repositoryListMode == .launch ? "Select Repository to Launch" : "Select a Repository")
            } footer: {
                if !filteredRepositories.isEmpty {
                    Text(repositoryListMode == .launch
                         ? "Choose a repository to create a new codespace from the main branch."
                         : "Choose a repository to add the Catnip feature. A pull request will be created for your review.")
                        .font(.footnote)
                }
            }

            // Show toggle button to switch between launch and install modes
            Section {
                if repositoryListMode == .launch {
                    Button {
                        repositoryListMode = .installation
                    } label: {
                        HStack {
                            Image(systemName: "plus.rectangle.on.folder")
                            Text("Install Catnip in Another Repository")
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .center)
                } else {
                    // Show launch button only if there are repos with Catnip
                    if installer.repositories.contains(where: { $0.hasCatnipFeature }) {
                        Button {
                            repositoryListMode = .launch
                        } label: {
                            HStack {
                                Image(systemName: "arrow.right.circle.fill")
                                Text("Launch Codespace")
                            }
                        }
                        .frame(maxWidth: .infinity, alignment: .center)
                    }
                }
            }
        }
        .listStyle(.insetGrouped)
        .scrollContentBackground(.hidden)
        .background(Color(uiColor: .systemGroupedBackground))
        .refreshable {
            // Pull-to-refresh: force a fresh fetch
            do {
                try await installer.fetchRepositories(forceRefresh: true)
            } catch {
                // Error is already set in installer.error
                NSLog("🐱 [CodespaceView] Failed to refresh repositories: \(error)")
            }
        }
    }

    // Filter repositories based on mode
    private var filteredRepositories: [Repository] {
        var filtered = installer.repositories

        // Filter by Catnip feature based on mode
        switch repositoryListMode {
        case .installation:
            // Show repos without Catnip feature
            filtered = filtered.filter { !$0.hasCatnipFeature }
            NSLog("🐱 [CodespaceView] Installation mode: \(filtered.count) repos without Catnip")
        case .launch:
            // Show repos with Catnip feature
            filtered = filtered.filter { $0.hasCatnipFeature }
            NSLog("🐱 [CodespaceView] Launch mode: \(filtered.count) repos with Catnip")
        }

        return filtered
    }

    private func handleInstallCatnip(repository: Repository) {
        phase = .installing
        Task {
            do {
                let result = try await installer.installCatnip(
                    repository: repository.fullName,
                    startCodespace: false
                )
                await MainActor.run {
                    installationResult = result
                    // Optimistically mark repository as having Catnip
                    // (Assumes user will merge the PR or wants to test the branch)
                    installer.markRepositoryAsHavingCatnip(repository.fullName)
                    NSLog("🐱 [CodespaceView] Installation complete for \(repository.fullName)")
                }
            } catch {
                await MainActor.run {
                    errorMessage = "Installation failed: \(error.localizedDescription)"
                    phase = .repositorySelection
                }
            }
        }
    }

    private func handleLaunchCodespace(repository: Repository) {
        phase = .creatingCodespace
        pendingRepository = repository.fullName

        Task {
            // Request notification permission before creating
            let permissionGranted = await NotificationManager.shared.requestPermission()
            if permissionGranted {
                NSLog("🔔 Notification permission granted for codespace creation")
            } else {
                NSLog("🔔 ⚠️ Notification permission denied, but continuing with creation")
            }

            do {
                // Start tracking BEFORE creation begins
                await MainActor.run {
                    tracker.startCreation(repositoryName: repository.fullName, codespaceName: nil)
                }

                let codespace = try await installer.createCodespace(
                    repository: repository.fullName,
                    branch: nil  // Use default branch
                )
                await MainActor.run {
                    createdCodespace = codespace
                    // Store the codespace name for future connections
                    UserDefaults.standard.set(codespace.name, forKey: "codespace_name")

                    // Update tracker with codespace name
                    tracker.updateCodespaceName(codespace.name)

                    NSLog("🐱 [CodespaceView] Codespace ready (Available or has credentials): \(codespace.name)")
                    NSLog("🐱 [CodespaceView] tracker.completeCreation() was already called by pollCodespaceStatus")
                    NSLog("🐱 [CodespaceView] Transitioning to connect screen to show SSE connection flow...")

                    // Transition to connect screen - the connect view will automatically
                    // call handleConnect() when it appears, showing the user the SSE status updates
                    phase = .connect
                }
            } catch {
                // Error is already set in installer.error by createCodespace
                // Stay on .creatingCodespace phase to show error screen with back button
                NSLog("🐱 [CodespaceView] Failed to launch codespace: \(error)")

                // Notify tracker of failure
                await MainActor.run {
                    tracker.failCreation(error: error.localizedDescription)
                }
            }
        }
    }

    // MARK: - Installing View

    private var installingView: some View {
        VStack(spacing: 24) {
            Spacer()

            // Progress animation
            if installer.currentStep != .complete {
                ProgressView()
                    .progressViewStyle(CircularProgressViewStyle(tint: .accentColor))
                    .scaleEffect(1.5)
            } else {
                Image(systemName: "checkmark.circle.fill")
                    .font(.system(size: 60))
                    .foregroundStyle(.green)
            }

            VStack(spacing: 8) {
                Text(installer.currentStep.description)
                    .font(.title3.weight(.semibold))
                    .multilineTextAlignment(.center)

                if let error = installer.error {
                    Text(error)
                        .font(.subheadline)
                        .foregroundStyle(.red)
                        .multilineTextAlignment(.center)
                        .padding(.horizontal)
                }
            }

            Spacer()

            // Show actions when complete
            if installer.currentStep == .complete, let result = installationResult {
                VStack(spacing: 12) {
                    // Start Codespace & Test button - most prominent
                    if let branch = result.branch {
                        Button {
                            handleStartCodespace(repository: result.repository ?? "", branch: branch)
                        } label: {
                            HStack {
                                Image(systemName: "terminal.fill")
                                Text("Start Codespace & Test")
                            }
                        }
                        .buttonStyle(ProminentButtonStyle(isDisabled: false))
                    }

                    // View PR button
                    if let prUrl = result.prUrl {
                        Button {
                            if let url = URL(string: prUrl) {
                                UIApplication.shared.open(url)
                            }
                        } label: {
                            HStack {
                                Image(systemName: "arrow.up.doc")
                                Text("View Pull Request")
                            }
                        }
                        .buttonStyle(SecondaryButtonStyle(isDisabled: false))
                    }

                    // Done button
                    Button("Done") {
                        phase = .connect
                        installer.reset()
                        installationResult = nil
                    }
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                }
                .padding(.horizontal, 20)
            }
        }
        .padding()
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Color(uiColor: .systemGroupedBackground))
    }

    // MARK: - Codespace Creation Handlers

    private func handleStartCodespace(repository: String, branch: String) {
        phase = .creatingCodespace
        pendingRepository = repository

        Task {
            // Request notification permission before creating
            let permissionGranted = await NotificationManager.shared.requestPermission()
            if permissionGranted {
                NSLog("🔔 Notification permission granted for codespace creation")
            } else {
                NSLog("🔔 ⚠️ Notification permission denied, but continuing with creation")
            }

            do {
                // Start tracking BEFORE creation begins
                await MainActor.run {
                    tracker.startCreation(repositoryName: repository, codespaceName: nil)
                }

                let codespace = try await installer.createCodespace(
                    repository: repository,
                    branch: branch
                )
                await MainActor.run {
                    createdCodespace = codespace
                    // Store the codespace name for future connections
                    UserDefaults.standard.set(codespace.name, forKey: "codespace_name")

                    // Update tracker with codespace name
                    tracker.updateCodespaceName(codespace.name)

                    NSLog("🐱 [CodespaceView] Codespace created: \(codespace.name), triggering SSE connection flow")

                    // Trigger SSE flow to handle startup, health check, etc.
                    // This leverages the existing robust connection logic
                    handleConnect(codespaceName: codespace.name)
                }
            } catch {
                // Error is already set in installer.error by createCodespace
                // Stay on .creatingCodespace phase to show error screen with back button
                NSLog("🐱 [CodespaceView] Failed to start codespace after install: \(error)")

                // Notify tracker of failure
                await MainActor.run {
                    tracker.failCreation(error: error.localizedDescription)
                }
            }
        }
    }

    // MARK: - Creating Codespace View

    private var creatingCodespaceView: some View {
        VStack(spacing: 24) {
            Spacer()

            // Progress animation
            if installer.currentStep != .complete && installer.error == nil {
                ProgressView()
                    .progressViewStyle(CircularProgressViewStyle(tint: .accentColor))
                    .scaleEffect(1.5)
            } else if installer.error != nil {
                Image(systemName: "exclamationmark.triangle.fill")
                    .font(.system(size: 60))
                    .foregroundStyle(.orange)
            } else {
                Image(systemName: "checkmark.circle.fill")
                    .font(.system(size: 60))
                    .foregroundStyle(.green)
            }

            VStack(spacing: 8) {
                Text(installer.error == nil ? installer.currentStep.description : "Codespace Creation")
                    .font(.title3.weight(.semibold))
                    .multilineTextAlignment(.center)

                if let error = installer.error {
                    Text(error)
                        .font(.subheadline)
                        .foregroundStyle(.primary)
                        .multilineTextAlignment(.center)
                        .padding(.horizontal)
                        .padding(.top, 4)
                } else if installer.currentStep == .creatingCodespace {
                    VStack(spacing: 8) {
                        if let repo = pendingRepository {
                            Text("Creating codespace in \(repo)")
                                .font(.subheadline)
                                .foregroundStyle(.secondary)
                                .multilineTextAlignment(.center)
                        } else {
                            Text("Creating your codespace...")
                                .font(.subheadline)
                                .foregroundStyle(.secondary)
                                .multilineTextAlignment(.center)
                        }

                        // Show progress from tracker if available
                        if tracker.isCreating && tracker.progress > 0 {
                            VStack(spacing: 4) {
                                ProgressView(value: tracker.progress)
                                    .progressViewStyle(LinearProgressViewStyle())
                                    .padding(.horizontal, 40)

                                Text("\(Int(tracker.progress * 100))% complete")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                } else if installer.currentStep == .waitingForCodespace {
                    VStack(spacing: 4) {
                        Text("Building and starting your codespace")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                            .multilineTextAlignment(.center)
                        Text("This may take up to 10 minutes on first launch")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .multilineTextAlignment(.center)

                        // Show progress from tracker if available
                        if tracker.isCreating && tracker.progress > 0 {
                            VStack(spacing: 4) {
                                ProgressView(value: tracker.progress)
                                    .progressViewStyle(LinearProgressViewStyle())
                                    .padding(.horizontal, 40)
                                    .padding(.top, 8)

                                Text("\(Int(tracker.progress * 100))% complete")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }

            // Notification info (show when creating, not on error or complete)
            if installer.error == nil && installer.currentStep != .complete {
                VStack(spacing: 8) {
                    Image(systemName: "bell.badge.fill")
                        .font(.title3)
                        .foregroundStyle(Color.accentColor)

                    Text("We'll notify you when it's ready")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)

                    Text("Feel free to navigate away and explore the app")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)
                }
                .padding()
                .background(Color(uiColor: .secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 12))
                .padding(.horizontal, 20)
            }

            Spacer()

            // Show error hint if there's an error
            if installer.error != nil {
                Text("You can try connecting again after a few minutes")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal, 20)
            }
        }
        .padding()
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Color(uiColor: .systemGroupedBackground))
    }

    // MARK: - App Lifecycle Handling

    private func handleScenePhaseChange(oldPhase: ScenePhase, newPhase: ScenePhase) {
        NSLog("🐱 [CodespaceView] ScenePhase: \(oldPhase) → \(newPhase), phase: \(phase), wasConnecting: \(wasConnectingBeforeBackground)")

        // Track when app goes to background during SSE connection and disconnect stale connection
        if newPhase == .background && phase == .connecting {
            wasConnectingBeforeBackground = true
            NSLog("🐱 [CodespaceView] App backgrounded during SSE connection, disconnecting and will reconnect on foreground")

            // Explicitly disconnect to avoid zombie connections
            sseService?.disconnect()
            sseService = nil
        }

        // Reconnect when app returns to active if we were connecting
        // Note: Don't check oldPhase == .background because iOS transitions through .inactive
        // (background → inactive → active), so oldPhase will be .inactive, not .background
        if newPhase == .active && wasConnectingBeforeBackground {
            NSLog("🐱 [CodespaceView] App foregrounded after backgrounding during connection, reconnecting SSE...")

            // Ensure old connection is fully cleaned up
            sseService?.disconnect()
            sseService = nil

            // Restart the connection with the same codespace name
            handleConnect(codespaceName: pendingCodespaceName)

            // Reset the flag
            wasConnectingBeforeBackground = false
        }
    }

    private var createRepositoryView: some View {
        ScrollView {
            VStack(spacing: 24) {
                Spacer()

                // Welcoming icon
                Image(systemName: "plus.rectangle.on.folder")
                    .font(.system(size: 60))
                    .foregroundStyle(Color.accentColor)

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
}

// MARK: - Previews

#if DEBUG
#Preview("Connect Screen") {
    NavigationStack {
        CodespaceView()
            .environmentObject(MockAuthManager() as AuthManager)
            .toolbar(.visible)
    }
}

#Preview("Selection Screen") {
    NavigationStack {
        CodespaceSelectionPreview()
    }
}

#Preview("Setup Screen") {
    NavigationStack {
        CodespaceSetupPreview()
    }
}

// Preview for selection state
private struct CodespaceSelectionPreview: View {
    var body: some View {
        List {
            Section("Select Codespace") {
                ForEach(CodespaceInfo.previewList) { codespace in
                    Button(action: {}) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(codespace.displayName).font(.body.weight(.semibold))
                            if let repo = codespace.repository {
                                Text(repo).font(.subheadline).foregroundStyle(.secondary)
                            }
                            Text("Last used: \(codespace.lastUsedDate, style: .date)").font(.caption).foregroundStyle(.tertiary)
                        }
                    }
                }
            }
            Section { Button("Back", action: {}) }
        }
        .listStyle(.insetGrouped)
    }
}

// Preview for setup state
private struct CodespaceSetupPreview: View {
    var body: some View {
        Form {
            Section {
                Label("Setup Required", systemImage: "wrench.and.screwdriver").font(.headline)
            }
            Section("Enable Catnip in your Codespace") {
                Text("Add the feature to **.devcontainer/devcontainer.json**:")
                Text(#"""
                "features": {
                  "ghcr.io/wandb/catnip/feature:1": {}
                }
                """#)
                .font(.system(.body, design: .monospaced))
                .padding(8)
                .background(Color(uiColor: .secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 8))
                Text("Create a new Codespace and return here to connect.")
            }
            Section { Button("Back", action: {}) }
        }
    }
}
#endif
