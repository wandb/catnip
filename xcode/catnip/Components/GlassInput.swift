//
//  GlassInput.swift
//  catnip
//
//  Glass effect text input
//

import SwiftUI

struct GlassInput: View {
    let placeholder: String
    @Binding var text: String
    var isMultiline: Bool = false
    var isDisabled: Bool = false

    var body: some View {
        Group {
            if isMultiline {
                TextEditor(text: $text)
                    .font(AppTheme.Typography.body)
                    .foregroundColor(AppTheme.Colors.Text.primary)
                    .frame(minHeight: 120)
                    .padding(AppTheme.Spacing.Component.inputPadding)
                    .background(AppTheme.Colors.Fill.secondary)
                    .clipShape(RoundedRectangle(cornerRadius: AppTheme.Spacing.Radius.md))
                    .overlay(
                        RoundedRectangle(cornerRadius: AppTheme.Spacing.Radius.md)
                            .strokeBorder(AppTheme.Colors.Separator.primary, lineWidth: AppTheme.Spacing.BorderWidth.thin)
                    )
                    .overlay(
                        text.isEmpty ?
                        Text(placeholder)
                            .font(AppTheme.Typography.body)
                            .foregroundColor(AppTheme.Colors.Text.placeholder)
                            .padding(AppTheme.Spacing.Component.inputPadding)
                            .allowsHitTesting(false)
                            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                        : nil
                    )
            } else {
                TextField(placeholder, text: $text)
                    .font(AppTheme.Typography.body)
                    .foregroundColor(AppTheme.Colors.Text.primary)
                    .padding(AppTheme.Spacing.Component.inputPadding)
                    .background(AppTheme.Colors.Fill.secondary)
                    .clipShape(RoundedRectangle(cornerRadius: AppTheme.Spacing.Radius.md))
                    .overlay(
                        RoundedRectangle(cornerRadius: AppTheme.Spacing.Radius.md)
                            .strokeBorder(AppTheme.Colors.Separator.primary, lineWidth: AppTheme.Spacing.BorderWidth.thin)
                    )
            }
        }
        .disabled(isDisabled)
        .opacity(isDisabled ? AppTheme.Opacity.disabled : 1.0)
    }
}

#Preview {
    VStack(spacing: 16) {
        GlassInput(placeholder: "Enter text", text: .constant(""))
        GlassInput(placeholder: "Enter multiple lines", text: .constant(""), isMultiline: true)
        GlassInput(placeholder: "Disabled", text: .constant(""), isDisabled: true)
    }
    .padding()
    .background(AppTheme.Colors.Background.grouped)
}
