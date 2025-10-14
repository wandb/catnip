//
//  StatusIndicator.swift
//  catnip
//
//  Status indicator with pulse animation for active state
//

import SwiftUI

struct StatusIndicator: View {
    let status: ClaudeActivityState?

    @State private var isPulsing = false

    private var indicatorColor: Color {
        switch status {
        case .active:
            return Color(hex: "22c55e") // Green
        case .running:
            return Color(hex: "6b7280") // Gray
        default:
            return .clear
        }
    }

    private var hasBorder: Bool {
        status == nil || status == .inactive
    }

    var body: some View {
        Circle()
            .fill(indicatorColor)
            .frame(width: 8, height: 8)
            .overlay(
                hasBorder ?
                Circle()
                    .strokeBorder(Color(hex: "d1d5db"), lineWidth: 1)
                : nil
            )
            .opacity(status == .active && isPulsing ? 0.4 : 1.0)
            .onAppear {
                if status == .active {
                    withAnimation(.easeInOut(duration: 1.0).repeatForever(autoreverses: true)) {
                        isPulsing = true
                    }
                }
            }
    }
}

#Preview {
    VStack(spacing: 32) {
        HStack(spacing: 20) {
            VStack(spacing: 8) {
                StatusIndicator(status: .active)
                Text("Active")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            VStack(spacing: 8) {
                StatusIndicator(status: .running)
                Text("Running")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            VStack(spacing: 8) {
                StatusIndicator(status: .inactive)
                Text("Inactive")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            VStack(spacing: 8) {
                StatusIndicator(status: nil)
                Text("Unknown")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }

        // Show in context
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text("wandb/catnip")
                    .font(.subheadline)
                Text("·")
                Text("feature/docs")
                    .font(.subheadline)
                Spacer()
                StatusIndicator(status: .active)
            }
        }
        .padding()
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12))
        .padding()
    }
    .frame(maxWidth: .infinity, maxHeight: .infinity)
    .background(Color(uiColor: .systemGroupedBackground))
}
