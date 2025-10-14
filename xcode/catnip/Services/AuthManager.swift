//
//  AuthManager.swift
//  catnip
//
//  Authentication manager using GitHub OAuth
//

import Foundation
import Combine
import AuthenticationServices
import UIKit

class AuthManager: NSObject, ObservableObject {
    @Published var isAuthenticated = false
    @Published var isLoading = true
    @Published var sessionToken: String?
    @Published var username: String?

    private let baseURL = "https://catnip.run"
    private var authSession: ASWebAuthenticationSession?

    override init() {
        super.init()
        Task { @MainActor in
            // Skip network calls during unit tests
            if UITestingHelper.isRunningTests {
                if UITestingHelper.shouldSkipAuthentication {
                    await UITestingHelper.setupMockAuthenticationIfNeeded(authManager: self)
                } else {
                    // Unit tests - just set loading to false, don't make network calls
                    self.isLoading = false
                }
            } else {
                await loadStoredSession()
            }
        }
    }

    @MainActor
    func loadStoredSession() async {
        do {
            let token = try await KeychainHelper.load(key: "session_token")
            let user = try? await KeychainHelper.load(key: "username")

            // Validate session with server
            let (authenticated, serverUsername) = await CatnipAPI.shared.checkAuthStatus()

            if authenticated {
                self.sessionToken = token
                self.username = user ?? serverUsername
                self.isAuthenticated = true
            } else {
                await clearSession()
            }
        } catch {
            print("No stored session found")
        }

        isLoading = false
    }

    @MainActor
    func login() async -> Bool {
        // Generate state for CSRF protection
        let state = UUID().uuidString

        do {
            try await KeychainHelper.save(key: "oauth_state", value: state)
        } catch {
            print("Failed to save OAuth state: \(error)")
            return false
        }

        // Create redirect URI
        let redirectURI = "catnip://auth"

        // Build OAuth URL
        guard let authURL = URL(string: "\(baseURL)/v1/auth/github/mobile?redirect_uri=\(redirectURI.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed)!)&state=\(state)") else {
            return false
        }

        return await withCheckedContinuation { continuation in
            authSession = ASWebAuthenticationSession(url: authURL, callbackURLScheme: "catnip") { callbackURL, error in
                Task { @MainActor in
                    if let error = error {
                        print("Auth error: \(error)")
                        continuation.resume(returning: false)
                        return
                    }

                    guard let callbackURL = callbackURL else {
                        continuation.resume(returning: false)
                        return
                    }

                    let components = URLComponents(url: callbackURL, resolvingAgainstBaseURL: false)
                    let queryItems = components?.queryItems

                    guard let returnedState = queryItems?.first(where: { $0.name == "state" })?.value,
                          let token = queryItems?.first(where: { $0.name == "token" })?.value,
                          let username = queryItems?.first(where: { $0.name == "username" })?.value else {
                        continuation.resume(returning: false)
                        return
                    }

                    // Verify state
                    let storedState = try? await KeychainHelper.load(key: "oauth_state")
                    guard returnedState == storedState else {
                        print("OAuth state mismatch")
                        continuation.resume(returning: false)
                        return
                    }

                    // Store credentials
                    _ = try? await KeychainHelper.save(key: "session_token", value: token)
                    _ = try? await KeychainHelper.save(key: "username", value: username)

                    self.sessionToken = token
                    self.username = username
                    self.isAuthenticated = true

                    continuation.resume(returning: true)
                }
            }

            authSession?.presentationContextProvider = self
            authSession?.prefersEphemeralWebBrowserSession = false
            authSession?.start()
        }
    }

    @MainActor
    func logout() async {
        // Notify server
        if let token = sessionToken {
            var request = URLRequest(url: URL(string: "\(baseURL)/v1/auth/mobile/logout")!)
            request.httpMethod = "POST"
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")

            _ = try? await URLSession.shared.data(for: request)
        }

        await clearSession()
    }

    @MainActor
    private func clearSession() async {
        _ = try? await KeychainHelper.delete(key: "session_token")
        _ = try? await KeychainHelper.delete(key: "username")
        _ = try? await KeychainHelper.delete(key: "oauth_state")

        // Clear non-sensitive app preferences
        UserDefaults.standard.removeObject(forKey: "codespace_name")
        UserDefaults.standard.removeObject(forKey: "org_name")

        sessionToken = nil
        username = nil
        isAuthenticated = false
    }
}

// MARK: - ASWebAuthenticationPresentationContextProviding

extension AuthManager: ASWebAuthenticationPresentationContextProviding {
    @MainActor
    func presentationAnchor(for session: ASWebAuthenticationSession) -> ASPresentationAnchor {
        // Get the first window scene
        let scenes = UIApplication.shared.connectedScenes
        if let windowScene = scenes.first as? UIWindowScene {
            if let window = windowScene.windows.first {
                return window
            }
            // If no window exists, create one for the window scene
            return ASPresentationAnchor(windowScene: windowScene)
        }
        // Fallback: This should rarely happen, but we need to handle it
        // Create a window for the first available scene
        if let firstScene = scenes.first as? UIWindowScene {
            return ASPresentationAnchor(windowScene: firstScene)
        }
        // Last resort fallback - should never happen in practice
        fatalError("No window scene available for authentication presentation")
    }
}
