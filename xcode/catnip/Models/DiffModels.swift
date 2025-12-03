//
//  DiffModels.swift
//  catnip
//
//  Data models for diff viewing
//

import Foundation

// MARK: - Diff Response Models

struct WorktreeDiffResponse: Codable {
    let summary: String
    let fileDiffs: [FileDiff]
    let totalFiles: Int
    let worktreeId: String
    let worktreeName: String
    let sourceBranch: String
    let forkCommit: String

    enum CodingKeys: String, CodingKey {
        case summary
        case fileDiffs = "file_diffs"
        case totalFiles = "total_files"
        case worktreeId = "worktree_id"
        case worktreeName = "worktree_name"
        case sourceBranch = "source_branch"
        case forkCommit = "fork_commit"
    }

    // Regular initializer for creating instances (e.g., in tests/previews)
    init(summary: String, fileDiffs: [FileDiff], totalFiles: Int, worktreeId: String, worktreeName: String, sourceBranch: String, forkCommit: String) {
        self.summary = summary
        self.fileDiffs = fileDiffs
        self.totalFiles = totalFiles
        self.worktreeId = worktreeId
        self.worktreeName = worktreeName
        self.sourceBranch = sourceBranch
        self.forkCommit = forkCommit
    }

    // Custom decoder to handle null file_diffs from backend
    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        summary = try container.decode(String.self, forKey: .summary)
        // If file_diffs is null, default to empty array
        fileDiffs = try container.decodeIfPresent([FileDiff].self, forKey: .fileDiffs) ?? []
        totalFiles = try container.decode(Int.self, forKey: .totalFiles)
        worktreeId = try container.decode(String.self, forKey: .worktreeId)
        worktreeName = try container.decode(String.self, forKey: .worktreeName)
        sourceBranch = try container.decode(String.self, forKey: .sourceBranch)
        forkCommit = try container.decode(String.self, forKey: .forkCommit)
    }
}

struct FileDiff: Codable, Identifiable {
    let id = UUID()
    let filePath: String
    let changeType: String
    let oldContent: String?
    let newContent: String?
    let diffText: String?
    let isExpanded: Bool

    enum CodingKeys: String, CodingKey {
        case filePath = "file_path"
        case changeType = "change_type"
        case oldContent = "old_content"
        case newContent = "new_content"
        case diffText = "diff_text"
        case isExpanded = "is_expanded"
    }

    /// Detect syntax highlighting language from file path
    /// Uses LanguageDetector service based on GitHub Linguist data
    var language: String? {
        LanguageDetector.detectLanguage(from: filePath)
    }
}

// MARK: - Diff Parsing Models

enum DiffLineType {
    case context
    case add
    case remove
    case header
}

struct DiffLine: Identifiable {
    let id = UUID()
    let type: DiffLineType
    let oldLineNumber: Int?
    let newLineNumber: Int?
    let content: String
}

struct FileDiffStats {
    let additions: Int
    let deletions: Int

    var totalChanges: Int {
        additions + deletions
    }
}

// MARK: - Preview Extensions

#if DEBUG
extension FileDiff {
    static var preview1: FileDiff {
        FileDiff(
            filePath: "src/components/Button.tsx",
            changeType: "modified",
            oldContent: nil,
            newContent: nil,
            diffText: """
@@ -1,8 +1,12 @@
 import React from 'react';
+import { cn } from '@/lib/utils';

 interface ButtonProps {
   label: string;
-  onClick: () => void;
+  onClick?: () => void;
+  variant?: 'primary' | 'secondary';
+  disabled?: boolean;
 }

-export const Button: React.FC<ButtonProps> = ({ label, onClick }) => {
-  return <button onClick={onClick}>{label}</button>;
+export const Button: React.FC<ButtonProps> = ({ label, onClick, variant = 'primary', disabled = false }) => {
+  return (
+    <button
+      onClick={onClick}
+      disabled={disabled}
+      className={cn('btn', `btn-${variant}`)}
+    >
+      {label}
+    </button>
+  );
 };
""",
            isExpanded: true
        )
    }

    static var preview2: FileDiff {
        FileDiff(
            filePath: "src/utils/helpers.ts",
            changeType: "modified",
            oldContent: nil,
            newContent: nil,
            diffText: """
@@ -15,6 +15,11 @@
   return str.charAt(0).toUpperCase() + str.slice(1);
 }

+export function formatDate(date: Date): string {
+  return date.toLocaleDateString('en-US', {
+    year: 'numeric', month: 'long', day: 'numeric'
+  });
+}
+
 export function debounce<T extends (...args: any[]) => any>(
   func: T,
   wait: number
""",
            isExpanded: true
        )
    }

    static var preview3: FileDiff {
        FileDiff(
            filePath: "README.md",
            changeType: "modified",
            oldContent: nil,
            newContent: nil,
            diffText: """
@@ -1,5 +1,7 @@
 # Project Name

-A simple project description
+A comprehensive project for building amazing things
+
+## Features

 ## Installation
""",
            isExpanded: true
        )
    }
}

extension WorktreeDiffResponse {
    static var preview: WorktreeDiffResponse {
        WorktreeDiffResponse(
            summary: "3 files changed, 25 insertions(+), 8 deletions(-)",
            fileDiffs: [
                .preview1,
                .preview2,
                .preview3
            ],
            totalFiles: 3,
            worktreeId: "preview-workspace-id",
            worktreeName: "feature/update-components",
            sourceBranch: "main",
            forkCommit: "abc123"
        )
    }
}
#endif

// MARK: - Diff Parser

struct DiffParser {
    /// Parse unified diff text into an array of DiffLine objects
    static func parse(_ diffText: String) -> [DiffLine] {
        let lines = diffText.split(separator: "\n", omittingEmptySubsequences: false)
        var result: [DiffLine] = []
        var oldLineNumber = 0
        var newLineNumber = 0

        for line in lines {
            let lineStr = String(line)

            // Parse hunk headers like @@ -1,3 +1,84 @@
            if lineStr.hasPrefix("@@") {
                if let match = lineStr.range(of: #"@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@"#, options: .regularExpression) {
                    let matchStr = String(lineStr[match])
                    let pattern = #"@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@"#
                    if let regex = try? NSRegularExpression(pattern: pattern),
                       let result = regex.firstMatch(in: matchStr, range: NSRange(matchStr.startIndex..., in: matchStr)) {
                        if let oldRange = Range(result.range(at: 1), in: matchStr),
                           let newRange = Range(result.range(at: 2), in: matchStr) {
                            oldLineNumber = Int(String(matchStr[oldRange])) ?? 0
                            newLineNumber = Int(String(matchStr[newRange])) ?? 0
                        }
                    }
                }
                result.append(DiffLine(type: .header, oldLineNumber: nil, newLineNumber: nil, content: lineStr))
                continue
            }

            // Skip file headers
            if lineStr.hasPrefix("---") || lineStr.hasPrefix("+++") {
                continue
            }

            // Handle different line types
            if lineStr.hasPrefix("+") {
                result.append(DiffLine(
                    type: .add,
                    oldLineNumber: nil,
                    newLineNumber: newLineNumber,
                    content: String(lineStr.dropFirst())
                ))
                newLineNumber += 1
            } else if lineStr.hasPrefix("-") {
                result.append(DiffLine(
                    type: .remove,
                    oldLineNumber: oldLineNumber,
                    newLineNumber: nil,
                    content: String(lineStr.dropFirst())
                ))
                oldLineNumber += 1
            } else {
                // Context line (unchanged)
                let content = lineStr.hasPrefix(" ") ? String(lineStr.dropFirst()) : lineStr
                result.append(DiffLine(
                    type: .context,
                    oldLineNumber: oldLineNumber,
                    newLineNumber: newLineNumber,
                    content: content
                ))
                oldLineNumber += 1
                newLineNumber += 1
            }
        }

        return result
    }

    /// Calculate statistics for a diff text
    static func calculateStats(_ diffText: String) -> FileDiffStats {
        let lines = diffText.split(separator: "\n")
        var additions = 0
        var deletions = 0

        for line in lines {
            if line.hasPrefix("+") && !line.hasPrefix("+++") {
                additions += 1
            } else if line.hasPrefix("-") && !line.hasPrefix("---") {
                deletions += 1
            }
        }

        return FileDiffStats(additions: additions, deletions: deletions)
    }
}
