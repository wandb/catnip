//
//  CodespaceActivityWidget.swift
//  catnip
//
//  Live Activity widget for codespace creation progress
//
//  IMPORTANT: This file requires a Widget Extension target to be added to the Xcode project.
//  See the setup instructions in LIVE_ACTIVITY_SETUP.md
//

import ActivityKit
import WidgetKit
import SwiftUI

// MARK: - Live Activity Widget

@available(iOS 16.1, *)
struct CodespaceActivityWidget: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: CodespaceActivityAttributes.self) { context in
            // Lock screen and banner UI
            CodespaceActivityView(context: context)
        } dynamicIsland: { context in
            DynamicIsland {
                // Expanded region
                DynamicIslandExpandedRegion(.leading) {
                    Image(systemName: "terminal.fill")
                        .foregroundStyle(Color.accentColor)
                }

                DynamicIslandExpandedRegion(.trailing) {
                    Text("\(Int(context.state.progress * 100))%")
                        .font(.title3.bold())
                }

                DynamicIslandExpandedRegion(.bottom) {
                    VStack(alignment: .leading, spacing: 8) {
                        Text(context.state.status)
                            .font(.caption)
                            .foregroundStyle(.secondary)

                        ProgressView(value: context.state.progress)
                            .progressViewStyle(.linear)
                            .tint(Color.accentColor)

                        if context.state.elapsedSeconds > 0 {
                            Text(formatElapsedTime(context.state.elapsedSeconds))
                                .font(.caption2)
                                .foregroundStyle(.tertiary)
                        }
                    }
                }
            } compactLeading: {
                Image(systemName: "terminal.fill")
                    .foregroundStyle(Color.accentColor)
            } compactTrailing: {
                ProgressView(value: context.state.progress)
                    .progressViewStyle(.circular)
                    .tint(Color.accentColor)
            } minimal: {
                ProgressView(value: context.state.progress)
                    .progressViewStyle(.circular)
                    .tint(Color.accentColor)
            }
        }
    }

    private func formatElapsedTime(_ seconds: Int) -> String {
        let minutes = seconds / 60
        let remainingSeconds = seconds % 60
        if minutes > 0 {
            return "\(minutes)m \(remainingSeconds)s elapsed"
        } else {
            return "\(seconds)s elapsed"
        }
    }
}

// MARK: - Lock Screen / Banner View

@available(iOS 16.1, *)
struct CodespaceActivityView: View {
    let context: ActivityViewContext<CodespaceActivityAttributes>

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 12) {
                Image(systemName: "terminal.fill")
                    .font(.title2)
                    .foregroundStyle(Color.accentColor)

                VStack(alignment: .leading, spacing: 4) {
                    Text("Catnip")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(.secondary)

                    Text(context.state.status)
                        .font(.body.weight(.medium))
                        .lineLimit(2)
                }

                Spacer()

                VStack(alignment: .trailing, spacing: 2) {
                    Text("\(Int(context.state.progress * 100))%")
                        .font(.title3.bold())
                        .foregroundStyle(.primary)

                    if context.state.elapsedSeconds > 0 {
                        Text(formatElapsedTime(context.state.elapsedSeconds))
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }
            }

            // Progress bar
            ProgressView(value: context.state.progress)
                .progressViewStyle(.linear)
                .tint(Color.accentColor)
        }
        .padding()
        .background(.regularMaterial)
    }

    private func formatElapsedTime(_ seconds: Int) -> String {
        let minutes = seconds / 60
        let remainingSeconds = seconds % 60
        if minutes > 0 {
            return "\(minutes)m \(remainingSeconds)s"
        } else {
            return "\(seconds)s"
        }
    }
}

// MARK: - Preview

@available(iOS 16.1, *)
#Preview("Live Activity", as: .content, using: CodespaceActivityAttributes(repositoryName: "wandb/catnip")) {
    CodespaceActivityWidget()
} contentStates: {
    CodespaceActivityAttributes.ContentState(
        status: "Creating codespace in wandb/catnip...",
        progress: 0.35,
        elapsedSeconds: 105
    )

    CodespaceActivityAttributes.ContentState(
        status: "Creating codespace in wandb/catnip...",
        progress: 0.75,
        elapsedSeconds: 225
    )

    CodespaceActivityAttributes.ContentState(
        status: "Codespace ready!",
        progress: 1.0,
        elapsedSeconds: 300
    )
}
