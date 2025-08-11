module github.com/vanpelt/catnip-desktop

go 1.24.0

toolchain go1.24.4

// This module serves as a wrapper for the container desktop app
require github.com/vanpelt/catnip v0.0.0-00010101000000-000000000000

replace github.com/vanpelt/catnip => ./container

// The actual desktop application is built from container/cmd/desktop/
