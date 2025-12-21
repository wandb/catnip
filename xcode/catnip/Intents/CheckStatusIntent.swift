//
//  CheckStatusIntent.swift
//  catnip
//
//  App Intent for checking Claude workspace status via Siri
//

import AppIntents
import Foundation

struct CheckStatusIntent: AppIntent {
    static var title: LocalizedStringResource = "Check Claude Status"
    static var description = IntentDescription("Check what Claude is working on")

    func perform() async throws -> some IntentResult & ProvidesDialog {
        // Check if user is authenticated
        guard let token = try? await KeychainHelper.load(key: "session_token") else {
            return .result(dialog: "You need to sign in to Catnip first.")
        }

        // Build request
        guard let url = URL(string: "https://catnip.run/v1/siri/status") else {
            return .result(dialog: "Something went wrong.")
        }

        var request = URLRequest(url: url)
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        request.timeoutInterval = 15.0

        do {
            let (data, response) = try await URLSession.shared.data(for: request)

            guard let httpResponse = response as? HTTPURLResponse,
                  httpResponse.statusCode == 200 else {
                return .result(dialog: "Couldn't get status. Your codespace may be offline.")
            }

            if let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
               let message = json["message"] as? String {
                return .result(dialog: "\(message)")
            }

            return .result(dialog: "Couldn't get status.")
        } catch {
            return .result(dialog: "Couldn't reach Catnip. Check your connection.")
        }
    }
}
