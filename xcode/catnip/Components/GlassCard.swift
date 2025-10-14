//
//  GlassCard.swift
//  catnip
//
//  Glass morphism card component
//

import SwiftUI

struct GlassCard<Content: View>: View {
    let content: Content

    init(@ViewBuilder content: () -> Content) {
        self.content = content()
    }

    var body: some View {
        content
            .padding(AppTheme.Spacing.Component.cardPadding)
            .background(.ultraThinMaterial)
            .clipShape(RoundedRectangle(cornerRadius: AppTheme.Spacing.Radius.xl))
            .shadow(
                color: AppTheme.Shadows.lg.color,
                radius: AppTheme.Shadows.lg.radius,
                x: AppTheme.Shadows.lg.x,
                y: AppTheme.Shadows.lg.y
            )
    }
}

#Preview {
    ZStack {
        AppTheme.Colors.Background.grouped
            .ignoresSafeArea()

        GlassCard {
            VStack(alignment: .leading, spacing: 16) {
                Text("Glass Card")
                    .font(AppTheme.Typography.title2)
                Text("This is a glass morphism card with blur effect")
                    .font(AppTheme.Typography.body)
                    .foregroundColor(AppTheme.Colors.Text.secondary)
            }
        }
        .padding()
    }
}
