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
