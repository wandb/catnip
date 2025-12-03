//
//  SyntaxHighlighter.swift
//  catnip
//
//  Syntax highlighting service using Highlightr
//

import Foundation
import SwiftUI
import Highlightr

/// Service for syntax highlighting code blocks
class SyntaxHighlighter {
    static let shared = SyntaxHighlighter()

    private let highlightr: Highlightr?
    private var currentTheme: String = "xcode"

    private init() {
        self.highlightr = Highlightr()
        configureHighlightr()
    }

    private func configureHighlightr() {
        guard let highlightr = highlightr else { return }

        // Use built-in xcode theme
        highlightr.setTheme(to: currentTheme)

        // Match AppTheme.Typography.codeRegular (13pt monospace)
        highlightr.theme.setCodeFont(.monospacedSystemFont(ofSize: 13, weight: .regular))
    }

    /// Update theme for light/dark mode
    func updateTheme(for colorScheme: ColorScheme) {
        guard let highlightr = highlightr else { return }

        let themeName = colorScheme == .dark ? "monokai" : "xcode"

        if currentTheme != themeName {
            currentTheme = themeName
            highlightr.setTheme(to: themeName)
        }
    }

    /// Highlight code and return AttributedString
    /// - Parameters:
    ///   - code: The source code to highlight
    ///   - language: Language identifier (e.g., "swift", "python", "javascript")
    ///   - colorScheme: The current color scheme (light or dark) to select appropriate theme
    /// - Returns: Highlighted AttributedString
    func highlight(code: String, language: String?, colorScheme: ColorScheme) -> AttributedString {
        guard let highlightr = highlightr else {
            return plainText(code)
        }

        // Update theme based on color scheme BEFORE highlighting
        updateTheme(for: colorScheme)

        // Try with language first
        if let lang = language, !lang.isEmpty,
           let highlighted = highlightr.highlight(code, as: lang) {
            return convert(highlighted) ?? plainText(code)
        }

        // Fallback to auto-detection
        if let highlighted = highlightr.highlight(code) {
            return convert(highlighted) ?? plainText(code)
        }

        return plainText(code)
    }

    private func convert(_ nsAttr: NSAttributedString) -> AttributedString? {
        try? AttributedString(nsAttr, including: \.uiKit)
    }

    private func plainText(_ code: String) -> AttributedString {
        var attr = AttributedString(code)
        attr.font = .monospacedSystemFont(ofSize: 13, weight: .regular)
        return attr
    }
}

/*
INSTRUCTIONS TO ADD HIGHLIGHTR:

1. Open the Xcode project
2. Select the project in the navigator (top-level "Catnip")
3. Select the "Catnip" app target
4. Go to the "Package Dependencies" tab
5. Click the "+" button at the bottom
6. Enter URL: https://github.com/raspu/Highlightr
7. Select "Up to Next Major" version 2.2.1
8. Click "Add Package"
9. Select "Highlightr" library
10. Click "Add Package"

Then uncomment the code marked above in this file.
*/
