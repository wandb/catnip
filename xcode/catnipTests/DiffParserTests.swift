//
//  DiffParserTests.swift
//  catnipTests
//
//  Tests for DiffParser logic
//

import Testing
import Foundation
@testable import Catnip

struct DiffParserTests {

    // MARK: - Basic Parsing Tests

    @Test func testParseSimpleAddition() {
        let diffText = """
        @@ -1,3 +1,4 @@
         line1
         line2
        +new line
         line3
        """

        let lines = DiffParser.parse(diffText)

        // Should have: header, 3 context lines, 1 addition
        #expect(lines.count == 5)
        #expect(lines[0].type == .header)
        #expect(lines[1].type == .context)
        #expect(lines[2].type == .context)
        #expect(lines[3].type == .add)
        #expect(lines[3].content == "new line")
        #expect(lines[4].type == .context)
    }

    @Test func testParseSimpleDeletion() {
        let diffText = """
        @@ -1,4 +1,3 @@
         line1
         line2
        -removed line
         line3
        """

        let lines = DiffParser.parse(diffText)

        #expect(lines.count == 5)
        #expect(lines[0].type == .header)
        #expect(lines[1].type == .context)
        #expect(lines[2].type == .context)
        #expect(lines[3].type == .remove)
        #expect(lines[3].content == "removed line")
        #expect(lines[4].type == .context)
    }

    @Test func testParseModification() {
        let diffText = """
        @@ -1,3 +1,3 @@
         line1
        -old line
        +new line
         line3
        """

        let lines = DiffParser.parse(diffText)

        #expect(lines.count == 5)
        #expect(lines[1].type == .context)
        #expect(lines[2].type == .remove)
        #expect(lines[2].content == "old line")
        #expect(lines[3].type == .add)
        #expect(lines[3].content == "new line")
        #expect(lines[4].type == .context)
    }

    // MARK: - Line Number Tests

    @Test func testLineNumbersForAdditions() {
        let diffText = """
        @@ -1,2 +1,3 @@
         line1
        +added
         line2
        """

        let lines = DiffParser.parse(diffText)

        // First context line
        #expect(lines[1].oldLineNumber == 1)
        #expect(lines[1].newLineNumber == 1)

        // Addition (only new line number)
        #expect(lines[2].oldLineNumber == nil)
        #expect(lines[2].newLineNumber == 2)

        // Second context line
        #expect(lines[3].oldLineNumber == 2)
        #expect(lines[3].newLineNumber == 3)
    }

    @Test func testLineNumbersForDeletions() {
        let diffText = """
        @@ -1,3 +1,2 @@
         line1
        -deleted
         line2
        """

        let lines = DiffParser.parse(diffText)

        // First context line
        #expect(lines[1].oldLineNumber == 1)
        #expect(lines[1].newLineNumber == 1)

        // Deletion (only old line number)
        #expect(lines[2].oldLineNumber == 2)
        #expect(lines[2].newLineNumber == nil)

        // Second context line
        #expect(lines[3].oldLineNumber == 3)
        #expect(lines[3].newLineNumber == 2)
    }

    // MARK: - Complex Diff Tests

    @Test func testParseMultipleHunks() {
        let diffText = """
        @@ -1,3 +1,3 @@
         line1
        -old
        +new
         line3
        @@ -10,2 +10,3 @@
         line10
        +added
         line11
        """

        let lines = DiffParser.parse(diffText)

        // Should parse both hunks
        #expect(lines.count == 9)

        // First hunk header
        #expect(lines[0].type == .header)

        // Second hunk header
        #expect(lines[5].type == .header)

        // Verify line numbers reset for second hunk
        #expect(lines[6].oldLineNumber == 10)
        #expect(lines[6].newLineNumber == 10)
    }

    @Test func testParseRealWorldDiff() {
        let diffText = """
        @@ -1,8 +1,12 @@
         import React from 'react';
        +import { cn } from '@/lib/utils';

         interface ButtonProps {
           label: string;
        -  onClick: () => void;
        +  onClick?: () => void;
        +  variant?: 'primary' | 'secondary';
        +  disabled?: boolean;
         }
        """

        let lines = DiffParser.parse(diffText)

        // Verify we correctly parse a realistic diff
        #expect(lines.count > 5)

        // Find the import addition
        let importAddition = lines.first { $0.type == .add && $0.content.contains("cn") }
        #expect(importAddition != nil)

        // Find the onClick modification
        let onClickRemoval = lines.first { $0.type == .remove && $0.content.contains("onClick: () => void") }
        #expect(onClickRemoval != nil)
    }

    @Test func testParseEmptyDiff() {
        let diffText = ""
        let lines = DiffParser.parse(diffText)
        #expect(lines.count == 1) // Just one empty line
    }

    @Test func testParseDiffWithFileHeaders() {
        let diffText = """
        --- a/file.txt
        +++ b/file.txt
        @@ -1,2 +1,2 @@
         line1
        -old
        +new
        """

        let lines = DiffParser.parse(diffText)

        // File headers (--- and +++) should be filtered out
        #expect(lines[0].type == .header) // The @@ line
        #expect(lines[1].type == .context)
    }

    // MARK: - Stats Calculation Tests

    @Test func testCalculateStatsSimple() {
        let diffText = """
        @@ -1,3 +1,4 @@
         line1
        +added line
         line2
        -removed line
         line3
        """

        let stats = DiffParser.calculateStats(diffText)

        #expect(stats.additions == 1)
        #expect(stats.deletions == 1)
        #expect(stats.totalChanges == 2)
    }

    @Test func testCalculateStatsMultipleAdditions() {
        let diffText = """
        @@ -1,2 +1,5 @@
         line1
        +added1
        +added2
        +added3
         line2
        """

        let stats = DiffParser.calculateStats(diffText)

        #expect(stats.additions == 3)
        #expect(stats.deletions == 0)
        #expect(stats.totalChanges == 3)
    }

    @Test func testCalculateStatsMultipleDeletions() {
        let diffText = """
        @@ -1,5 +1,2 @@
         line1
        -removed1
        -removed2
        -removed3
         line2
        """

        let stats = DiffParser.calculateStats(diffText)

        #expect(stats.additions == 0)
        #expect(stats.deletions == 3)
        #expect(stats.totalChanges == 3)
    }

    @Test func testCalculateStatsIgnoresFileHeaders() {
        let diffText = """
        --- a/file.txt
        +++ b/file.txt
        @@ -1,2 +1,2 @@
         line1
        +added
        """

        let stats = DiffParser.calculateStats(diffText)

        // Should not count file headers (+++ and ---) as additions/deletions
        #expect(stats.additions == 1)
        #expect(stats.deletions == 0)
    }

    @Test func testCalculateStatsEmptyDiff() {
        let diffText = ""
        let stats = DiffParser.calculateStats(diffText)

        #expect(stats.additions == 0)
        #expect(stats.deletions == 0)
        #expect(stats.totalChanges == 0)
    }

    @Test func testCalculateStatsLargeChange() {
        var diffLines = ["@@ -1,100 +1,100 @@"]
        for i in 1...50 {
            diffLines.append("+added line \(i)")
        }
        for i in 1...30 {
            diffLines.append("-removed line \(i)")
        }
        let diffText = diffLines.joined(separator: "\n")

        let stats = DiffParser.calculateStats(diffText)

        #expect(stats.additions == 50)
        #expect(stats.deletions == 30)
        #expect(stats.totalChanges == 80)
    }

    // MARK: - Edge Cases

    @Test func testParseDiffWithOnlyContextLines() {
        let diffText = """
        @@ -1,3 +1,3 @@
         line1
         line2
         line3
        """

        let lines = DiffParser.parse(diffText)

        #expect(lines.count == 4)
        #expect(lines.allSatisfy { $0.type == .header || $0.type == .context })
    }

    @Test func testParseDiffWithEmptyLines() {
        let diffText = """
        @@ -1,4 +1,4 @@
         line1
        +
         line2
        -
        """

        let lines = DiffParser.parse(diffText)

        // Should handle empty additions and deletions
        #expect(lines.count == 5)
        let addedEmpty = lines.first { $0.type == .add && $0.content.isEmpty }
        #expect(addedEmpty != nil)
    }

    @Test func testParseDiffWithSpecialCharacters() {
        let diffText = """
        @@ -1,2 +1,2 @@
        -const foo = "bar";
        +const foo = "baz & qux";
        """

        let lines = DiffParser.parse(diffText)

        #expect(lines[1].type == .remove)
        #expect(lines[1].content.contains("\"bar\""))
        #expect(lines[2].type == .add)
        #expect(lines[2].content.contains("&"))
    }
}
