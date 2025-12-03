//
//  EnhancedMarkdownText.swift
//  catnip
//
//  Markdown renderer with syntax-highlighted code blocks
//

import SwiftUI
import MarkdownUI

/// Enhanced markdown text component that renders code blocks with syntax highlighting
struct EnhancedMarkdownText: View {
    let markdown: String

    @State private var codeBlocks: [CodeBlockInfo] = []
    @State private var renderedSections: [MarkdownSection] = []

    init(_ markdown: String) {
        self.markdown = markdown
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            // Process markdown and render sections
            ForEach(renderedSections, id: \.id) { section in
                section.view
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .onAppear {
            processMarkdown()
        }
        .onChange(of: markdown) { _, _ in
            processMarkdown()
        }
    }

    private func processMarkdown() {
        let result = MarkdownCodeBlockProcessor.extractCodeBlocks(from: markdown)
        codeBlocks = result.codeBlocks
        renderedSections = renderSections()
    }

    private func renderSections() -> [MarkdownSection] {
        var sections: [MarkdownSection] = []

        // Sort code blocks by their position in the original markdown
        let sortedBlocks = codeBlocks.sorted { block1, block2 in
            block1.range.lowerBound < block2.range.lowerBound
        }

        var lastIndex = markdown.startIndex

        for codeBlock in sortedBlocks {
            // Add markdown before this code block
            if lastIndex < codeBlock.range.lowerBound {
                let beforeText = String(markdown[lastIndex..<codeBlock.range.lowerBound])
                if !beforeText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    sections.append(MarkdownSection(
                        id: "md-\\(sections.count)",
                        view: AnyView(MarkdownText(beforeText))
                    ))
                }
            }

            // Add the code block
            sections.append(MarkdownSection(
                id: "code-\\(codeBlock.id)",
                view: AnyView(
                    HighlightedCodeBlock(
                        code: codeBlock.code,
                        language: codeBlock.language
                    )
                )
            ))

            lastIndex = codeBlock.range.upperBound
        }

        // Add any remaining markdown after the last code block
        if lastIndex < markdown.endIndex {
            let afterText = String(markdown[lastIndex...])
            if !afterText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                sections.append(MarkdownSection(
                    id: "md-end",
                    view: AnyView(MarkdownText(afterText))
                ))
            }
        }

        // If no code blocks, just render the whole thing as markdown
        if sections.isEmpty {
            sections.append(MarkdownSection(
                id: "md-all",
                view: AnyView(MarkdownText(markdown))
            ))
        }

        return sections
    }
}

/// Helper struct for rendering sections
private struct MarkdownSection: Identifiable {
    let id: String
    let view: AnyView
}

#if DEBUG
#Preview("Enhanced Markdown with Code") {
    ScrollView {
        EnhancedMarkdownText("""
        # Example with Code

        Here's some Swift code:

        ```swift
        import SwiftUI

        struct ContentView: View {
            var body: some View {
                Text("Hello, World!")
            }
        }
        ```

        And here's some Python:

        ```python
        def hello():
            print("Hello, World!")
        ```

        That's it!
        """)
        .padding()
    }
}
#endif
