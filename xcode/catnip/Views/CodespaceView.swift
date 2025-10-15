//
//  CodespaceView.swift
//  catnip
//
//  Codespace connection screen with SSE support
//

import SwiftUI

enum CodespacePhase {
    case connect
    case connecting
    case setup
    case selection
    case repositorySelection
    case installing
    case error
}

struct CodespaceView: View {
    @EnvironmentObject var authManager: AuthManager
    @StateObject private var installer = CatnipInstaller.shared
    @State private var phase: CodespacePhase = .connect
    @State private var orgName: String = ""
    @State private var statusMessage: String = ""
    @State private var errorMessage: String = ""
    @State private var codespaces: [CodespaceInfo] = []
    @State private var sseService: SSEService?
    @State private var navigateToWorkspaces = false
    @State private var currentCatFact: String = ""
    @State private var installationResult: InstallationResult?

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
            if phase == .setup {
                setupView
            } else if phase == .selection {
                selectionView
            } else if phase == .repositorySelection {
                repositorySelectionView
            } else if phase == .installing {
                installingView
            } else {
                connectView
            }
        }
        .navigationTitle("Catnip")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .navigationBarTrailing) {
                Button("Logout") {
                    Task { await authManager.logout() }
                }
                .disabled(phase == .connecting)
                .accessibilityIdentifier("logoutButton")
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
            }
        }
        .task {
            // Auto-navigate to workspaces in UI testing mode
            if UITestingHelper.shouldAutoNavigateToWorkspaces() {
                UserDefaults.standard.set("mock-codespace", forKey: "codespace_name")
                await MainActor.run {
                    navigateToWorkspaces = true
                }
            }
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

                if !orgName.isEmpty {
                    Text("Organization: \(orgName)")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }

                VStack(spacing: 16) {
                    Button {
                        handleConnect()
                    } label: {
                        HStack {
                            if phase == .connecting {
                                ProgressView()
                                    .progressViewStyle(CircularProgressViewStyle(tint: .white))
                                    .padding(.trailing, 6)
                            }
                            Text(phase == .connecting ? "Connecting…" : "Access My Codespace")
                        }
                    }
                    .buttonStyle(ProminentButtonStyle(isDisabled: phase == .connecting))
                    .disabled(phase == .connecting)

                    HStack(spacing: 10) {
                        TextField("Organization (e.g., wandb)", text: $orgName)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled(true)
                            .textFieldStyle(.plain)
                            .padding(.horizontal, 14)
                            .padding(.vertical, 12)
                            .background(
                                RoundedRectangle(cornerRadius: 10)
                                    .strokeBorder(Color.gray.opacity(0.3), lineWidth: 1.5)
                            )
                            .submitLabel(.go)
                            .onSubmit { handleConnect(org: orgName) }
                            .accessibilityIdentifier("organizationTextField")

                        Button("Go") {
                            handleConnect(org: orgName)
                        }
                        .buttonStyle(SecondaryButtonStyle(isDisabled: orgName.isEmpty || phase == .connecting))
                        .disabled(orgName.isEmpty || phase == .connecting)
                        .accessibilityIdentifier("goButton")
                    }
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

                // Add to new repository button
                Button {
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
                    HStack(spacing: 8) {
                        Image(systemName: "plus.rectangle.on.folder")
                        Text("Setup Catnip in Another Repository")
                    }
                    .font(.subheadline)
                }
                .buttonStyle(SecondaryButtonStyle(isDisabled: false))
                .padding(.horizontal, 20)

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

            Section {
                Button("Back") { phase = .connect }
                    .frame(maxWidth: .infinity, alignment: .center)
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
                        handleConnect(codespaceName: codespace.name, org: orgName.isEmpty ? nil : orgName)
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
                Button("Back") { phase = .connect }
                    .frame(maxWidth: .infinity, alignment: .center)
            }
        }
        .listStyle(.insetGrouped)
        .scrollContentBackground(.hidden)
        .background(Color(uiColor: .systemGroupedBackground))
    }

    private func handleConnect(codespaceName: String? = nil, org: String? = nil) {
        phase = .connecting
        errorMessage = ""
        statusMessage = ""
        statusMessage = "Finding your codespace..."

        // Mock connection for UI tests
        if UITestingHelper.isUITesting {
            UserDefaults.standard.set("mock-codespace", forKey: "codespace_name")
            phase = .connect
            statusMessage = "Connected."
            navigateToWorkspaces = true
            return
        }

        // Save codespace name and org immediately when selected (non-sensitive app state)
        if let codespaceName = codespaceName, !codespaceName.isEmpty {
            UserDefaults.standard.set(codespaceName, forKey: "codespace_name")
        }

        if let org = org, !org.isEmpty {
            UserDefaults.standard.set(org, forKey: "org_name")
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

                service.connect(codespaceName: codespaceName, org: org, headers: headers) { event in
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

        case .setup(let message):
            statusMessage = ""
            errorMessage = message
            phase = .setup
            sseService?.disconnect()
            sseService = nil

        case .multiple(let foundCodespaces):
            codespaces = foundCodespaces
            phase = .selection
            sseService?.disconnect()
            sseService = nil
        }
    }

    // MARK: - Repository Selection View

    private var repositorySelectionView: some View {
        List {
            Section {
                if installer.isLoading {
                    HStack {
                        Spacer()
                        ProgressView()
                        Spacer()
                    }
                    .padding()
                } else if installer.repositories.isEmpty {
                    Text("No repositories found")
                        .foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, alignment: .center)
                        .padding()
                } else {
                    ForEach(installer.repositories) { repo in
                        Button {
                            handleInstallCatnip(repository: repo)
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

                                if repo.hasCatnipFeature {
                                    Text("Installed")
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }
                            }
                            .padding(.vertical, 4)
                        }
                        .disabled(repo.hasCatnipFeature)
                    }
                }
            } header: {
                Text("Select a Repository")
            } footer: {
                if !installer.repositories.isEmpty {
                    Text("Choose a repository to add the Catnip feature. A pull request will be created for your review.")
                        .font(.footnote)
                }
            }

            Section {
                Button("Back") {
                    phase = .connect
                    installer.reset()
                }
                .frame(maxWidth: .infinity, alignment: .center)
            }
        }
        .listStyle(.insetGrouped)
        .scrollContentBackground(.hidden)
        .background(Color(uiColor: .systemGroupedBackground))
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
                }
            } catch {
                await MainActor.run {
                    errorMessage = "Installation failed: \(error.localizedDescription)"
                    phase = .repositorySelection
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
                        .buttonStyle(ProminentButtonStyle(isDisabled: false))
                    }

                    Button("Done") {
                        phase = .connect
                        installer.reset()
                        installationResult = nil
                    }
                    .buttonStyle(SecondaryButtonStyle(isDisabled: false))
                }
                .padding(.horizontal, 20)
            }
        }
        .padding()
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Color(uiColor: .systemGroupedBackground))
    }
}

// MARK: - Previews

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
