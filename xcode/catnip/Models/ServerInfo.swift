//
//  ServerInfo.swift
//  catnip
//
//  Server information model
//

import Foundation

struct ServerInfo: Codable {
    let version: String
    let build: BuildInfo?

    struct BuildInfo: Codable {
        let commit: String
        let date: String
        let builtBy: String

        enum CodingKeys: String, CodingKey {
            case commit
            case date
            case builtBy = "builtBy"
        }
    }
}
