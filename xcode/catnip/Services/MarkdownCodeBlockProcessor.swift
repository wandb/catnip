//
//  MarkdownCodeBlockProcessor.swift
//  catnip
//
//  Preprocessor to extract code blocks from markdown for syntax highlighting
//

import Foundation
import SwiftUI

/// Represents a code block found in markdown
struct CodeBlockInfo: Identifiable {
    let id: String
    let code: String
    let language: String?
    let range: Range<String.Index>

    init(code: String, language: String?, range: Range<String.Index>) {
        self.id = UUID().uuidString
        self.code = code
        self.language = language
        self.range = range
    }
}

/// Processes markdown to extract and replace code blocks with placeholders
class MarkdownCodeBlockProcessor {

    /// Extract code blocks from markdown and return processed text + code blocks
    /// - Parameter markdown: Original markdown text
    /// - Returns: Tuple of (processed markdown with placeholders, extracted code blocks)
    static func extractCodeBlocks(from markdown: String) -> (processedMarkdown: String, codeBlocks: [CodeBlockInfo]) {
        var codeBlocks: [CodeBlockInfo] = []
        var processedMarkdown = markdown

        // Regex pattern to match fenced code blocks
        // Matches: ```language\ncode\n```
        let pattern = #"```(\w+)?\n([\s\S]*?)```"#

        guard let regex = try? NSRegularExpression(pattern: pattern, options: []) else {
            return (markdown, [])
        }

        let nsString = markdown as NSString
        let matches = regex.matches(in: markdown, range: NSRange(location: 0, length: nsString.length))

        // Process matches in reverse to maintain string indices
        for match in matches.reversed() {
            guard let fullRange = Range(match.range, in: markdown),
                  let codeRange = Range(match.range(at: 2), in: markdown) else {
                continue
            }

            // Extract language (optional)
            let language: String?
            if match.range(at: 1).location != NSNotFound,
               let langRange = Range(match.range(at: 1), in: markdown) {
                language = String(markdown[langRange])
            } else {
                language = nil
            }

            // Extract code
            let code = String(markdown[codeRange])

            // Create code block info
            let codeBlock = CodeBlockInfo(code: code, language: language, range: fullRange)
            codeBlocks.insert(codeBlock, at: 0) // Insert at beginning since we're processing in reverse

            // Replace with placeholder
            let placeholder = "{{CODE_BLOCK_\\(codeBlock.id)}}"
            processedMarkdown.replaceSubrange(fullRange, with: placeholder)
        }

        return (processedMarkdown, codeBlocks)
    }

    /// Find placeholder positions in the processed markdown
    /// - Parameters:
    ///   - processedMarkdown: Markdown with placeholders
    ///   - codeBlocks: Array of code blocks
    /// - Returns: Dictionary mapping code block IDs to their placeholder text
    static func findPlaceholders(in processedMarkdown: String, for codeBlocks: [CodeBlockInfo]) -> [String: String] {
        var placeholders: [String: String] = [:]

        for codeBlock in codeBlocks {
            let placeholder = "{{CODE_BLOCK_\\(codeBlock.id)}}"
            placeholders[codeBlock.id] = placeholder
        }

        return placeholders
    }
}
