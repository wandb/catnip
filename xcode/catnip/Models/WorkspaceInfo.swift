//
//  WorkspaceInfo.swift
//  catnip
//
//  Data models for workspaces and todos
//

import Foundation

struct WorkspaceInfo: Codable, Identifiable, Hashable {
    let id: String
    let name: String
    let branch: String
    let repoId: String
    let claudeActivityState: ClaudeActivityState?
    let commitCount: Int?
    let isDirty: Bool?
    let lastAccessed: String?
    let createdAt: String?
    let todos: [Todo]?
    let latestSessionTitle: String?
    let latestUserPrompt: String?
    let pullRequestUrl: String?
    let pullRequestState: String?
    let hasCommitsAheadOfRemote: Bool?
    let path: String
    let cacheStatus: CacheStatus?

    enum CodingKeys: String, CodingKey {
        case id, name, branch, path, todos
        case repoId = "repo_id"
        case claudeActivityState = "claude_activity_state"
        case commitCount = "commit_count"
        case isDirty = "is_dirty"
        case lastAccessed = "last_accessed"
        case createdAt = "created_at"
        case latestSessionTitle = "latest_session_title"
        case latestUserPrompt = "latest_user_prompt"
        case pullRequestUrl = "pull_request_url"
        case pullRequestState = "pull_request_state"
        case hasCommitsAheadOfRemote = "has_commits_ahead_of_remote"
        case cacheStatus = "cache_status"
    }

    var displayName: String {
        // For worktrees, name is already the friendly name
        if !name.isEmpty {
            return name
        }
        return "Unnamed workspace"
    }

    var cleanBranch: String {
        var cleaned = branch
        // Handle refs/catnip/name format
        if cleaned.hasPrefix("refs/catnip/") {
            cleaned = String(cleaned.dropFirst("refs/catnip/".count))
        }
        // Handle leading slash
        if cleaned.hasPrefix("/") {
            cleaned = String(cleaned.dropFirst())
        }
        return cleaned.isEmpty ? "main" : cleaned
    }

    var statusText: String {
        switch claudeActivityState {
        case .active: return "Active now"
        case .running: return "Running"
        default: return "Inactive"
        }
    }

    var timeDisplay: String {
        guard let lastAccessedStr = lastAccessed ?? createdAt else {
            return ""
        }

        let formatter = ISO8601DateFormatter()
        guard let date = formatter.date(from: lastAccessedStr) else {
            return ""
        }

        let now = Date()
        let diffComponents = Calendar.current.dateComponents([.day], from: date, to: now)
        guard let days = diffComponents.day else { return "" }

        if days == 0 {
            let timeFormatter = DateFormatter()
            timeFormatter.timeStyle = .short
            return timeFormatter.string(from: date)
        } else if days == 1 {
            return "Yesterday"
        } else if days < 7 {
            let dateFormatter = DateFormatter()
            dateFormatter.dateFormat = "EEE"
            return dateFormatter.string(from: date)
        } else {
            let dateFormatter = DateFormatter()
            dateFormatter.dateFormat = "MMM d"
            return dateFormatter.string(from: date)
        }
    }

    var activityDescription: String? {
        // Prefer session title over user prompt
        if let title = latestSessionTitle, !title.isEmpty {
            return title
        }
        if let prompt = latestUserPrompt, !prompt.isEmpty {
            return prompt
        }
        return nil
    }
}

enum ClaudeActivityState: String, Codable {
    case active
    case running
    case inactive

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let rawValue = try container.decode(String.self)

        // Handle empty string as inactive
        if rawValue.isEmpty {
            self = .inactive
        } else if let state = ClaudeActivityState(rawValue: rawValue) {
            self = state
        } else {
            // Unknown values default to inactive
            self = .inactive
        }
    }
}

struct Todo: Codable, Identifiable, Hashable {
    let id = UUID()
    let content: String
    let status: TodoStatus
    let activeForm: String?

    enum CodingKeys: String, CodingKey {
        case content, status
        case activeForm = "activeForm"
    }
}

enum TodoStatus: String, Codable {
    case pending
    case inProgress = "in_progress"
    case completed
}

struct CacheStatus: Codable, Hashable {
    let isCached: Bool?
    let isLoading: Bool?
    let lastUpdated: Int?

    enum CodingKeys: String, CodingKey {
        case isCached = "is_cached"
        case isLoading = "is_loading"
        case lastUpdated = "last_updated"
    }
}

struct PRSummary: Codable {
    let title: String
    let description: String
}

// MARK: - Session Data Models

/// Full session data returned by /v1/sessions/workspace/{path} endpoint
/// This is a lightweight endpoint for polling during active sessions
struct SessionData: Codable {
    let sessionInfo: SessionSummary?
    let allSessions: [SessionListEntry]?
    let latestUserPrompt: String?
    let latestMessage: String?
    let latestThought: String?
    let stats: SessionStats?
    // Messages and userPrompts are only included when full=true
    // We don't include them here as we're using this for lightweight polling
}

/// Session summary information
struct SessionSummary: Codable {
    let worktreePath: String?
    let sessionStartTime: String?
    let sessionEndTime: String?
    let turnCount: Int?
    let isActive: Bool?
    let lastSessionId: String?
    let currentSessionId: String?
    let header: String?
    let lastCost: Double?
    let lastDuration: Int?
    let lastTotalInputTokens: Int?
    let lastTotalOutputTokens: Int?
}

/// Entry in the sessions list
struct SessionListEntry: Codable {
    let sessionId: String
    let lastModified: String?
    let startTime: String?
    let endTime: String?
    let isActive: Bool?
}

/// Session statistics including token counts and activity metrics
struct SessionStats: Codable, Hashable {
    let totalMessages: Int?
    let userMessages: Int?
    let assistantMessages: Int?
    let humanPromptCount: Int?
    let toolCallCount: Int?
    let totalInputTokens: Int64?
    let totalOutputTokens: Int64?
    let cacheReadTokens: Int64?
    let cacheCreationTokens: Int64?
    let lastContextSizeTokens: Int64?
    let apiCallCount: Int?
    let sessionDurationSeconds: Double?
    let activeDurationSeconds: Double?
    let thinkingBlockCount: Int?
    let subAgentCount: Int?
    let compactionCount: Int?
    let imageCount: Int?
    let activeToolNames: [String: Int]?

    /// Returns the context size in a human-readable format (e.g., "125K tokens")
    var contextSizeDisplay: String? {
        guard let tokens = lastContextSizeTokens, tokens > 0 else { return nil }
        if tokens >= 1000 {
            return "\(tokens / 1000)K tokens"
        }
        return "\(tokens) tokens"
    }
}
