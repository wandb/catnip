//
//  CodespaceInfo.swift
//  catnip
//
//  Data model for GitHub Codespace information
//

import Foundation

struct CodespaceInfo: Codable, Identifiable, Hashable {
    let id = UUID()
    let name: String
    let lastUsed: TimeInterval
    let repository: String?

    var displayName: String {
        name.replacingOccurrences(of: "-", with: " ")
    }

    var lastUsedDate: Date {
        Date(timeIntervalSince1970: lastUsed / 1000) // Convert from milliseconds
    }

    enum CodingKeys: String, CodingKey {
        case name = "name"
        case lastUsed = "lastUsed"
        case repository = "repository"
    }
}
