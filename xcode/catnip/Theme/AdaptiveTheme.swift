//
//  AdaptiveTheme.swift
//  catnip
//
//  Adaptive theming system for iPhone, iPad, and macOS
//

import SwiftUI
import Combine

// MARK: - Device Context

/// Represents the current device and size class context
enum DeviceContext {
    case iPhoneCompact      // iPhone portrait
    case iPhoneLandscape    // iPhone landscape
    case iPadCompact        // iPad in compact mode (1/3 split view, portrait)
    case iPadRegular        // iPad in regular mode (full screen, landscape)
    case macCompact         // macOS compact window
    case macRegular         // macOS regular/large window

    var isPhone: Bool {
        switch self {
        case .iPhoneCompact, .iPhoneLandscape:
            return true
        default:
            return false
        }
    }

    var isTablet: Bool {
        switch self {
        case .iPadCompact, .iPadRegular:
            return true
        default:
            return false
        }
    }

    var isMac: Bool {
        switch self {
        case .macCompact, .macRegular:
            return true
        default:
            return false
        }
    }

    var isCompact: Bool {
        switch self {
        case .iPhoneCompact, .iPadCompact, .macCompact:
            return true
        default:
            return false
        }
    }
}

// MARK: - Adaptive Theme

/// Provides device-appropriate layout values and preferences
class AdaptiveTheme: ObservableObject {
    @Published var context: DeviceContext

    init(horizontalSizeClass: UserInterfaceSizeClass?,
         verticalSizeClass: UserInterfaceSizeClass?,
         idiom: UIUserInterfaceIdiom = UIDevice.current.userInterfaceIdiom) {
        self.context = Self.determineContext(
            horizontal: horizontalSizeClass,
            vertical: verticalSizeClass,
            idiom: idiom
        )
    }

    // MARK: - Context Determination

    static func determineContext(
        horizontal: UserInterfaceSizeClass?,
        vertical: UserInterfaceSizeClass?,
        idiom: UIUserInterfaceIdiom
    ) -> DeviceContext {
        switch idiom {
        case .phone:
            // iPhone: portrait = compact width, landscape = compact height
            if horizontal == .compact {
                return .iPhoneCompact  // Portrait
            } else {
                return .iPhoneLandscape  // Landscape
            }

        case .pad:
            // iPad: compact horizontal = split view or portrait, regular = full screen landscape
            if horizontal == .compact {
                return .iPadCompact  // Split view or portrait
            } else {
                return .iPadRegular  // Full screen landscape
            }

        case .mac:
            // macOS: based on window size
            if horizontal == .compact {
                return .macCompact
            } else {
                return .macRegular
            }

        default:
            // Default to phone compact for unknown devices
            return .iPhoneCompact
        }
    }

    func update(horizontalSizeClass: UserInterfaceSizeClass?,
                verticalSizeClass: UserInterfaceSizeClass?,
                idiom: UIUserInterfaceIdiom = UIDevice.current.userInterfaceIdiom) {
        let newContext = Self.determineContext(
            horizontal: horizontalSizeClass,
            vertical: verticalSizeClass,
            idiom: idiom
        )
        if newContext != context {
            context = newContext
        }
    }

    // MARK: - Adaptive Spacing

    /// Container padding (outer edges)
    var containerPadding: CGFloat {
        switch context {
        case .iPhoneCompact, .iPhoneLandscape:
            return 16
        case .iPadCompact:
            return 20
        case .iPadRegular:
            return 24
        case .macCompact:
            return 20
        case .macRegular:
            return 32
        }
    }

    /// Card/component internal padding
    var cardPadding: CGFloat {
        switch context {
        case .iPhoneCompact, .iPhoneLandscape:
            return 16
        case .iPadCompact:
            return 20
        case .iPadRegular:
            return 24
        case .macCompact:
            return 20
        case .macRegular:
            return 24
        }
    }

    /// Spacing between sections
    var sectionSpacing: CGFloat {
        switch context {
        case .iPhoneCompact, .iPhoneLandscape:
            return 20
        case .iPadCompact:
            return 24
        case .iPadRegular:
            return 28
        case .macCompact:
            return 24
        case .macRegular:
            return 32
        }
    }

    /// Minimum touch target size
    var minimumTouchTarget: CGFloat {
        switch context {
        case .iPhoneCompact, .iPhoneLandscape, .iPadCompact, .iPadRegular:
            return 48  // iOS HIG minimum
        case .macCompact, .macRegular:
            return 32  // macOS can be smaller (pointer precision)
        }
    }

    // MARK: - Layout Preferences

    /// Whether to use split view layout (sidebar + detail)
    var prefersSplitView: Bool {
        switch context {
        case .iPhoneCompact, .iPhoneLandscape:
            return false  // iPhone uses stack navigation
        case .iPadCompact, .iPadRegular, .macCompact, .macRegular:
            return true  // iPad and Mac use split view
        }
    }

    /// Whether to show persistent sidebar (vs overlay)
    var prefersSidebar: Bool {
        switch context {
        case .iPhoneCompact, .iPhoneLandscape, .iPadCompact:
            return false  // Overlay on iPhone and compact iPad
        case .iPadRegular, .macCompact, .macRegular:
            return true  // Persistent sidebar on regular iPad and Mac
        }
    }

    /// Preferred sidebar width
    var sidebarWidth: CGFloat {
        switch context {
        case .iPhoneCompact, .iPhoneLandscape:
            return 0  // No sidebar on iPhone
        case .iPadCompact:
            return 320  // Compact sidebar
        case .iPadRegular:
            return 360  // Comfortable sidebar
        case .macCompact:
            return 280
        case .macRegular:
            return 320
        }
    }

    /// Whether to show terminal and chat side-by-side
    var prefersSideBySideTerminal: Bool {
        switch context {
        case .iPhoneCompact, .iPhoneLandscape, .iPadCompact:
            return false  // Too narrow for side-by-side
        case .iPadRegular, .macCompact, .macRegular:
            return true  // Enough space for side-by-side
        }
    }

    /// Maximum content width for readability
    var maxContentWidth: CGFloat {
        switch context {
        case .iPhoneCompact, .iPhoneLandscape:
            return .infinity  // Use full width on iPhone
        case .iPadCompact:
            return 600
        case .iPadRegular:
            return 800
        case .macCompact:
            return 700
        case .macRegular:
            return 1000
        }
    }
}

// MARK: - Environment Key

private struct AdaptiveThemeKey: EnvironmentKey {
    static let defaultValue = AdaptiveTheme(
        horizontalSizeClass: .compact,
        verticalSizeClass: .regular
    )
}

extension EnvironmentValues {
    var adaptiveTheme: AdaptiveTheme {
        get { self[AdaptiveThemeKey.self] }
        set { self[AdaptiveThemeKey.self] = newValue }
    }
}

// MARK: - Adaptive Theme Host

/// Wrapper view that injects and updates adaptive theme based on size classes
struct AdaptiveThemeHost<Content: View>: View {
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @Environment(\.verticalSizeClass) private var verticalSizeClass
    @StateObject private var adaptiveTheme: AdaptiveTheme

    let content: Content

    init(@ViewBuilder content: () -> Content) {
        self.content = content()
        _adaptiveTheme = StateObject(wrappedValue: AdaptiveTheme(
            horizontalSizeClass: nil,
            verticalSizeClass: nil
        ))
    }

    var body: some View {
        content
            .environment(\.adaptiveTheme, adaptiveTheme)
            .onChange(of: horizontalSizeClass) { _, _ in
                updateTheme()
            }
            .onChange(of: verticalSizeClass) { _, _ in
                updateTheme()
            }
            .onAppear {
                updateTheme()
            }
    }

    private func updateTheme() {
        adaptiveTheme.update(
            horizontalSizeClass: horizontalSizeClass,
            verticalSizeClass: verticalSizeClass
        )
    }
}

// MARK: - View Extension

extension View {
    /// Wraps the view with adaptive theme support
    func withAdaptiveTheme() -> some View {
        AdaptiveThemeHost {
            self
        }
    }
}
