//
//  ClaudeAuthSheet.swift
//  catnip
//
//  Claude Code authentication sheet with automated onboarding flow
//

import SwiftUI

struct ClaudeAuthSheet: View {
    @Binding var isPresented: Bool
    let codespaceName: String
    let onAuthComplete: () -> Void

    @State private var status: ClaudeOnboardingStatus?
    @State private var loading = false
    @State private var polling = false
    @State private var code = ""
    @State private var submittingCode = false
    @State private var hasClickedOAuthLink = false
    @State private var hasSubmittedCode = false
    @State private var pollingTimer: Timer?

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 20) {
                    // Header
                    VStack(spacing: 8) {
                        Text("Login to Claude")
                            .font(.title2.weight(.bold))

                        Text("Connect your Claude account to start vibing.")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                            .multilineTextAlignment(.center)
                    }
                    .padding(.top, 20)

                    // Content based on state
                    if isProcessing && !isWaitingForAuth {
                        processingView
                    } else if isWaitingForAuth, let oauthUrl = status?.oauthUrl {
                        authWaitingView(oauthUrl: oauthUrl)
                    } else if status?.parsedState == .complete {
                        successView
                    } else if isEffectiveErrorState {
                        errorView
                    }

                    Spacer()
                }
                .padding(.horizontal, 20)
            }
            .scrollBounceBehavior(.basedOnSize)
            .background(Color(uiColor: .systemGroupedBackground))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .navigationBarLeading) {
                    if !isProcessing && status?.parsedState != .complete {
                        Button("Cancel") {
                            handleClose(dismissed: true)
                        }
                    }
                }
            }
        }
        .interactiveDismissDisabled(isProcessing || status?.parsedState == .complete)
        .onAppear {
            if status == nil {
                Task {
                    await startOnboarding()
                }
            }
        }
        .onDisappear {
            stopPolling()
        }
    }

    // MARK: - View Components

    private var processingView: some View {
        VStack(spacing: 16) {
            ProgressView()
                .scaleEffect(1.2)
                .tint(.accentColor)

            Text(status?.message ?? "Setting up...")
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 40)
    }

    private func authWaitingView(oauthUrl: String) -> some View {
        VStack(spacing: 16) {
            // Warning message if present
            if let errorMessage = status?.errorMessage, !errorMessage.isEmpty {
                HStack(spacing: 10) {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .foregroundStyle(.orange)
                    Text(errorMessage)
                        .font(.subheadline)
                        .foregroundStyle(.primary)
                    Spacer()
                }
                .padding(12)
                .background(Color.orange.opacity(0.1))
                .clipShape(RoundedRectangle(cornerRadius: 10))
            }

            // OAuth button or confirmation
            if !hasClickedOAuthLink {
                Button {
                    openOAuthURL(oauthUrl)
                } label: {
                    HStack {
                        Image(systemName: "arrow.up.forward.square")
                        Text("Open Login Page")
                    }
                }
                .buttonStyle(ProminentButtonStyle())
            } else {
                HStack(spacing: 10) {
                    Image(systemName: "checkmark.circle.fill")
                        .foregroundStyle(.green)
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Login page opened")
                            .font(.subheadline.weight(.medium))
                        Text("Complete authentication and paste your code below.")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                }
                .padding(12)
                .background(Color.green.opacity(0.1))
                .clipShape(RoundedRectangle(cornerRadius: 10))
            }

            // Code input field (shown after OAuth link clicked)
            if hasClickedOAuthLink {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Authentication code:")
                        .font(.subheadline.weight(.medium))

                    TextField("Enter authentication code", text: $code)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled(true)
                        .textFieldStyle(.plain)
                        .padding(.horizontal, 14)
                        .padding(.vertical, 12)
                        .background(
                            RoundedRectangle(cornerRadius: 10)
                                .strokeBorder(Color.gray.opacity(0.3), lineWidth: 1.5)
                                .background(Color(uiColor: .systemBackground))
                        )
                        .clipShape(RoundedRectangle(cornerRadius: 10))
                        .disabled(submittingCode)
                        .onSubmit {
                            if !submittingCode && !code.trimmingCharacters(in: .whitespaces).isEmpty {
                                Task { await submitCode() }
                            }
                        }

                    Button {
                        Task { await submitCode() }
                    } label: {
                        HStack {
                            if submittingCode {
                                ProgressView()
                                    .progressViewStyle(CircularProgressViewStyle(tint: .white))
                                    .padding(.trailing, 6)
                            }
                            Text(submittingCode ? "Submitting..." : "Submit Code")
                        }
                    }
                    .buttonStyle(ProminentButtonStyle(isDisabled: code.trimmingCharacters(in: .whitespaces).isEmpty || submittingCode))
                    .disabled(code.trimmingCharacters(in: .whitespaces).isEmpty || submittingCode)
                }
            }
        }
    }

    private var successView: some View {
        VStack(spacing: 16) {
            HStack(spacing: 10) {
                Image(systemName: "checkmark.circle.fill")
                    .font(.title3)
                    .foregroundStyle(.green)
                Text("Connected successfully!")
                    .font(.subheadline)
                    .foregroundStyle(.primary)
                Spacer()
            }
            .padding(12)
            .background(Color.green.opacity(0.1))
            .clipShape(RoundedRectangle(cornerRadius: 10))

            Button("Close") {
                handleSuccessClose()
            }
            .buttonStyle(ProminentButtonStyle())
        }
    }

    private var errorView: some View {
        VStack(spacing: 16) {
            HStack(spacing: 10) {
                Image(systemName: "xmark.circle.fill")
                    .font(.title3)
                    .foregroundStyle(.red)
                VStack(alignment: .leading, spacing: 4) {
                    Text(status?.errorMessage ?? "Authentication failed")
                        .font(.subheadline)
                        .foregroundStyle(.primary)
                }
                Spacer()
            }
            .padding(12)
            .background(Color.red.opacity(0.1))
            .clipShape(RoundedRectangle(cornerRadius: 10))

            // Manual authentication instructions if error suggests it
            if status?.errorMessage?.contains("run 'claude' directly") == true {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Manual Authentication:")
                        .font(.subheadline.weight(.medium))

                    VStack(alignment: .leading, spacing: 6) {
                        Text("1. Open a terminal in your project directory")
                        Text("2. Run: ") + Text("claude").font(.system(.body, design: .monospaced)).foregroundStyle(.primary)
                        Text("3. Follow the authentication prompts")
                        Text("4. Reload this page once authenticated")
                    }
                    .font(.caption)
                    .foregroundStyle(.secondary)
                }
                .padding(12)
                .background(Color(uiColor: .secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 10))
            }

            HStack(spacing: 12) {
                Button("Try Again") {
                    Task { await startOnboarding() }
                }
                .buttonStyle(SecondaryButtonStyle())
                .frame(maxWidth: .infinity)

                Button("Cancel") {
                    handleClose(dismissed: true)
                }
                .buttonStyle(SecondaryButtonStyle())
                .frame(maxWidth: .infinity)
            }
        }
    }

    // MARK: - Computed Properties

    private var hasCriticalError: Bool {
        status?.errorMessage?.contains("Connection to authentication process lost") ?? false
    }

    private var isEffectiveErrorState: Bool {
        status?.parsedState == .error || hasCriticalError
    }

    private var isWaitingForAuth: Bool {
        let state = status?.parsedState
        return (state == .authWaiting || state == .authUrl) && !hasCriticalError
    }

    private var isProcessing: Bool {
        if polling || loading {
            return true
        }

        guard let state = status?.parsedState else {
            return false
        }

        // Processing if we're in any non-terminal, non-waiting state
        return state != .idle && state != .authWaiting && state != .complete && state != .error && !hasCriticalError
    }

    // MARK: - Actions

    private func startOnboarding() async {
        await MainActor.run {
            loading = true
            status = nil
            code = ""
            hasSubmittedCode = false
            hasClickedOAuthLink = false
        }

        do {
            let result = try await CatnipAPI.shared.startClaudeOnboarding()

            await MainActor.run {
                loading = false
            }

            // Check if onboarding was resumed
            if result.status == "resumed" {
                NSLog("🐱 [ClaudeAuth] Onboarding already in progress, resuming polling...")

                // Get current status
                if let currentStatus = try? await CatnipAPI.shared.getClaudeOnboardingStatus() {
                    await MainActor.run {
                        status = currentStatus
                    }
                }

                // Resume polling
                await MainActor.run {
                    startPolling()
                }
                return
            }

            // Start polling for new onboarding
            await MainActor.run {
                startPolling()
            }
        } catch {
            await MainActor.run {
                loading = false
                status = ClaudeOnboardingStatus(
                    state: "error",
                    oauthUrl: nil,
                    message: nil,
                    errorMessage: "Failed to start onboarding: \(error.localizedDescription)"
                )
            }
        }
    }

    private func openOAuthURL(_ urlString: String) {
        if let url = URL(string: urlString) {
            UIApplication.shared.open(url)
            hasClickedOAuthLink = true
        }
    }

    private func submitCode() async {
        let trimmedCode = code.trimmingCharacters(in: .whitespaces)
        guard !trimmedCode.isEmpty else {
            return
        }

        NSLog("🐱 [ClaudeAuth] Submitting code: \(trimmedCode)")

        await MainActor.run {
            submittingCode = true
            hasSubmittedCode = true
        }

        do {
            try await CatnipAPI.shared.submitClaudeOnboardingCode(trimmedCode)

            await MainActor.run {
                submittingCode = false
                // Resume polling to wait for completion
                startPolling()
            }
        } catch {
            NSLog("🐱 [ClaudeAuth] Submit code error: \(error)")

            await MainActor.run {
                submittingCode = false
                hasSubmittedCode = false // Reset on error so user can retry
                status = ClaudeOnboardingStatus(
                    state: "error",
                    oauthUrl: status?.oauthUrl,
                    message: nil,
                    errorMessage: "Failed to submit authentication code: \(error.localizedDescription)"
                )
            }
        }
    }

    private func handleClose(dismissed: Bool) {
        if dismissed {
            // Store dismissal state in session storage, scoped to this codespace
            SessionStorage.shared.set(true, forKey: "claude-auth-dismissed", scope: codespaceName)
            NSLog("🐱 [ClaudeAuth] Stored dismissal for codespace: \(codespaceName)")

            // Reset backend state
            Task {
                try? await CatnipAPI.shared.cancelClaudeOnboarding()
            }
        }

        stopPolling()

        // Reset state
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) {
            status = nil
            polling = false
            code = ""
            hasSubmittedCode = false
            hasClickedOAuthLink = false
        }

        isPresented = false
    }

    private func handleSuccessClose() {
        stopPolling()

        // Notify parent about successful auth
        onAuthComplete()

        // Reset state
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) {
            status = nil
            polling = false
            code = ""
            hasClickedOAuthLink = false
            hasSubmittedCode = false
        }

        isPresented = false
    }

    // MARK: - Polling

    private func startPolling() {
        guard pollingTimer == nil else { return }

        polling = true

        pollingTimer = Timer.scheduledTimer(withTimeInterval: 1.0, repeats: true) { _ in
            Task {
                await pollStatus()
            }
        }
    }

    private func stopPolling() {
        pollingTimer?.invalidate()
        pollingTimer = nil
        polling = false
    }

    private func pollStatus() async {
        do {
            let newStatus = try await CatnipAPI.shared.getClaudeOnboardingStatus()

            await MainActor.run {
                status = newStatus

                // Stop polling when we reach terminal states
                if newStatus.parsedState == .complete || newStatus.parsedState == .error {
                    stopPolling()

                    // Auto-dismiss after 2 seconds on success
                    if newStatus.parsedState == .complete {
                        DispatchQueue.main.asyncAfter(deadline: .now() + 2.0) {
                            handleSuccessClose()
                        }
                    }
                } else if newStatus.parsedState == .authWaiting && !hasSubmittedCode {
                    // Only stop on auth_waiting if we haven't submitted code yet
                    stopPolling()
                }
            }
        } catch {
            NSLog("🐱 [ClaudeAuth] Failed to check onboarding status: \(error)")
        }
    }
}

#Preview {
    @Previewable @State var isPresented = true

    ClaudeAuthSheet(isPresented: $isPresented, codespaceName: "preview-codespace") {
        print("Auth completed")
    }
}
