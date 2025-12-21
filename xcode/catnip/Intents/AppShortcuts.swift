//
//  AppShortcuts.swift
//  catnip
//
//  Provides Siri phrases for Catnip App Intents
//

import AppIntents

struct CatnipShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(
            intent: SendPromptIntent(),
            phrases: [
                "Send a prompt to Claude in \(.applicationName)",
                "Ask Claude something in \(.applicationName)",
                "Tell Claude in \(.applicationName)"
            ],
            shortTitle: "Send to Claude",
            systemImageName: "message"
        )

        AppShortcut(
            intent: CheckStatusIntent(),
            phrases: [
                "What's Claude working on in \(.applicationName)",
                "Check \(.applicationName) status",
                "What's happening in \(.applicationName)"
            ],
            shortTitle: "Check Status",
            systemImageName: "info.circle"
        )
    }
}
