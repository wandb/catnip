//
//  PRCreationSheet.swift
//  catnip
//
//  Pull request creation sheet
//

import SwiftUI

struct PRCreationSheet: View {
    @Binding var isPresented: Bool
    let workspace: WorkspaceInfo
    @Binding var isCreating: Bool

    @State private var title = ""
    @State private var description = ""
    @State private var error = ""
    @State private var isGeneratingSummary = false
    @State private var showPreview = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("PR Title", text: $title, axis: .vertical)
                        .lineLimit(1...3)
                } header: {
                    Text("Pull Request Details")
                } footer: {
                    if workspace.branch.hasPrefix("/") {
                        Text("Branch: \(workspace.branch.dropFirst())")
                    } else {
                        Text("Branch: \(workspace.branch)")
                    }
                }

                Section {
                    if showPreview {
                        ScrollView {
                            if description.isEmpty {
                                Text("No description")
                                    .foregroundStyle(.secondary)
                                    .italic()
                                    .frame(maxWidth: .infinity, alignment: .leading)
                                    .padding(.vertical, 8)
                            } else {
                                MarkdownText(description)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                            }
                        }
                        .frame(minHeight: 200)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            showPreview = false
                        }
                    } else {
                        TextField("Description (optional)", text: $description, axis: .vertical)
                            .lineLimit(5...15)
                    }
                } header: {
                    HStack {
                        Text("Description")
                        Spacer()
                        Button {
                            showPreview.toggle()
                        } label: {
                            Text(showPreview ? "Edit" : "Preview")
                                .font(.caption)
                        }
                        .buttonStyle(.borderless)
                        .disabled(isGeneratingSummary)
                    }
                }

                if !error.isEmpty {
                    Section {
                        Text(error)
                            .foregroundStyle(.red)
                            .font(.callout)
                    }
                }

                Section {
                    Button {
                        Task {
                            await generateSummary()
                        }
                    } label: {
                        if isGeneratingSummary {
                            HStack {
                                ProgressView()
                                    .scaleEffect(0.8)
                                Text("Generating...")
                            }
                        } else {
                            Label("Generate PR Summary", systemImage: "sparkles")
                        }
                    }
                    .disabled(isGeneratingSummary || isCreating)
                }

                Section {
                    Button {
                        Task {
                            await createPR()
                        }
                    } label: {
                        if isCreating {
                            HStack {
                                ProgressView()
                                    .scaleEffect(0.8)
                                Text("Creating PR...")
                            }
                        } else {
                            Text("Create Pull Request")
                        }
                    }
                    .disabled(title.isEmpty || isCreating || isGeneratingSummary)
                }
            }
            .navigationTitle("Create Pull Request")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button {
                        isPresented = false
                    } label: {
                        Text("Cancel")
                    }
                    .disabled(isCreating)
                }
            }
            .onAppear {
                // Auto-generate summary when sheet appears
                if title.isEmpty && description.isEmpty {
                    Task {
                        await generateSummary()
                    }
                }
            }
        }
    }

    private func generateSummary() async {
        NSLog("🔄 Generating PR summary for workspace: \(workspace.id)")
        isGeneratingSummary = true
        error = ""

        do {
            let summary = try await CatnipAPI.shared.generatePRSummary(
                workspacePath: workspace.path,
                branch: workspace.branch
            )
            await MainActor.run {
                NSLog("✅ Successfully generated PR summary")
                self.title = summary.title
                self.description = summary.description
                self.showPreview = true // Auto-show preview after generation
                isGeneratingSummary = false
            }
        } catch {
            NSLog("❌ Failed to generate PR summary: \(error)")
            await MainActor.run {
                self.error = "Failed to generate summary: \(error.localizedDescription)"
                isGeneratingSummary = false
            }
        }
    }

    private func createPR() async {
        NSLog("🔄 Creating PR for workspace: \(workspace.id)")
        isCreating = true
        error = ""

        do {
            let prUrl = try await CatnipAPI.shared.createPullRequest(
                workspaceId: workspace.id,
                title: title,
                description: description
            )

            await MainActor.run {
                NSLog("✅ Successfully created PR: \(prUrl)")
                isCreating = false
                isPresented = false

                // Open the newly created PR
                if let url = URL(string: prUrl) {
                    UIApplication.shared.open(url)
                }
            }
        } catch {
            NSLog("❌ Failed to create PR: \(error)")
            await MainActor.run {
                self.error = "Failed to create PR: \(error.localizedDescription)"
                isCreating = false
            }
        }
    }
}

#Preview {
    PRCreationSheet(
        isPresented: .constant(true),
        workspace: WorkspaceInfo.preview1,
        isCreating: .constant(false)
    )
}
