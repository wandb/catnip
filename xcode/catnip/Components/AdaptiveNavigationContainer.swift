//
//  AdaptiveNavigationContainer.swift
//  catnip
//
//  Adaptive navigation: NavigationStack on iPhone, NavigationSplitView on iPad
//

import SwiftUI

/// Adaptive navigation container that uses stack navigation on iPhone and split view on iPad
struct AdaptiveNavigationContainer<Sidebar: View, Detail: View, EmptyDetail: View>: View {
    @Environment(\.adaptiveTheme) private var adaptiveTheme

    let sidebar: () -> Sidebar
    let detail: (Binding<NavigationPath>) -> Detail
    let emptyDetail: () -> EmptyDetail

    @State private var navigationPath = NavigationPath()
    @State private var columnVisibility: NavigationSplitViewVisibility = .all

    init(
        @ViewBuilder sidebar: @escaping () -> Sidebar,
        @ViewBuilder detail: @escaping (Binding<NavigationPath>) -> Detail,
        @ViewBuilder emptyDetail: @escaping () -> EmptyDetail
    ) {
        self.sidebar = sidebar
        self.detail = detail
        self.emptyDetail = emptyDetail
    }

    var body: some View {
        if adaptiveTheme.prefersSplitView {
            // iPad/Mac: Use NavigationSplitView with sidebar + detail
            NavigationSplitView(columnVisibility: $columnVisibility) {
                NavigationStack {
                    sidebar()
                }
                .navigationSplitViewColumnWidth(adaptiveTheme.sidebarWidth)
            } detail: {
                NavigationStack(path: $navigationPath) {
                    detail($navigationPath)
                }
            }
        } else {
            // iPhone: Use NavigationStack with full-screen push
            NavigationStack(path: $navigationPath) {
                sidebar()
                    .navigationDestination(for: String.self) { _ in
                        detail($navigationPath)
                    }
            }
        }
    }
}

// MARK: - Preview

#Preview("iPhone") {
    AdaptiveNavigationContainer {
        List(0..<10) { index in
            NavigationLink("Item \(index)", value: "item-\(index)")
        }
        .navigationTitle("Sidebar")
    } detail: { _ in
        Text("Detail View")
            .navigationTitle("Detail")
    } emptyDetail: {
        VStack {
            Image(systemName: "square.stack")
                .font(.largeTitle)
                .foregroundStyle(.secondary)
            Text("Select an item")
                .font(.headline)
        }
    }
    .environment(\.adaptiveTheme, AdaptiveTheme(
        horizontalSizeClass: .compact,
        verticalSizeClass: .regular,
        idiom: .phone
    ))
}

#Preview("iPad") {
    AdaptiveNavigationContainer {
        List(0..<10) { index in
            NavigationLink("Item \(index)", value: "item-\(index)")
        }
        .navigationTitle("Sidebar")
    } detail: { _ in
        Text("Detail View")
            .navigationTitle("Detail")
    } emptyDetail: {
        VStack {
            Image(systemName: "square.stack")
                .font(.largeTitle)
                .foregroundStyle(.secondary)
            Text("Select an item")
                .font(.headline)
        }
    }
    .environment(\.adaptiveTheme, AdaptiveTheme(
        horizontalSizeClass: .regular,
        verticalSizeClass: .regular,
        idiom: .pad
    ))
}
