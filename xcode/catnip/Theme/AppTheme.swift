//
//  AppTheme.swift
//  catnip
//
//  Theme system matching iOS design guidelines
//

import SwiftUI

struct AppTheme {
    // MARK: - Colors

    struct Colors {
        // Text colors using system semantics
        struct Text {
            static let primary = Color.primary
            static let secondary = Color.secondary
            static let tertiary = Color(uiColor: .tertiaryLabel)
            static let quaternary = Color(uiColor: .quaternaryLabel)
            static let placeholder = Color(uiColor: .placeholderText)
            static let link = Color.blue
        }

        // Background colors
        struct Background {
            static let primary = Color(uiColor: .systemBackground)
            static let secondary = Color(uiColor: .secondarySystemBackground)
            static let tertiary = Color(uiColor: .tertiarySystemBackground)
            static let grouped = Color(uiColor: .systemGroupedBackground)
            static let secondaryGrouped = Color(uiColor: .secondarySystemGroupedBackground)
            static let tertiaryGrouped = Color(uiColor: .tertiarySystemGroupedBackground)
        }

        // Fill colors
        struct Fill {
            static let primary = Color(uiColor: .systemFill)
            static let secondary = Color(uiColor: .secondarySystemFill)
            static let tertiary = Color(uiColor: .tertiarySystemFill)
            static let quaternary = Color(uiColor: .quaternarySystemFill)
        }

        // Separator colors
        struct Separator {
            static let primary = Color(uiColor: .separator)
            static let opaque = Color(uiColor: .opaqueSeparator)
        }

        // Brand colors
        struct Brand {
            static let primary = Color(hex: "7c3aed") // Purple
            static let primaryLight = Color(hex: "a855f7")
            static let primaryDark = Color(hex: "5b21b6")
            static let accent = Color(hex: "3b82f6") // Blue
            static let accentLight = Color(hex: "60a5fa")
            static let accentDark = Color(hex: "1d4ed8")
        }

        // Status colors
        struct Status {
            static let success = Color.green
            static let warning = Color.orange
            static let error = Color.red
            static let info = Color.blue
        }

        // Control colors
        struct Control {
            static let background = Color(uiColor: .systemFill)
            static let tint = Color.blue
        }

        // Syntax highlighting theme support
        struct SyntaxHighlighting {
            // Light theme background (Xcode-inspired)
            static let lightBackground = Color(hex: "f7f7f7")

            // Dark theme background (Xcode-inspired)
            static let darkBackground = Color(hex: "1f1f24")

            static func background(for colorScheme: ColorScheme) -> Color {
                colorScheme == .dark ? darkBackground : lightBackground
            }
        }
    }

    // MARK: - Typography

    struct Typography {
        // Display styles
        static let largeTitle = Font.system(size: 34, weight: .regular, design: .default)
        static let title1 = Font.system(size: 28, weight: .regular, design: .default)
        static let title2 = Font.system(size: 22, weight: .regular, design: .default)
        static let title3 = Font.system(size: 20, weight: .regular, design: .default)

        // Headline
        static let headline = Font.system(size: 17, weight: .semibold, design: .default)

        // Body
        static let body = Font.system(size: 17, weight: .regular, design: .default)
        static let bodyEmphasized = Font.system(size: 17, weight: .semibold, design: .default)

        // Callout
        static let callout = Font.system(size: 16, weight: .regular, design: .default)
        static let calloutEmphasized = Font.system(size: 16, weight: .semibold, design: .default)

        // Subheadline
        static let subheadline = Font.system(size: 15, weight: .regular, design: .default)
        static let subheadlineEmphasized = Font.system(size: 15, weight: .semibold, design: .default)

        // Footnote
        static let footnote = Font.system(size: 13, weight: .regular, design: .default)
        static let footnoteEmphasized = Font.system(size: 13, weight: .semibold, design: .default)

        // Caption
        static let caption1 = Font.system(size: 12, weight: .regular, design: .default)
        static let caption1Emphasized = Font.system(size: 12, weight: .semibold, design: .default)
        static let caption2 = Font.system(size: 11, weight: .regular, design: .default)
        static let caption2Emphasized = Font.system(size: 11, weight: .semibold, design: .default)

        // Monospace for code
        static let codeRegular = Font.system(size: 13, weight: .regular, design: .monospaced)
        static let codeBold = Font.system(size: 13, weight: .semibold, design: .monospaced)
    }

    // MARK: - Spacing

    struct Spacing {
        // Base unit (8pt grid)
        static let base: CGFloat = 8

        // Scale based on 8pt grid
        static let xs: CGFloat = 4  // 0.5x
        static let sm: CGFloat = 8  // 1x
        static let md: CGFloat = 16 // 2x
        static let lg: CGFloat = 24 // 3x
        static let xl: CGFloat = 32 // 4x
        static let xxl: CGFloat = 40 // 5x
        static let xxxl: CGFloat = 48 // 6x

        // Component-specific spacing
        struct Component {
            static let containerPadding: CGFloat = 16
            static let cardPadding: CGFloat = 16
            static let buttonPadding: CGFloat = 12
            static let inputPadding: CGFloat = 12
            static let sectionSpacing: CGFloat = 24
            static let itemSpacing: CGFloat = 12
            static let groupSpacing: CGFloat = 8
            static let headerPadding: CGFloat = 16
            static let listItemPadding: CGFloat = 16
            static let formFieldSpacing: CGFloat = 16
        }

        // Border radius
        struct Radius {
            static let none: CGFloat = 0
            static let xs: CGFloat = 4
            static let sm: CGFloat = 6
            static let md: CGFloat = 8
            static let lg: CGFloat = 12
            static let xl: CGFloat = 16
            static let xxl: CGFloat = 20
            static let full: CGFloat = 9999

            static let button: CGFloat = 8
            static let card: CGFloat = 12
            static let modal: CGFloat = 12
            static let control: CGFloat = 8
        }

        // Border widths
        struct BorderWidth {
            static let none: CGFloat = 0
            static let thin: CGFloat = 0.5 // Hairline
            static let normal: CGFloat = 1
            static let thick: CGFloat = 2
        }
    }

    // MARK: - Shadows

    struct Shadows {
        static let none: (color: Color, radius: CGFloat, x: CGFloat, y: CGFloat) = (.clear, 0, 0, 0)
        static let sm: (color: Color, radius: CGFloat, x: CGFloat, y: CGFloat) = (.black.opacity(0.05), 2, 0, 1)
        static let md: (color: Color, radius: CGFloat, x: CGFloat, y: CGFloat) = (.black.opacity(0.1), 4, 0, 2)
        static let lg: (color: Color, radius: CGFloat, x: CGFloat, y: CGFloat) = (.black.opacity(0.15), 8, 0, 4)
        static let xl: (color: Color, radius: CGFloat, x: CGFloat, y: CGFloat) = (.black.opacity(0.2), 16, 0, 8)
    }

    // MARK: - Animation Durations

    struct Animation {
        static let fast: Double = 0.2
        static let normal: Double = 0.3
        static let slow: Double = 0.5
    }

    // MARK: - Opacity

    struct Opacity {
        static let disabled: Double = 0.4
        static let pressed: Double = 0.7
        static let loading: Double = 0.6
        static let overlay: Double = 0.8
    }
}

// MARK: - Color Extension for Hex

extension Color {
    init(hex: String) {
        let hex = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
        var int: UInt64 = 0
        Scanner(string: hex).scanHexInt64(&int)
        let a, r, g, b: UInt64
        switch hex.count {
        case 3: // RGB (12-bit)
            (a, r, g, b) = (255, (int >> 8) * 17, (int >> 4 & 0xF) * 17, (int & 0xF) * 17)
        case 6: // RGB (24-bit)
            (a, r, g, b) = (255, int >> 16, int >> 8 & 0xFF, int & 0xFF)
        case 8: // ARGB (32-bit)
            (a, r, g, b) = (int >> 24, int >> 16 & 0xFF, int >> 8 & 0xFF, int & 0xFF)
        default:
            (a, r, g, b) = (1, 1, 1, 0)
        }

        self.init(
            .sRGB,
            red: Double(r) / 255,
            green: Double(g) / 255,
            blue:  Double(b) / 255,
            opacity: Double(a) / 255
        )
    }
}
