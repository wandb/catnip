//
//  IOSButton.swift
//  catnip
//
//  iOS-style button component
//

import SwiftUI

struct IOSButton: View {
    let title: String
    let action: () -> Void
    var variant: Variant = .primary
    var size: Size = .medium
    var isDisabled: Bool = false
    var isLoading: Bool = false

    enum Variant {
        case primary
        case secondary
        case tertiary
        case destructive
    }

    enum Size {
        case small
        case medium
        case large
    }

    private var backgroundColor: Color {
        switch variant {
        case .primary:
            return Color(hex: "007AFF")
        case .secondary:
            return AppTheme.Colors.Fill.secondary
        case .tertiary:
            return .clear
        case .destructive:
            return Color(hex: "FF3B30")
        }
    }

    private var foregroundColor: Color {
        switch variant {
        case .primary, .destructive:
            return .white
        case .secondary, .tertiary:
            return Color(hex: "007AFF")
        }
    }

    private var padding: EdgeInsets {
        switch size {
        case .small:
            return EdgeInsets(top: 8, leading: 16, bottom: 8, trailing: 16)
        case .medium:
            return EdgeInsets(top: 12, leading: 20, bottom: 12, trailing: 20)
        case .large:
            return EdgeInsets(top: 16, leading: 24, bottom: 16, trailing: 24)
        }
    }

    private var minHeight: CGFloat {
        switch size {
        case .small: return 32
        case .medium: return 44
        case .large: return 50
        }
    }

    private var fontSize: CGFloat {
        switch size {
        case .small: return 14
        case .medium: return 16
        case .large: return 18
        }
    }

    var body: some View {
        Button(action: action) {
            ZStack {
                if isLoading {
                    ProgressView()
                        .progressViewStyle(CircularProgressViewStyle(tint: foregroundColor))
                } else {
                    Text(title)
                        .font(.system(size: fontSize, weight: .semibold))
                        .foregroundColor(foregroundColor)
                }
            }
            .frame(maxWidth: .infinity)
            .frame(minHeight: minHeight)
            .padding(padding)
            .background(backgroundColor)
            .clipShape(RoundedRectangle(cornerRadius: 10))
            .overlay(
                variant == .secondary ?
                RoundedRectangle(cornerRadius: 10)
                    .strokeBorder(AppTheme.Colors.Separator.primary, lineWidth: 0.5)
                : nil
            )
        }
        .disabled(isDisabled || isLoading)
        .opacity(isDisabled ? AppTheme.Opacity.disabled : 1.0)
    }
}

#Preview {
    VStack(spacing: 16) {
        IOSButton(title: "Primary Button", action: {})
        IOSButton(title: "Secondary Button", action: {}, variant: .secondary)
        IOSButton(title: "Tertiary Button", action: {}, variant: .tertiary)
        IOSButton(title: "Destructive Button", action: {}, variant: .destructive)
        IOSButton(title: "Loading", action: {}, isLoading: true)
        IOSButton(title: "Disabled", action: {}, isDisabled: true)
    }
    .padding()
}
