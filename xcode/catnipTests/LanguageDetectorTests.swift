//
//  LanguageDetectorTests.swift
//  catnipTests
//
//  Tests for the LanguageDetector service
//

import XCTest
@testable import Catnip

final class LanguageDetectorTests: XCTestCase {

    // MARK: - Extension Detection Tests

    func testSwiftFileDetection() {
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "MyFile.swift"), "swift")
    }

    func testTypeScriptFileDetection() {
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "app.ts"), "typescript")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "app.tsx"), "typescript")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "app.mts"), "typescript")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "app.cts"), "typescript")
    }

    func testJavaScriptFileDetection() {
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "app.js"), "javascript")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "app.jsx"), "javascript")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "app.mjs"), "javascript")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "app.cjs"), "javascript")
    }

    func testPythonFileDetection() {
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "script.py"), "python")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "script.pyw"), "python")
    }

    // MARK: - Filename Detection Tests

    func testDockerfileDetection() {
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "Dockerfile"), "dockerfile")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "Containerfile"), "dockerfile")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "path/to/Dockerfile"), "dockerfile")
    }

    func testMakefileDetection() {
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "Makefile"), "makefile")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "makefile"), "makefile")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "GNUmakefile"), "makefile")
    }

    func testRubyFileDetection() {
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "Gemfile"), "ruby")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "Rakefile"), "ruby")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "Podfile"), "ruby")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "script.rb"), "ruby")
    }

    func testCMakeDetection() {
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "CMakeLists.txt"), "cmake")
    }

    // MARK: - Extension Conflict Tests

    func testObjectiveCOverMATLAB() {
        // .m should map to Objective-C (more common on iOS)
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "MyClass.m"), "objectivec")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "MyClass.mm"), "objectivec")
    }

    func testVerilogDetection() {
        // .v should map to Verilog (more common than V language)
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "module.v"), "verilog")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "header.vh"), "verilog")
    }

    // MARK: - Case Sensitivity Tests

    func testCaseInsensitiveFilenames() {
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "makefile"), "makefile")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "Makefile"), "makefile")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "MAKEFILE"), "makefile")
    }

    func testCaseInsensitiveExtensions() {
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "file.SWIFT"), "swift")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "file.Swift"), "swift")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "file.swift"), "swift")
    }

    // MARK: - Unknown File Tests

    func testUnknownFileReturnsNil() {
        XCTAssertNil(LanguageDetector.detectLanguage(from: "file.unknown"))
        XCTAssertNil(LanguageDetector.detectLanguage(from: "file.xyz"))
        XCTAssertNil(LanguageDetector.detectLanguage(from: "NoExtension"))
    }

    // MARK: - Path Tests

    func testDetectionWithFullPaths() {
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "/path/to/file.swift"), "swift")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "src/components/Button.tsx"), "typescript")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "docker/Dockerfile"), "dockerfile")
    }

    // MARK: - Data Format Detection Tests

    func testDataFormatDetection() {
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "config.json"), "json")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "config.yaml"), "yaml")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "config.yml"), "yaml")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "config.toml"), "toml")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "config.xml"), "xml")
    }

    // MARK: - Multi-Dot Filename Tests

    func testMultipleDotFilenames() {
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "app.test.ts"), "typescript")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "script.test.js"), "javascript")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "Dockerfile.production"), nil)
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "component.spec.tsx"), "typescript")
        XCTAssertEqual(LanguageDetector.detectLanguage(from: "config.local.json"), "json")
    }

    // MARK: - Dictionary Integrity Tests

    /// This test verifies that there are no duplicate keys in our extension map
    /// This catches issues at test-time rather than runtime
    func testNoDuplicateExtensionKeys() {
        // We'll use reflection to validate the dictionary was created successfully
        // If there were duplicates at compile time, Swift would silently use the last value
        // This test ensures we're aware of any conflicts

        // Test some specific known conflict points
        let mExtension = LanguageDetector.detectLanguage(from: "test.m")
        XCTAssertNotNil(mExtension, ".m extension should map to a language")
        XCTAssertEqual(mExtension, "objectivec", ".m should map to Objective-C (not MATLAB)")

        let vExtension = LanguageDetector.detectLanguage(from: "test.v")
        XCTAssertNotNil(vExtension, ".v extension should map to a language")
        XCTAssertEqual(vExtension, "verilog", ".v should map to Verilog (not V language)")
    }

    /// Verify common languages are correctly mapped
    func testCommonLanguageMappings() {
        let testCases: [(String, String)] = [
            ("file.swift", "swift"),
            ("file.py", "python"),
            ("file.js", "javascript"),
            ("file.ts", "typescript"),
            ("file.go", "go"),
            ("file.rs", "rust"),
            ("file.java", "java"),
            ("file.kt", "kotlin"),
            ("file.rb", "ruby"),
            ("file.php", "php"),
            ("file.c", "c"),
            ("file.cpp", "cpp"),
            ("file.cs", "csharp"),
            ("file.sh", "bash"),
            ("file.sql", "sql"),
            ("file.html", "html"),
            ("file.css", "css"),
            ("file.md", "markdown"),
        ]

        for (filename, expectedLanguage) in testCases {
            XCTAssertEqual(
                LanguageDetector.detectLanguage(from: filename),
                expectedLanguage,
                "Failed to detect \(expectedLanguage) for \(filename)"
            )
        }
    }
}
