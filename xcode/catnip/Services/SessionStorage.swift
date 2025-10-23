//
//  SessionStorage.swift
//  catnip
//
//  Session-scoped storage that resets on app restart
//

import Foundation

/// Session-scoped storage that resets when the app restarts
/// Used for temporary state like Claude auth sheet dismissals
class SessionStorage {
    static let shared = SessionStorage()

    // In-memory storage that gets cleared on app restart
    private var storage: [String: Any] = [:]

    private init() {}

    /// Store a value for a scoped key
    func set(_ value: Any, forKey key: String, scope: String? = nil) {
        let scopedKey = makeScopedKey(key, scope: scope)
        storage[scopedKey] = value
    }

    /// Retrieve a value for a scoped key
    func get<T>(forKey key: String, scope: String? = nil) -> T? {
        let scopedKey = makeScopedKey(key, scope: scope)
        return storage[scopedKey] as? T
    }

    /// Remove a value for a scoped key
    func remove(forKey key: String, scope: String? = nil) {
        let scopedKey = makeScopedKey(key, scope: scope)
        storage.removeValue(forKey: scopedKey)
    }

    /// Clear all storage
    func clear() {
        storage.removeAll()
    }

    /// Create a scoped key by combining base key with optional scope
    private func makeScopedKey(_ key: String, scope: String?) -> String {
        if let scope = scope, !scope.isEmpty {
            return "\(key)-\(scope)"
        }
        return key
    }
}
