//
//  KeychainHelperTests.swift
//  catnipTests
//
//  Tests for KeychainHelper functionality
//

import Testing
import Foundation
@testable import catnip

struct KeychainHelperTests {

    // MARK: - Save and Load Tests

    @Test func testSaveAndLoadString() async throws {
        let testKey = "test_key_\(UUID().uuidString)"
        let testValue = "test_value_123"

        // Save value
        try await KeychainHelper.save(key: testKey, value: testValue)

        // Load value
        let loadedValue = try await KeychainHelper.load(key: testKey)

        #expect(loadedValue == testValue)

        // Cleanup
        try? await KeychainHelper.delete(key: testKey)
    }

    @Test func testSaveOverwritesExisting() async throws {
        let testKey = "test_overwrite_\(UUID().uuidString)"
        let firstValue = "first_value"
        let secondValue = "second_value"

        // Save first value
        try await KeychainHelper.save(key: testKey, value: firstValue)

        // Overwrite with second value
        try await KeychainHelper.save(key: testKey, value: secondValue)

        // Load and verify second value
        let loadedValue = try await KeychainHelper.load(key: testKey)
        #expect(loadedValue == secondValue)

        // Cleanup
        try? await KeychainHelper.delete(key: testKey)
    }

    @Test func testLoadNonExistentKey() async {
        let testKey = "nonexistent_key_\(UUID().uuidString)"

        do {
            _ = try await KeychainHelper.load(key: testKey)
            Issue.record("Expected error when loading non-existent key")
        } catch {
            // Expected to throw
            #expect(error != nil)
        }
    }

    @Test func testSaveEmptyString() async throws {
        let testKey = "test_empty_\(UUID().uuidString)"
        let emptyValue = ""

        try await KeychainHelper.save(key: testKey, value: emptyValue)
        let loadedValue = try await KeychainHelper.load(key: testKey)

        #expect(loadedValue == emptyValue)

        // Cleanup
        try? await KeychainHelper.delete(key: testKey)
    }

    @Test func testSaveLongString() async throws {
        let testKey = "test_long_\(UUID().uuidString)"
        let longValue = String(repeating: "a", count: 10000)

        try await KeychainHelper.save(key: testKey, value: longValue)
        let loadedValue = try await KeychainHelper.load(key: testKey)

        #expect(loadedValue == longValue)

        // Cleanup
        try? await KeychainHelper.delete(key: testKey)
    }

    @Test func testSaveSpecialCharacters() async throws {
        let testKey = "test_special_\(UUID().uuidString)"
        let specialValue = "Hello! @#$%^&*() 你好 🚀"

        try await KeychainHelper.save(key: testKey, value: specialValue)
        let loadedValue = try await KeychainHelper.load(key: testKey)

        #expect(loadedValue == specialValue)

        // Cleanup
        try? await KeychainHelper.delete(key: testKey)
    }

    // MARK: - Delete Tests

    @Test func testDeleteExistingKey() async throws {
        let testKey = "test_delete_\(UUID().uuidString)"
        let testValue = "value_to_delete"

        // Save value
        try await KeychainHelper.save(key: testKey, value: testValue)

        // Delete
        try await KeychainHelper.delete(key: testKey)

        // Try to load - should fail
        do {
            _ = try await KeychainHelper.load(key: testKey)
            Issue.record("Expected error when loading deleted key")
        } catch {
            // Expected to throw
            #expect(error != nil)
        }
    }

    @Test func testDeleteNonExistentKey() async throws {
        let testKey = "nonexistent_delete_\(UUID().uuidString)"

        // Deleting non-existent key should not throw
        // (as per implementation: status == errSecSuccess || status == errSecItemNotFound)
        try await KeychainHelper.delete(key: testKey)
    }

    @Test func testDeleteTwice() async throws {
        let testKey = "test_delete_twice_\(UUID().uuidString)"
        let testValue = "value"

        // Save
        try await KeychainHelper.save(key: testKey, value: testValue)

        // Delete first time
        try await KeychainHelper.delete(key: testKey)

        // Delete second time - should not throw
        try await KeychainHelper.delete(key: testKey)
    }

    // MARK: - Multiple Keys Tests

    @Test func testMultipleKeysIndependent() async throws {
        let key1 = "test_multi_1_\(UUID().uuidString)"
        let key2 = "test_multi_2_\(UUID().uuidString)"
        let value1 = "value_one"
        let value2 = "value_two"

        // Save both
        try await KeychainHelper.save(key: key1, value: value1)
        try await KeychainHelper.save(key: key2, value: value2)

        // Load both
        let loaded1 = try await KeychainHelper.load(key: key1)
        let loaded2 = try await KeychainHelper.load(key: key2)

        #expect(loaded1 == value1)
        #expect(loaded2 == value2)

        // Delete one
        try await KeychainHelper.delete(key: key1)

        // Second should still exist
        let stillLoaded = try await KeychainHelper.load(key: key2)
        #expect(stillLoaded == value2)

        // Cleanup
        try? await KeychainHelper.delete(key: key2)
    }

    // MARK: - Realistic Use Cases

    @Test func testSessionTokenWorkflow() async throws {
        let tokenKey = "session_token_test_\(UUID().uuidString)"
        let usernameKey = "username_test_\(UUID().uuidString)"

        let token = "github_token_abc123xyz"
        let username = "testuser"

        // Save session
        try await KeychainHelper.save(key: tokenKey, value: token)
        try await KeychainHelper.save(key: usernameKey, value: username)

        // Load session
        let loadedToken = try await KeychainHelper.load(key: tokenKey)
        let loadedUsername = try await KeychainHelper.load(key: usernameKey)

        #expect(loadedToken == token)
        #expect(loadedUsername == username)

        // Clear session
        try await KeychainHelper.delete(key: tokenKey)
        try await KeychainHelper.delete(key: usernameKey)

        // Verify cleared
        do {
            _ = try await KeychainHelper.load(key: tokenKey)
            Issue.record("Token should be deleted")
        } catch {
            #expect(error != nil)
        }

        do {
            _ = try await KeychainHelper.load(key: usernameKey)
            Issue.record("Username should be deleted")
        } catch {
            #expect(error != nil)
        }
    }

    @Test func testOAuthStateWorkflow() async throws {
        let stateKey = "oauth_state_test_\(UUID().uuidString)"
        let state = UUID().uuidString

        // Save state
        try await KeychainHelper.save(key: stateKey, value: state)

        // Load state
        let loadedState = try await KeychainHelper.load(key: stateKey)
        #expect(loadedState == state)

        // Verify state matches (CSRF protection)
        #expect(loadedState == state)

        // Cleanup after OAuth
        try await KeychainHelper.delete(key: stateKey)
    }

    // MARK: - UTF-8 Encoding Tests

    @Test func testUTF8Encoding() async throws {
        let testKey = "test_utf8_\(UUID().uuidString)"
        let testValues = [
            "Hello World",
            "مرحبا",  // Arabic
            "こんにちは",  // Japanese
            "안녕하세요",  // Korean
            "Здравствуйте",  // Russian
            "🎉🎊🎈"  // Emojis
        ]

        for testValue in testValues {
            try await KeychainHelper.save(key: testKey, value: testValue)
            let loadedValue = try await KeychainHelper.load(key: testKey)
            #expect(loadedValue == testValue)
        }

        // Cleanup
        try? await KeychainHelper.delete(key: testKey)
    }
}
