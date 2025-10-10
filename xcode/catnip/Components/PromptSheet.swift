//
//  PromptSheet.swift
//  catnip
//
//  Modern prompt input sheet with optional branch selector
//

import SwiftUI

enum PromptSheetMode {
    case askForChanges
    case createWorkspace
}

struct PromptSheet: View {
    @Binding var isPresented: Bool
    @Binding var prompt: String
    @Binding var selectedBranch: String?

    let mode: PromptSheetMode
    let availableBranches: [String]
    let onSubmit: () -> Void
    let isSubmitting: Bool

    @FocusState private var isTextFieldFocused: Bool

    init(
        isPresented: Binding<Bool>,
        prompt: Binding<String>,
        selectedBranch: Binding<String?> = .constant(nil),
        mode: PromptSheetMode,
        availableBranches: [String] = [],
        isSubmitting: Bool = false,
        onSubmit: @escaping () -> Void
    ) {
        self._isPresented = isPresented
        self._prompt = prompt
        self._selectedBranch = selectedBranch
        self.mode = mode
        self.availableBranches = availableBranches
        self.isSubmitting = isSubmitting
        self.onSubmit = onSubmit
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                // Text input area
                ZStack(alignment: .topLeading) {
                    if prompt.isEmpty {
                        Text(placeholderText)
                            .foregroundStyle(.tertiary)
                            .padding(.horizontal, 4)
                            .padding(.top, 8)
                    }

                    TextEditor(text: $prompt)
                        .focused($isTextFieldFocused)
                        .scrollContentBackground(.hidden)
                        .frame(minHeight: 120, maxHeight: 300)
                }
                .padding(.horizontal, 20)
                .padding(.top, 16)

                // Branch selector (only for create workspace mode)
                if mode == .createWorkspace && !availableBranches.isEmpty {
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 8) {
                            ForEach(availableBranches, id: \.self) { branch in
                                BranchPill(
                                    branch: branch,
                                    isSelected: selectedBranch == branch,
                                    onTap: { selectedBranch = branch }
                                )
                            }
                        }
                        .padding(.horizontal, 20)
                        .padding(.vertical, 12)
                    }
                }

                Spacer()

                // Submit button area
                HStack {
                    Spacer()

                    Button {
                        if !prompt.trimmingCharacters(in: .whitespaces).isEmpty && !isSubmitting {
                            onSubmit()
                        }
                    } label: {
                        Group {
                            if isSubmitting {
                                ProgressView()
                                    .progressViewStyle(CircularProgressViewStyle(tint: .white))
                                    .scaleEffect(0.8)
                            } else {
                                Image(systemName: "arrow.up")
                                    .font(.body.weight(.semibold))
                            }
                        }
                        .frame(width: 32, height: 32)
                        .foregroundStyle(.white)
                        .background(
                            Circle()
                                .fill(canSubmit ? Color.accentColor : Color.gray.opacity(0.3))
                        )
                    }
                    .disabled(!canSubmit)
                }
                .padding(.horizontal, 20)
                .padding(.bottom, 20)
            }
            .background(Color(uiColor: .systemBackground))
            .navigationTitle(navigationTitle)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .navigationBarLeading) {
                    Button("Cancel") {
                        isPresented = false
                    }
                    .foregroundStyle(.primary)
                }
            }
            .onAppear {
                // Auto-focus text field when sheet appears
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) {
                    isTextFieldFocused = true
                }
            }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }

    private var navigationTitle: String {
        switch mode {
        case .askForChanges:
            return "Ask for changes"
        case .createWorkspace:
            return "New task"
        }
    }

    private var placeholderText: String {
        switch mode {
        case .askForChanges:
            return "Describe the changes you want"
        case .createWorkspace:
            return "Describe a coding task in wandb/catnip @ feature/codespaces"
        }
    }

    private var canSubmit: Bool {
        !prompt.trimmingCharacters(in: .whitespaces).isEmpty && !isSubmitting
    }
}

struct BranchPill: View {
    let branch: String
    let isSelected: Bool
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            HStack(spacing: 6) {
                Image(systemName: "arrow.branch")
                    .font(.caption)
                Text(branch)
                    .font(.subheadline)
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
            .background(
                RoundedRectangle(cornerRadius: 20)
                    .fill(isSelected ? Color.accentColor.opacity(0.15) : Color(uiColor: .secondarySystemBackground))
            )
            .overlay(
                RoundedRectangle(cornerRadius: 20)
                    .strokeBorder(
                        isSelected ? Color.accentColor : Color.clear,
                        lineWidth: 1.5
                    )
            )
            .foregroundStyle(isSelected ? Color.accentColor : .secondary)
        }
    }
}

#Preview("Ask for Changes") {
    Text("Main View")
        .sheet(isPresented: .constant(true)) {
            PromptSheet(
                isPresented: .constant(true),
                prompt: .constant(""),
                mode: .askForChanges,
                onSubmit: {}
            )
        }
}

#Preview("Create Workspace") {
    Text("Main View")
        .sheet(isPresented: .constant(true)) {
            PromptSheet(
                isPresented: .constant(true),
                prompt: .constant(""),
                selectedBranch: .constant("feature/codespaces"),
                mode: .createWorkspace,
                availableBranches: ["main", "feature/codespaces", "feature/mobile-app"],
                onSubmit: {}
            )
        }
}
