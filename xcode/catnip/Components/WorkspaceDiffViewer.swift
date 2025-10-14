//
//  WorkspaceDiffViewer.swift
//  catnip
//
//  Main diff viewer component for displaying workspace changes
//

import SwiftUI

struct WorkspaceDiffViewer: View {
    let workspaceId: String
    let selectedFile: String?
    let onClose: (() -> Void)?
    let onExpand: (() -> Void)?
    let preloadedDiff: WorktreeDiffResponse?
    let onDiffLoaded: ((WorktreeDiffResponse) -> Void)?

    @State private var diffResponse: WorktreeDiffResponse?
    @State private var loading = true
    @State private var error: String?
    @State private var expandedFiles = Set<String>()
    @Namespace private var namespace

    init(workspaceId: String,
         selectedFile: String? = nil,
         onClose: (() -> Void)? = nil,
         onExpand: (() -> Void)? = nil,
         preloadedDiff: WorktreeDiffResponse? = nil,
         onDiffLoaded: ((WorktreeDiffResponse) -> Void)? = nil) {
        self.workspaceId = workspaceId
        self.selectedFile = selectedFile
        self.onClose = onClose
        self.onExpand = onExpand
        self.preloadedDiff = preloadedDiff
        self.onDiffLoaded = onDiffLoaded
    }

    var body: some View {
        VStack(spacing: 0) {
            // Header
            header

            // Content
            if loading {
                loadingView
            } else if let error = error {
                errorView(error)
            } else if let response = diffResponse {
                if response.fileDiffs.isEmpty {
                    emptyView
                } else {
                    diffList(response)
                }
            }
        }
        .background(Color(uiColor: .systemGroupedBackground))
        .task {
            await loadDiff()
        }
        .onChange(of: selectedFile) {
            // Scroll to selected file when it changes
            if let file = selectedFile {
                scrollToFile(file)
            }
        }
    }

    // MARK: - Header

    private var header: some View {
        HStack(spacing: 12) {
            Image(systemName: "doc.text")
                .font(.caption)
                .foregroundStyle(.secondary)

            if let response = diffResponse {
                VStack(alignment: .leading, spacing: 2) {
                    Text("Diff")
                        .font(.caption.weight(.medium))
                    Text(response.summary)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
            } else {
                Text("Diff")
                    .font(.caption.weight(.medium))
            }

            Spacer()

            if let onExpand = onExpand {
                Button {
                    onExpand()
                } label: {
                    Image(systemName: "arrow.up.left.and.arrow.down.right")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .frame(width: 24, height: 24)
                }
                .buttonStyle(.plain)
            } else if let onClose = onClose {
                Button {
                    onClose()
                } label: {
                    Image(systemName: "xmark")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .frame(width: 24, height: 24)
                }
                .buttonStyle(.plain)
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .background(Color(uiColor: .systemBackground).opacity(0.95))
        .overlay(
            Rectangle()
                .fill(Color(uiColor: .separator))
                .frame(height: 0.5),
            alignment: .bottom
        )
    }

    // MARK: - Loading View

    private var loadingView: some View {
        VStack(spacing: 16) {
            ProgressView()
            Text("Loading diff...")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    // MARK: - Error View

    private func errorView(_ message: String) -> some View {
        VStack(spacing: 16) {
            Image(systemName: "exclamationmark.triangle")
                .font(.title)
                .foregroundStyle(.secondary)
            Text("Failed to load diff")
                .font(.body.weight(.medium))
            Text(message)
                .font(.caption)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
            Button {
                Task { await loadDiff() }
            } label: {
                Text("Retry")
            }
            .buttonStyle(ProminentButtonStyle())
        }
        .padding()
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    // MARK: - Empty View

    private var emptyView: some View {
        VStack(spacing: 16) {
            Image(systemName: "doc.text")
                .font(.largeTitle)
                .foregroundStyle(.secondary.opacity(0.5))
            Text("No changes to show")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    // MARK: - Diff List

    private func diffList(_ response: WorktreeDiffResponse) -> some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(spacing: 8) {
                    ForEach(response.fileDiffs) { fileDiff in
                        DiffFileView(
                            fileDiff: fileDiff,
                            initiallyExpanded: shouldExpand(fileDiff)
                        )
                        .id(fileDiff.filePath)
                        .overlay(
                            Group {
                                if selectedFile == fileDiff.filePath {
                                    RoundedRectangle(cornerRadius: 0)
                                        .strokeBorder(Color.accentColor.opacity(0.5), lineWidth: 2)
                                }
                            }
                        )
                    }
                }
                .padding(.vertical, 8)
            }
            .onChange(of: selectedFile) {
                if let file = selectedFile {
                    withAnimation {
                        proxy.scrollTo(file, anchor: .top)
                    }
                }
            }
        }
    }

    // MARK: - Helpers

    private func shouldExpand(_ fileDiff: FileDiff) -> Bool {
        // Auto-expand if it's the selected file
        if fileDiff.filePath == selectedFile {
            return true
        }

        // Auto-expand small files
        let stats = DiffParser.calculateStats(fileDiff.diffText ?? "")
        return stats.totalChanges <= 500
    }

    private func scrollToFile(_ filePath: String) {
        // Trigger scroll via onChange in diffList
    }

    private func loadDiff() async {
        // Use preloaded diff if available
        if let preloaded = preloadedDiff {
            await MainActor.run {
                diffResponse = preloaded
                loading = false
            }
            return
        }

        loading = true
        error = nil

        do {
            let response = try await CatnipAPI.shared.getWorkspaceDiff(id: workspaceId)
            await MainActor.run {
                diffResponse = response
                loading = false
                onDiffLoaded?(response)
            }
        } catch {
            await MainActor.run {
                self.error = error.localizedDescription
                loading = false
            }
        }
    }
}

// MARK: - Preview

#if DEBUG
// Preview wrapper that uses mock data instead of API
private struct WorkspaceDiffViewerPreview: View {
    let mockResponse: WorktreeDiffResponse
    let selectedFile: String?

    @State private var loading = false
    @State private var error: String? = nil

    var body: some View {
        VStack(spacing: 0) {
            // Header
            HStack(spacing: 12) {
                Image(systemName: "doc.text")
                    .font(.caption)
                    .foregroundStyle(.secondary)

                VStack(alignment: .leading, spacing: 2) {
                    Text("Diff")
                        .font(.caption.weight(.medium))
                    Text(mockResponse.summary)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }

                Spacer()

                Button {
                    // Preview close action
                } label: {
                    Image(systemName: "xmark")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .frame(width: 24, height: 24)
                }
                .buttonStyle(.plain)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
            .background(Color(uiColor: .systemBackground).opacity(0.95))
            .overlay(
                Rectangle()
                    .fill(Color(uiColor: .separator))
                    .frame(height: 0.5),
                alignment: .bottom
            )

            // Content
            ScrollView {
                LazyVStack(spacing: 8) {
                    ForEach(mockResponse.fileDiffs) { fileDiff in
                        DiffFileView(
                            fileDiff: fileDiff,
                            initiallyExpanded: fileDiff.filePath == selectedFile || DiffParser.calculateStats(fileDiff.diffText ?? "").totalChanges <= 500
                        )
                        .id(fileDiff.filePath)
                        .overlay(
                            Group {
                                if selectedFile == fileDiff.filePath {
                                    RoundedRectangle(cornerRadius: 0)
                                        .strokeBorder(Color.accentColor.opacity(0.5), lineWidth: 2)
                                }
                            }
                        )
                    }
                }
                .padding(.vertical, 8)
            }
        }
        .background(Color(uiColor: .systemGroupedBackground))
    }
}
#endif

#Preview("Diff Viewer - Multiple Files") {
    WorkspaceDiffViewerPreview(
        mockResponse: .preview,
        selectedFile: nil
    )
}

#Preview("Diff Viewer - Selected File") {
    WorkspaceDiffViewerPreview(
        mockResponse: .preview,
        selectedFile: "src/components/Button.tsx"
    )
}

#Preview("Diff Viewer - Loading") {
    VStack(spacing: 16) {
        ProgressView()
        Text("Loading diff...")
            .font(.caption)
            .foregroundStyle(.secondary)
    }
    .frame(maxWidth: .infinity, maxHeight: .infinity)
    .background(Color(uiColor: .systemGroupedBackground))
}

#Preview("Diff Viewer - Empty") {
    VStack(spacing: 16) {
        Image(systemName: "doc.text")
            .font(.largeTitle)
            .foregroundStyle(.secondary.opacity(0.5))
        Text("No changes to show")
            .font(.caption)
            .foregroundStyle(.secondary)
    }
    .frame(maxWidth: .infinity, maxHeight: .infinity)
    .background(Color(uiColor: .systemGroupedBackground))
}
