//
//  ContentView.swift
//  catnip
//
//  Created by CVP on 10/5/25.
//

import SwiftUI

struct ContentView: View {
    @EnvironmentObject var authManager: AuthManager

    var body: some View {
        NavigationStack {
            if authManager.isLoading {
                // Splash/Loading screen
                ZStack {
                    AppTheme.Colors.Background.grouped
                        .ignoresSafeArea()

                    VStack(spacing: AppTheme.Spacing.md) {
                        ProgressView()
                            .scaleEffect(1.5)
                            .tint(AppTheme.Colors.Brand.primary)
                    }
                }
            } else if authManager.isAuthenticated || authManager.isPreviewMode {
                // Main app flow: Codespace -> Workspaces -> Workspace Detail
                // Also show main app when in preview mode
                CodespaceView()
            } else {
                // Auth flow
                AuthView()
            }
        }
    }
}

#Preview {
    ContentView()
        .environmentObject(AuthManager())
}
