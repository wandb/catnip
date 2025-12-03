//
//  DiffFileView.swift
//  catnip
//
//  Component for displaying a single file diff with GitHub-style UI
//

import SwiftUI

struct DiffFileView: View {
    let fileDiff: FileDiff
    @State private var isExpanded: Bool

    private let maxAutoExpandLines = 500
    private let stats: FileDiffStats

    @Environment(\.colorScheme) private var colorScheme

    init(fileDiff: FileDiff, initiallyExpanded: Bool = false) {
        self.fileDiff = fileDiff
        self.stats = DiffParser.calculateStats(fileDiff.diffText ?? "")
        self._isExpanded = State(initialValue: initiallyExpanded || stats.totalChanges <= maxAutoExpandLines)
    }

    var body: some View {
        VStack(spacing: 0) {
            // File Header
            fileHeader

            // Diff Content
            if isExpanded, let diffText = fileDiff.diffText {
                diffContent(diffText)
            } else if !isExpanded && stats.totalChanges > maxAutoExpandLines {
                collapsedMessage
            }
        }
        .background(Color(uiColor: .secondarySystemBackground))
    }

    // MARK: - File Header

    private var fileHeader: some View {
        Button {
            withAnimation(.easeInOut(duration: 0.2)) {
                isExpanded.toggle()
            }
        } label: {
            HStack(spacing: 12) {
                Image(systemName: isExpanded ? "chevron.down" : "chevron.right")
                    .font(.caption)
                    .foregroundStyle(.secondary)

                Image(systemName: "doc.text")
                    .font(.caption)
                    .foregroundStyle(.secondary)

                Text(fileDiff.filePath)
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(.primary)
                    .lineLimit(1)

                Spacer()

                // GitHub-style stats bars
                if stats.additions > 0 || stats.deletions > 0 {
                    statsView
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
        }
        .buttonStyle(.plain)
        .background(Color(uiColor: .tertiarySystemBackground))
    }

    private var statsView: some View {
        HStack(spacing: 8) {
            // Colored bars
            HStack(spacing: 2) {
                let total = stats.totalChanges
                let greenBars = min(5, Int(round(Double(stats.additions) / Double(total) * 5)))
                let redBars = min(5 - greenBars, Int(round(Double(stats.deletions) / Double(total) * 5)))
                let grayBars = 5 - greenBars - redBars

                ForEach(0..<greenBars, id: \.self) { _ in
                    RoundedRectangle(cornerRadius: 1)
                        .fill(Color.green)
                        .frame(width: 6, height: 6)
                }
                ForEach(0..<redBars, id: \.self) { _ in
                    RoundedRectangle(cornerRadius: 1)
                        .fill(Color.red)
                        .frame(width: 6, height: 6)
                }
                ForEach(0..<grayBars, id: \.self) { _ in
                    RoundedRectangle(cornerRadius: 1)
                        .fill(Color.gray.opacity(0.3))
                        .frame(width: 6, height: 6)
                }
            }

            // Numbers
            HStack(spacing: 4) {
                if stats.additions > 0 {
                    Text("+\(stats.additions)")
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(.green)
                }
                if stats.deletions > 0 {
                    Text("-\(stats.deletions)")
                        .font(.system(.caption, design: .monospaced))
                        .foregroundStyle(.red)
                }
            }
        }
    }

    // MARK: - Diff Content

    private func diffContent(_ diffText: String) -> some View {
        ScrollView(.horizontal, showsIndicators: false) {
            VStack(alignment: .leading, spacing: 0) {
                let lines = DiffParser.parse(diffText)
                ForEach(Array(lines.enumerated()), id: \.element.id) { index, line in
                    diffLineView(line: line, lineId: "\(fileDiff.id)-\(index)")
                        .id(line.id)
                }
            }
        }
        .overlay(
            // Right fade effect (GitHub-style)
            HStack {
                Spacer()
                LinearGradient(
                    gradient: Gradient(colors: [
                        Color(uiColor: .systemGroupedBackground).opacity(0),
                        Color(uiColor: .systemGroupedBackground).opacity(0.8),
                        Color(uiColor: .systemGroupedBackground)
                    ]),
                    startPoint: .leading,
                    endPoint: .trailing
                )
                .frame(width: 20)
                .allowsHitTesting(false)
            }
        )
    }

    private func diffLineView(line: DiffLine, lineId: String) -> some View {
        HStack(spacing: 0) {
            if line.type == .header {
                Text(line.content)
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 4)
                    .background(Color(uiColor: .tertiarySystemBackground))
            } else {
                // Single compact line number (GitHub-style)
                Text(lineNumberText(for: line))
                    .frame(width: 32, alignment: .trailing)
                    .font(.system(.caption2, design: .monospaced))
                    .foregroundStyle(.secondary.opacity(0.6))
                    .padding(.horizontal, 6)
                    .background(lineNumberBackground(for: line.type))

                // Change indicator
                Text(linePrefix(for: line.type))
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(lineColor(for: line.type))
                    .frame(width: 16)

                // Content with syntax highlighting
                Text(highlightedContent(for: line))
                    .padding(.leading, 4)
                    .padding(.trailing, 12)
                    .background(lineBackground(for: line.type))
            }
        }
        .fixedSize(horizontal: true, vertical: false)
        .background(line.type != .header ? lineBackground(for: line.type) : Color.clear)
    }

    // MARK: - Collapsed Message

    private var collapsedMessage: some View {
        Button {
            withAnimation(.easeInOut(duration: 0.2)) {
                isExpanded = true
            }
        } label: {
            HStack {
                Spacer()
                VStack(spacing: 4) {
                    Text("Large diff (\(stats.totalChanges) changes)")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Text("Click to expand")
                        .font(.caption2)
                        .foregroundStyle(.secondary.opacity(0.8))
                }
                Spacer()
            }
            .padding(.vertical, 20)
        }
        .buttonStyle(.plain)
        .background(Color(uiColor: .tertiarySystemBackground).opacity(0.5))
    }

    // MARK: - Syntax Highlighting

    /// Apply syntax highlighting to line content
    private func highlightedContent(for line: DiffLine) -> AttributedString {
        // Use empty string as placeholder for empty lines
        let content = line.content.isEmpty ? " " : line.content

        // Only apply syntax highlighting if we have a language
        guard let language = fileDiff.language else {
            var attr = AttributedString(content)
            attr.font = .system(.caption, design: .monospaced)
            attr.foregroundColor = lineColor(for: line.type)
            return attr
        }

        // Get highlighted code from SyntaxHighlighter
        // The background colors (green/red) will show the diff,
        // while the syntax highlighting colors show the code structure
        let highlighted = SyntaxHighlighter.shared.highlight(
            code: content,
            language: language,
            colorScheme: colorScheme
        )

        return highlighted
    }

    // MARK: - Style Helpers

    private func lineNumberText(for line: DiffLine) -> String {
        // Show line number based on type (GitHub-style)
        switch line.type {
        case .add:
            return line.newLineNumber.map { "\($0)" } ?? ""
        case .remove:
            return line.oldLineNumber.map { "\($0)" } ?? ""
        case .context:
            // For context lines, prefer new line number
            return line.newLineNumber.map { "\($0)" } ?? line.oldLineNumber.map { "\($0)" } ?? ""
        case .header:
            return ""
        }
    }

    private func linePrefix(for type: DiffLineType) -> String {
        switch type {
        case .add: return "+"
        case .remove: return "-"
        case .context: return " "
        case .header: return ""
        }
    }

    private func lineColor(for type: DiffLineType) -> Color {
        switch type {
        case .add: return Color(red: 0.0, green: 0.5, blue: 0.0)
        case .remove: return Color(red: 0.7, green: 0.0, blue: 0.0)
        case .context: return .primary
        case .header: return .secondary
        }
    }

    private func lineBackground(for type: DiffLineType) -> Color {
        switch type {
        case .add: return Color.green.opacity(0.1)
        case .remove: return Color.red.opacity(0.1)
        case .context: return Color.clear
        case .header: return Color.clear
        }
    }

    private func lineNumberBackground(for type: DiffLineType) -> Color {
        switch type {
        case .add: return Color.green.opacity(0.15)
        case .remove: return Color.red.opacity(0.15)
        case .context: return Color(uiColor: .tertiarySystemBackground).opacity(0.3)
        case .header: return Color.clear
        }
    }
}

// MARK: - Preference Key for scroll width

private struct ScrollWidthKey: PreferenceKey {
    static var defaultValue: CGFloat = 0
    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
        value = max(value, nextValue())
    }
}

// MARK: - Preview

#if DEBUG
#Preview("Expanded File") {
    ScrollView {
        DiffFileView(fileDiff: .preview1, initiallyExpanded: true)
    }
    .background(Color(uiColor: .systemGroupedBackground))
}

#Preview("Collapsed File") {
    ScrollView {
        DiffFileView(fileDiff: .preview1, initiallyExpanded: false)
    }
    .background(Color(uiColor: .systemGroupedBackground))
}

#Preview("Multiple Files - Edge to Edge") {
    ScrollView {
        VStack(spacing: 8) {
            DiffFileView(fileDiff: .preview1, initiallyExpanded: true)
            DiffFileView(fileDiff: .preview2, initiallyExpanded: true)
            DiffFileView(fileDiff: .preview3, initiallyExpanded: false)
        }
        .padding(.vertical, 8)
    }
    .background(Color(uiColor: .systemGroupedBackground))
}
#endif
