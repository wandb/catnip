//
//  HighlightedCodeBlock.swift
//  catnip
//
//  Syntax-highlighted code block component using Highlightr
//

import SwiftUI

/// A code block view with syntax highlighting
struct HighlightedCodeBlock: View {
    let code: String
    let language: String?

    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        ScrollView(.horizontal, showsIndicators: true) {
            Text(highlightedCode)
                .font(AppTheme.Typography.codeRegular)
                .textSelection(.enabled)
                .padding(AppTheme.Spacing.md)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .background(AppTheme.Colors.SyntaxHighlighting.background(for: colorScheme))
        .clipShape(RoundedRectangle(cornerRadius: AppTheme.Spacing.Radius.md))
        .overlay(
            RoundedRectangle(cornerRadius: AppTheme.Spacing.Radius.md)
                .strokeBorder(AppTheme.Colors.Separator.primary, lineWidth: 0.5)
        )
    }

    private var highlightedCode: AttributedString {
        // Use SyntaxHighlighter service with current color scheme
        SyntaxHighlighter.shared.highlight(code: code, language: language, colorScheme: colorScheme)
    }
}

#if DEBUG
#Preview("Swift Code Block") {
    VStack(spacing: 16) {
        Text("Syntax Highlighted Code Block")
            .font(.headline)

        HighlightedCodeBlock(
            code: """
            import SwiftUI

            struct ContentView: View {
                @State private var count = 0

                var body: some View {
                    Button("Count: \\(count)") {
                        count += 1
                    }
                }
            }
            """,
            language: "swift"
        )
    }
    .padding()
}

#Preview("Python Code Block") {
    VStack(spacing: 16) {
        Text("Python Code")
            .font(.headline)

        HighlightedCodeBlock(
            code: """
            def fibonacci(n):
                if n <= 1:
                    return n
                return fibonacci(n-1) + fibonacci(n-2)

            print(fibonacci(10))
            """,
            language: "python"
        )
    }
    .padding()
}
#endif
