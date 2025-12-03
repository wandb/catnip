//
//  LanguageDetector.swift
//  catnip
//
//  Language detection service based on GitHub Linguist data
//  Detects programming language from file paths using extensions and filenames
//

import Foundation

/// Service for detecting programming languages from file paths
/// Based on GitHub Linguist: https://github.com/github-linguist/linguist
struct LanguageDetector {

    /// Detect language from a file path
    /// - Parameter filePath: The file path to analyze
    /// - Returns: Language identifier suitable for syntax highlighting, or nil if unknown
    static func detectLanguage(from filePath: String) -> String? {
        let filename = (filePath as NSString).lastPathComponent
        let ext = (filePath as NSString).pathExtension.lowercased()

        // Check for exact filename matches first (e.g., "Dockerfile", "Makefile")
        if let language = filenameMap[filename] {
            return language
        }

        // Check for case-insensitive filename matches
        if let language = filenameMap[filename.lowercased()] {
            return language
        }

        // Check for extension matches
        if !ext.isEmpty, let language = extensionMap[".\(ext)"] {
            return language
        }

        return nil
    }

    // MARK: - Language Mappings

    /// Map of exact filenames to language identifiers
    /// Based on GitHub Linguist filenames field
    private static let filenameMap: [String: String] = [
        // Docker
        "Dockerfile": "dockerfile",
        "Containerfile": "dockerfile",

        // Make
        "Makefile": "makefile",
        "makefile": "makefile",
        "GNUmakefile": "makefile",

        // CMake
        "CMakeLists.txt": "cmake",

        // Git
        ".gitignore": "gitignore",
        ".gitattributes": "gitattributes",
        ".gitmodules": "gitconfig",

        // Ruby/Rake
        "Rakefile": "ruby",
        "Gemfile": "ruby",
        "Podfile": "ruby",
        "Vagrantfile": "ruby",

        // JavaScript/Node
        "Gruntfile": "javascript",
        "Gulpfile": "javascript",

        // Python
        "Pipfile": "toml",

        // Other
        "BUILD": "python",  // Bazel
        "WORKSPACE": "python",  // Bazel
    ]

    /// Map of file extensions to language identifiers
    /// Based on GitHub Linguist extensions field, mapped to Highlightr language names
    private static let extensionMap: [String: String] = [
        // Swift
        ".swift": "swift",

        // JavaScript/TypeScript
        ".js": "javascript",
        ".mjs": "javascript",
        ".cjs": "javascript",
        ".jsx": "javascript",
        ".ts": "typescript",
        ".mts": "typescript",
        ".cts": "typescript",
        ".tsx": "typescript",

        // Web
        ".html": "html",
        ".htm": "html",
        ".xhtml": "xml",
        ".css": "css",
        ".scss": "scss",
        ".sass": "scss",
        ".less": "less",

        // Python
        ".py": "python",
        ".pyw": "python",
        ".pyx": "python",

        // Ruby
        ".rb": "ruby",
        ".rake": "ruby",

        // Java/Kotlin
        ".java": "java",
        ".kt": "kotlin",
        ".kts": "kotlin",

        // Go
        ".go": "go",

        // Rust
        ".rs": "rust",

        // C/C++
        ".c": "c",
        ".h": "c",
        ".cpp": "cpp",
        ".cc": "cpp",
        ".cxx": "cpp",
        ".c++": "cpp",
        ".hpp": "cpp",
        ".hh": "cpp",
        ".hxx": "cpp",
        ".h++": "cpp",

        // C#
        ".cs": "csharp",
        ".csx": "csharp",

        // Objective-C (Note: .m conflicts with MATLAB, prioritizing Objective-C on iOS)
        ".m": "objectivec",
        ".mm": "objectivec",

        // PHP
        ".php": "php",
        ".phtml": "php",
        ".php3": "php",
        ".php4": "php",
        ".php5": "php",

        // Shell
        ".sh": "bash",
        ".bash": "bash",
        ".zsh": "zsh",
        ".fish": "fish",

        // PowerShell
        ".ps1": "powershell",
        ".psm1": "powershell",
        ".psd1": "powershell",

        // Perl
        ".pl": "perl",
        ".pm": "perl",

        // Lua
        ".lua": "lua",

        // R
        ".r": "r",
        ".R": "r",

        // Scala
        ".scala": "scala",
        ".sc": "scala",

        // Clojure
        ".clj": "clojure",
        ".cljs": "clojure",
        ".cljc": "clojure",

        // Elixir
        ".ex": "elixir",
        ".exs": "elixir",

        // Erlang
        ".erl": "erlang",
        ".hrl": "erlang",

        // Haskell
        ".hs": "haskell",
        ".lhs": "haskell",

        // OCaml
        ".ml": "ocaml",
        ".mli": "ocaml",

        // F#
        ".fs": "fsharp",
        ".fsi": "fsharp",
        ".fsx": "fsharp",

        // Dart
        ".dart": "dart",

        // Dockerfile
        ".dockerfile": "dockerfile",
        ".containerfile": "dockerfile",

        // SQL
        ".sql": "sql",

        // Data formats
        ".json": "json",
        ".xml": "xml",
        ".yaml": "yaml",
        ".yml": "yaml",
        ".toml": "toml",
        ".ini": "ini",
        ".cfg": "ini",
        ".conf": "nginx",

        // Markdown
        ".md": "markdown",
        ".markdown": "markdown",
        ".mdown": "markdown",
        ".mkd": "markdown",
        ".mkdown": "markdown",

        // reStructuredText
        ".rst": "rst",

        // LaTeX
        ".tex": "latex",

        // CSV
        ".csv": "plaintext",

        // Protocol Buffers
        ".proto": "protobuf",

        // GraphQL
        ".graphql": "graphql",
        ".gql": "graphql",

        // Vim
        ".vim": "vim",

        // Lisp
        ".lisp": "lisp",
        ".lsp": "lisp",

        // Scheme
        ".scm": "scheme",
        ".ss": "scheme",

        // Assembly
        ".asm": "x86asm",
        ".s": "x86asm",

        // Groovy
        ".groovy": "groovy",
        ".gradle": "groovy",

        // CMake
        ".cmake": "cmake",

        // Diff/Patch
        ".diff": "diff",
        ".patch": "diff",

        // Nginx
        ".nginx": "nginx",

        // Apache
        ".htaccess": "apache",

        // Makefile
        ".mk": "makefile",
        ".make": "makefile",

        // Zig
        ".zig": "zig",

        // Crystal
        ".cr": "crystal",

        // Nim
        ".nim": "nim",

        // Elm
        ".elm": "elm",

        // PureScript
        ".purs": "purescript",

        // Reason
        ".re": "reason",
        ".rei": "reason",

        // MATLAB/Octave (Note: .m removed due to conflict with Objective-C)
        ".mat": "matlab",

        // Julia
        ".jl": "julia",

        // Fortran
        ".f": "fortran",
        ".f90": "fortran",
        ".f95": "fortran",

        // COBOL
        ".cob": "cobol",
        ".cbl": "cobol",

        // Verilog (Note: .v also used by V language, prioritizing Verilog as it's more common)
        ".v": "verilog",
        ".vh": "verilog",

        // VHDL
        ".vhd": "vhdl",
        ".vhdl": "vhdl",

        // Solidity
        ".sol": "solidity",
    ]
}
