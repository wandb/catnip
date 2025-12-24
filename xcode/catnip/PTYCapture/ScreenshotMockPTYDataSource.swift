//
//  ScreenshotMockPTYDataSource.swift
//  catnip
//
//  Width-adaptive mock PTY data source for App Store screenshots
//  Generates Claude Code-like content that adapts to any terminal size
//

import Foundation
import Combine

/// Content style for mock PTY data
enum MockPTYContentStyle {
    /// Active session with conversation history (default for screenshots)
    case activeSession
    /// Fresh workspace with no conversation - just welcome screen
    case newWorkspace
}

/// Mock PTY data source that generates width-adaptive Claude Code content
/// Used for App Store screenshots where captured PTY data doesn't render correctly
/// at different resolutions
class ScreenshotMockPTYDataSource: PTYDataSource {
    @Published private var _isConnected = false
    @Published private var _error: String?

    var isConnected: Published<Bool>.Publisher { $_isConnected }
    var error: Published<String?>.Publisher { $_error }

    var onData: ((Data) -> Void)?
    var onJSONMessage: ((PTYControlMessage) -> Void)?

    private var terminalCols: Int = 80
    private var terminalRows: Int = 24

    // Content style determines what kind of terminal content to show
    private let contentStyle: MockPTYContentStyle

    // Configurable display values (randomized for variety)
    private let version: String
    private let modelName: String
    private let projectPath: String
    private let recentProject: String
    private let tokenCount: String

    private static let versions = ["v2.0.71", "v2.0.72", "v2.1.0", "v2.0.69"]
    private static let models = ["Opus 4.5", "Sonnet 4.5", "Claude Max"]
    private static let projects = ["~/projects/mobile-app", "~/Development/catnip", "~/code/web-dashboard", "~/work/api-server"]
    private static let recentProjects = ["catnip-ios", "mobile-app", "dashboard", "api-service"]

    init(contentStyle: MockPTYContentStyle = .activeSession) {
        self.contentStyle = contentStyle
        self.version = Self.versions.randomElement() ?? "v2.0.71"
        self.modelName = Self.models.randomElement() ?? "Opus 4.5"
        self.projectPath = Self.projects.randomElement() ?? "~/projects/mobile-app"
        self.recentProject = Self.recentProjects.randomElement() ?? "catnip-ios"
        self.tokenCount = "\(Int.random(in: 8000...25000).formatted()) tokens"
    }

    func connect() {
        NSLog("📸 Screenshot Mock PTY: Connecting (style: \(contentStyle))...")
        // Defer to next run loop to allow SwiftUI to set up Combine subscriptions
        // before we publish the connected state. No actual time delay needed.
        DispatchQueue.main.async { [weak self] in
            self?._isConnected = true
            NSLog("📸 Screenshot Mock PTY: Connected")
        }
    }

    func disconnect() {
        _isConnected = false
        NSLog("📸 Screenshot Mock PTY: Disconnected")
    }

    func sendInput(_ text: String) {
        NSLog("📸 Screenshot Mock PTY: Input ignored: %@", text)
    }

    func sendResize(cols: UInt16, rows: UInt16) {
        terminalCols = Int(cols)
        terminalRows = Int(rows)
        NSLog("📸 Screenshot Mock PTY: Resize to %dx%d", terminalCols, terminalRows)

        // Send buffer-size acknowledgment on next run loop
        DispatchQueue.main.async { [weak self] in
            let message = PTYControlMessage(
                type: "buffer-size",
                data: nil,
                submit: nil,
                cols: cols,
                rows: rows
            )
            self?.onJSONMessage?(message)
        }
    }

    func sendReady() {
        NSLog("📸 Screenshot Mock PTY: Ready, generating content for %dx%d", terminalCols, terminalRows)
        generateAndSendContent()
    }

    // MARK: - Content Generation

    private func generateAndSendContent() {
        let content: String
        switch contentStyle {
        case .activeSession:
            content = generateActiveSessionContent()
        case .newWorkspace:
            content = generateNewWorkspaceContent()
        }

        guard let data = content.data(using: .utf8) else { return }

        // Defer to next run loop to ensure terminal view is ready to receive data.
        // Then send buffer-complete immediately after data to signal replay is done.
        // Using async (no delay) is deterministic - it just waits for current
        // run loop to finish, unlike asyncAfter which depends on system timing.
        DispatchQueue.main.async { [weak self] in
            self?.onData?(data)

            // Send buffer-complete on next run loop after data is processed
            DispatchQueue.main.async {
                let message = PTYControlMessage(
                    type: "buffer-complete",
                    data: nil,
                    submit: nil,
                    cols: nil,
                    rows: nil
                )
                self?.onJSONMessage?(message)
            }
        }
    }

    /// Generate content for an active session with conversation history
    private func generateActiveSessionContent() -> String {
        let width = terminalCols
        var lines: [String] = []

        // ANSI color codes
        let reset = "\u{1B}[0m"
        let orange = "\u{1B}[38;2;215;119;87m"
        let dim = "\u{1B}[2m"
        let green = "\u{1B}[32m"
        let cyan = "\u{1B}[36m"
        let bold = "\u{1B}[1m"
        let italic = "\u{1B}[3m"
        let yellow = "\u{1B}[33m"
        let blue = "\u{1B}[34m"
        let magenta = "\u{1B}[35m"

        // Clear screen and move cursor to top
        lines.append("\u{1B}[2J\u{1B}[H")

        // Add padding at top so header is visible below nav bar
        for _ in 0..<5 {
            lines.append("")
        }

        // Calculate box dimensions - use ~95% of available width dynamically
        let boxWidth = max(40, width - 4)
        let leftColWidth = boxWidth / 2
        let rightColWidth = boxWidth - leftColWidth - 1  // -1 for divider

        // Welcome box - top border with title
        let title = " Claude Code \(version) "
        let titlePadding = max(0, boxWidth - title.count - 2) / 2
        lines.append(orange + "╭─" + String(repeating: "─", count: titlePadding) + title + String(repeating: "─", count: boxWidth - title.count - titlePadding - 2) + "╮" + reset)

        // Two-column content
        let leftContent = [
            "",
            centerInWidth("Welcome back!", leftColWidth),
            "",
            centerInWidth("* ▐▛███▜▌ *", leftColWidth),
            centerInWidth("* ▝▜█████▛▘ *", leftColWidth),
            centerInWidth("*  ▘▘ ▝▝  *", leftColWidth),
            "",
            centerInWidth("\(modelName) · Claude API", leftColWidth),
            centerInWidth(projectPath, leftColWidth),
            ""
        ]

        let rightContent = [
            orange + "Tips for getting started" + reset,
            "Run /init to create a CLAUDE.md",
            "file with instructions for Claude",
            String(repeating: "─", count: rightColWidth),
            orange + "Recent activity" + reset,
            dim + recentProject + reset + " · feature/screenshots",
            dim + "2 hours ago" + reset,
            "",
            "",
            ""
        ]

        // Generate rows with two columns
        for i in 0..<max(leftContent.count, rightContent.count) {
            let left = i < leftContent.count ? leftContent[i] : ""
            let right = i < rightContent.count ? rightContent[i] : ""
            let leftPadded = padToWidth(left, width: leftColWidth)
            let rightPadded = padToWidth(right, width: rightColWidth)
            lines.append(orange + "│" + reset + leftPadded + orange + "│" + reset + rightPadded + orange + "│" + reset)
        }

        // Bottom border
        lines.append(orange + "╰" + String(repeating: "─", count: boxWidth) + "╯" + reset)

        // Conversation - keep it short so header stays visible
        lines.append("")

        // User prompt
        lines.append(green + "> " + reset + bold + "Let's make magic" + reset)
        lines.append("")

        // Thinking indicator
        lines.append(yellow + "∴ " + reset + italic + "Thinking…" + reset)
        lines.append("")

        // Thinking content - more lines for taller screens (iPhone), fewer for iPad
        let isCompactHeight = terminalRows < 30

        lines.append(dim + italic + "  The user said \"Let's make magic\" - this is a greeting." + reset)
        lines.append(dim + italic + "  Let me check if any skills apply here." + reset)

        if !isCompactHeight {
            // Extra lines for taller screens (iPhone)
            lines.append("")
            lines.append(dim + italic + "  Looking at the available skills:" + reset)
            lines.append(dim + italic + "  - superpowers:brainstorming - Use when creating or developing" + reset)
            lines.append(dim + italic + "  - superpowers:using-superpowers - Use when starting any conversation" + reset)
        }
        lines.append("")

        // Claude's response (using ● instead of ⏺ which renders as emoji)
        lines.append(green + "●" + reset + " Ready when you are. What are we building?")
        lines.append("")

        // Separator and input prompt
        let separatorWidth = min(width, boxWidth + 2)
        lines.append(dim + String(repeating: "─", count: separatorWidth) + reset)
        lines.append(green + "> " + reset + "build me a simple todo app" + dim + String(repeating: " ", count: max(0, separatorWidth - 35)) + "↵ send" + reset)
        lines.append(dim + String(repeating: "─", count: separatorWidth) + reset)

        // Status bar
        let statusPath = "  " + projectPath
        let branch = "main*"
        let statusLeft = blue + statusPath + reset + "   " + green + branch + reset
        let statusRight = dim + tokenCount + reset
        let statusPadding = max(0, separatorWidth - statusPath.count - branch.count - tokenCount.count - 8)
        lines.append(statusLeft + String(repeating: " ", count: statusPadding) + statusRight)

        return lines.joined(separator: "\r\n")
    }

    /// Generate content for a new/fresh workspace - just welcome screen, no conversation
    private func generateNewWorkspaceContent() -> String {
        let width = terminalCols
        var lines: [String] = []

        // ANSI color codes
        let reset = "\u{1B}[0m"
        let orange = "\u{1B}[38;2;215;119;87m"
        let dim = "\u{1B}[2m"
        let green = "\u{1B}[32m"
        let blue = "\u{1B}[34m"

        // Clear screen and move cursor to top
        lines.append("\u{1B}[2J\u{1B}[H")

        // Add padding at top so header is visible below nav bar
        for _ in 0..<5 {
            lines.append("")
        }

        // Calculate box dimensions
        let boxWidth = max(40, width - 4)
        let leftColWidth = boxWidth / 2
        let rightColWidth = boxWidth - leftColWidth - 1

        // Welcome box - top border with title
        let title = " Claude Code \(version) "
        let titlePadding = max(0, boxWidth - title.count - 2) / 2
        lines.append(orange + "╭─" + String(repeating: "─", count: titlePadding) + title + String(repeating: "─", count: boxWidth - title.count - titlePadding - 2) + "╮" + reset)

        // Two-column content - cleaner for new workspace
        let leftContent = [
            "",
            centerInWidth("Ready to begin", leftColWidth),
            "",
            centerInWidth("* ▐▛███▜▌ *", leftColWidth),
            centerInWidth("* ▝▜█████▛▘ *", leftColWidth),
            centerInWidth("*  ▘▘ ▝▝  *", leftColWidth),
            "",
            centerInWidth("\(modelName) · Claude API", leftColWidth),
            centerInWidth(projectPath, leftColWidth),
            ""
        ]

        let rightContent = [
            orange + "Tips for getting started" + reset,
            "Run /init to create a CLAUDE.md",
            "file with instructions for Claude",
            String(repeating: "─", count: rightColWidth),
            orange + "Quick commands" + reset,
            dim + "/help" + reset + " - Show available commands",
            dim + "/clear" + reset + " - Clear conversation",
            "",
            "",
            ""
        ]

        // Generate rows with two columns
        for i in 0..<max(leftContent.count, rightContent.count) {
            let left = i < leftContent.count ? leftContent[i] : ""
            let right = i < rightContent.count ? rightContent[i] : ""
            let leftPadded = padToWidth(left, width: leftColWidth)
            let rightPadded = padToWidth(right, width: rightColWidth)
            lines.append(orange + "│" + reset + leftPadded + orange + "│" + reset + rightPadded + orange + "│" + reset)
        }

        // Bottom border
        lines.append(orange + "╰" + String(repeating: "─", count: boxWidth) + "╯" + reset)

        // Empty prompt area - ready for input
        lines.append("")

        // Input prompt (empty, ready for user)
        let separatorWidth = min(width, boxWidth + 2)
        lines.append(dim + String(repeating: "─", count: separatorWidth) + reset)
        lines.append(green + "> " + reset + dim + "What would you like to build?" + String(repeating: " ", count: max(0, separatorWidth - 35)) + "↵ send" + reset)
        lines.append(dim + String(repeating: "─", count: separatorWidth) + reset)

        // Status bar - fresh workspace, no tokens used yet
        let statusPath = "  " + projectPath
        let branch = "main"
        let statusLeft = blue + statusPath + reset + "   " + green + branch + reset
        let statusRight = dim + "0 tokens" + reset
        let statusPadding = max(0, separatorWidth - statusPath.count - branch.count - "0 tokens".count - 8)
        lines.append(statusLeft + String(repeating: " ", count: statusPadding) + statusRight)

        return lines.joined(separator: "\r\n")
    }

    private func centerInWidth(_ text: String, _ width: Int) -> String {
        let textLen = visibleLength(text)
        if textLen >= width {
            return text
        }
        let padding = (width - textLen) / 2
        return String(repeating: " ", count: padding) + text + String(repeating: " ", count: width - textLen - padding)
    }

    private func padToWidth(_ text: String, width: Int) -> String {
        let textLen = visibleLength(text)
        if textLen >= width {
            return text
        }
        return text + String(repeating: " ", count: width - textLen)
    }

    // Calculate visible length ignoring ANSI escape codes
    private func visibleLength(_ text: String) -> Int {
        // Remove ANSI escape sequences
        let pattern = "\u{1B}\\[[0-9;]*m"
        let stripped = text.replacingOccurrences(of: pattern, with: "", options: .regularExpression)
        return stripped.count
    }
}

#if DEBUG
extension ScreenshotMockPTYDataSource {
    /// Create mock data source for screenshots (active session with conversation)
    static func createForScreenshots() -> ScreenshotMockPTYDataSource {
        return ScreenshotMockPTYDataSource(contentStyle: .activeSession)
    }

    /// Create mock data source for a new/fresh workspace (no conversation history)
    static func createForNewWorkspace() -> ScreenshotMockPTYDataSource {
        return ScreenshotMockPTYDataSource(contentStyle: .newWorkspace)
    }

    /// Create mock data source with specified content style
    static func create(style: MockPTYContentStyle) -> ScreenshotMockPTYDataSource {
        return ScreenshotMockPTYDataSource(contentStyle: style)
    }
}
#endif
