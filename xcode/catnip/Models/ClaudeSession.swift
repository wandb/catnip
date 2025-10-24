//
//  ClaudeSession.swift
//  catnip
//
//  Data model for Claude session information
//

import Foundation

struct ClaudeSessionsResponse: Codable {
    let sessions: [String: ClaudeSessionData]

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        sessions = try container.decode([String: ClaudeSessionData].self)
    }
}

struct ClaudeSessionData: Codable {
    let turnCount: Int
    let isActive: Bool

    enum CodingKeys: String, CodingKey {
        case turnCount = "turnCount"
        case isActive = "isActive"
    }
}

struct LatestMessageResponse: Codable {
    let content: String
    let isError: Bool

    enum CodingKeys: String, CodingKey {
        case content = "content"
        case isError = "isError"
    }
}

struct CheckoutResponse: Codable {
    let worktree: WorkspaceInfo

    enum CodingKeys: String, CodingKey {
        case worktree = "worktree"
    }
}

// MARK: - Claude Settings

struct ClaudeSettings: Codable {
    let theme: String?
    let notificationsEnabled: Bool
    let authenticated: Bool
    let hasCompletedOnboarding: Bool
    let numStartups: Int?
    let version: String?

    enum CodingKeys: String, CodingKey {
        case theme
        case notificationsEnabled = "notificationsEnabled"
        case authenticated = "isAuthenticated"
        case hasCompletedOnboarding = "hasCompletedOnboarding"
        case numStartups = "numStartups"
        case version
    }
}

// MARK: - Claude Onboarding

enum ClaudeOnboardingState: String, Codable {
    case idle = "idle"
    case starting = "starting"
    case authUrl = "auth_url"
    case authWaiting = "auth_waiting"
    case codeSubmitted = "code_submitted"
    case complete = "complete"
    case error = "error"
}

struct ClaudeOnboardingStatus: Codable {
    let state: String
    let oauthUrl: String?
    let message: String?
    let errorMessage: String?

    enum CodingKeys: String, CodingKey {
        case state
        case oauthUrl = "oauth_url"
        case message
        case errorMessage = "error_message"
    }

    var parsedState: ClaudeOnboardingState {
        ClaudeOnboardingState(rawValue: state) ?? .idle
    }
}

struct ClaudeOnboardingSubmitCodeRequest: Codable {
    let code: String
}
