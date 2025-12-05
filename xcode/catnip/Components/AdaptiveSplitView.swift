//
//  AdaptiveSplitView.swift
//  catnip
//
//  Adaptive split view: single pane with toggle on iPhone, side-by-side on iPad
//

import SwiftUI

/// Display mode for split content
enum AdaptiveSplitMode {
    case leading    // Show only leading content
    case trailing   // Show only trailing content
    case split      // Show both side-by-side
}

/// Adaptive split view that shows single pane on iPhone and side-by-side on iPad
struct AdaptiveSplitView<Leading: View, Trailing: View>: View {
    @Environment(\.adaptiveTheme) private var adaptiveTheme

    let defaultMode: AdaptiveSplitMode
    let allowModeToggle: Bool
    let contextTokens: Int64?
    let leading: () -> Leading
    let trailing: () -> Trailing

    @State private var currentMode: AdaptiveSplitMode

    init(
        defaultMode: AdaptiveSplitMode = .split,
        allowModeToggle: Bool = true,
        contextTokens: Int64? = nil,
        @ViewBuilder leading: @escaping () -> Leading,
        @ViewBuilder trailing: @escaping () -> Trailing
    ) {
        self.defaultMode = defaultMode
        self.allowModeToggle = allowModeToggle
        self.contextTokens = contextTokens
        self.leading = leading
        self.trailing = trailing
        _currentMode = State(initialValue: defaultMode)
    }

    var body: some View {
        Group {
            if adaptiveTheme.context.isPhone {
                // iPhone: Single pane with toggle
                singlePaneLayout
            } else {
                // iPad (both portrait and landscape): Vertical split (top/bottom)
                // Terminal on top, chat on bottom for better code visibility
                verticalSplitLayout
            }
        }
        .onChange(of: adaptiveTheme.context) { _, _ in
            // Reset to default mode when context changes
            currentMode = defaultMode
        }
    }

    // MARK: - Horizontal Split Layout (iPad Landscape/Mac)

    private var horizontalSplitLayout: some View {
        HStack(spacing: 0) {
            if currentMode == .leading || currentMode == .split {
                leading()
                    .frame(maxWidth: currentMode == .split ? adaptiveTheme.maxContentWidth : .infinity)

                if currentMode == .split {
                    Divider()
                }
            }

            if currentMode == .trailing || currentMode == .split {
                trailing()
                    .frame(maxWidth: .infinity)
            }
        }
        .toolbar {
            if allowModeToggle {
                ToolbarItem(placement: .topBarTrailing) {
                    modeToggleMenu
                }
            }
        }
    }

    // MARK: - Vertical Split Layout (iPad Portrait)

    private var verticalSplitLayout: some View {
        VStack(spacing: 0) {
            if currentMode == .leading || currentMode == .split {
                leading()
                    .frame(maxHeight: currentMode == .split ? .infinity : .infinity)

                if currentMode == .split {
                    Divider()
                }
            }

            if currentMode == .trailing || currentMode == .split {
                trailing()
                    .frame(maxHeight: .infinity)
            }
        }
        .toolbar {
            if allowModeToggle {
                ToolbarItem(placement: .topBarTrailing) {
                    modeToggleMenu
                }
            }
        }
    }

    // MARK: - Single Pane Layout (iPhone)

    private var singlePaneLayout: some View {
        ZStack {
            if currentMode == .leading || currentMode == .split {
                leading()
                    .opacity(currentMode == .leading ? 1 : 0)
                    .zIndex(currentMode == .leading ? 1 : 0)
            }

            if currentMode == .trailing || currentMode == .split {
                trailing()
                    .opacity(currentMode == .trailing ? 1 : 0)
                    .zIndex(currentMode == .trailing ? 1 : 0)
            }
        }
        .toolbar {
            if allowModeToggle {
                ToolbarItem(placement: .topBarTrailing) {
                    simpleToggleButton
                }
            }
        }
    }

    // MARK: - Controls

    private var modeToggleMenu: some View {
        Menu {
            Button {
                withAnimation(.easeInOut(duration: 0.2)) {
                    currentMode = .split
                }
            } label: {
                Label("Split View", systemImage: "rectangle.split.2x1")
            }

            Button {
                withAnimation(.easeInOut(duration: 0.2)) {
                    currentMode = .leading
                }
            } label: {
                Label("Terminal Only", systemImage: "rectangle.leadinghalf.filled")
            }

            Button {
                withAnimation(.easeInOut(duration: 0.2)) {
                    currentMode = .trailing
                }
            } label: {
                Label("Overview Only", systemImage: "rectangle.trailinghalf.filled")
            }
        } label: {
            ContextProgressRing(contextTokens: contextTokens) {
                Image(systemName: currentMode == .split ? "rectangle.split.2x1" :
                      currentMode == .leading ? "rectangle.leadinghalf.filled" :
                      "rectangle.trailinghalf.filled")
                    .font(.system(size: 11, weight: .medium))
            }
        }
    }

    private var simpleToggleButton: some View {
        Button {
            withAnimation(.easeInOut(duration: 0.2)) {
                // Simple toggle between leading and trailing on iPhone
                if currentMode == .leading {
                    currentMode = .trailing
                } else {
                    currentMode = .leading
                }
            }
        } label: {
            Image(systemName: currentMode == .leading ? "rectangle.leadinghalf.filled" : "rectangle.trailinghalf.filled")
        }
    }
}

// MARK: - Preview

#Preview("iPhone") {
    NavigationStack {
        AdaptiveSplitView {
            VStack {
                Text("Leading Content")
                    .font(.largeTitle)
                List(0..<20) { index in
                    Text("Leading Item \(index)")
                }
            }
        } trailing: {
            VStack {
                Text("Trailing Content")
                    .font(.largeTitle)
                List(0..<20) { index in
                    Text("Trailing Item \(index)")
                }
            }
        }
        .navigationTitle("Split View")
    }
    .environment(\.adaptiveTheme, AdaptiveTheme(
        horizontalSizeClass: .compact,
        verticalSizeClass: .regular,
        idiom: .phone
    ))
}

#Preview("iPad") {
    NavigationStack {
        AdaptiveSplitView {
            VStack {
                Text("Leading Content")
                    .font(.largeTitle)
                List(0..<20) { index in
                    Text("Leading Item \(index)")
                }
            }
        } trailing: {
            VStack {
                Text("Trailing Content")
                    .font(.largeTitle)
                List(0..<20) { index in
                    Text("Trailing Item \(index)")
                }
            }
        }
        .navigationTitle("Split View")
    }
    .environment(\.adaptiveTheme, AdaptiveTheme(
        horizontalSizeClass: .regular,
        verticalSizeClass: .regular,
        idiom: .pad
    ))
}
