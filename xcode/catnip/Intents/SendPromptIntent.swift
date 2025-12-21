//
//  SendPromptIntent.swift
//  catnip
//
//  App Intent for sending prompts to Claude via Siri
//

import AppIntents
import Foundation

struct SendPromptIntent: AppIntent {
    static var title: LocalizedStringResource = "Send Prompt to Claude"
    static var description = IntentDescription("Send a coding prompt to your Claude agent")

    @Parameter(title: "Prompt")
    var prompt: String

    static var parameterSummary: some ParameterSummary {
        Summary("Tell Claude to \(\.$prompt)")
    }

    func perform() async throws -> some IntentResult & ProvidesDialog {
        // Check if user is authenticated
        guard let token = try? await KeychainHelper.load(key: "session_token") else {
            return .result(dialog: "You need to sign in to Catnip first.")
        }

        // Get device token for notifications
        let deviceToken = UserDefaults.standard.string(forKey: "apnsDeviceToken")

        // Build request
        guard let url = URL(string: "https://catnip.run/v1/siri/prompt") else {
            return .result(dialog: "Failed to send prompt. Please try again.")
        }

        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")

        let body: [String: Any] = [
            "prompt": prompt,
            "deviceToken": deviceToken ?? ""
        ]

        request.httpBody = try? JSONSerialization.data(withJSONObject: body)

        do {
            let (_, response) = try await URLSession.shared.data(for: request)

            guard let httpResponse = response as? HTTPURLResponse,
                  httpResponse.statusCode == 200 else {
                return .result(dialog: "Failed to send prompt. Please try again.")
            }

            return .result(dialog: "Sending to Claude. I'll notify you when there's a response.")
        } catch {
            return .result(dialog: "Couldn't reach Catnip. Check your connection and try again.")
        }
    }
}
