//
//  catnipApp.swift
//  catnip
//
//  Created by CVP on 10/5/25.
//

import SwiftUI

@main
struct catnipApp: App {
    @StateObject private var authManager = AuthManager()

    init() {
        // Disable animations during UI testing for faster tests
        if UITestingHelper.shouldDisableAnimations {
            UIView.setAnimationsEnabled(false)
        }
    }

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(authManager)
        }
    }
}
