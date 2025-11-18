//go:build unix

package services

import (
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// Stderr redirection state
var (
	savedStderrFd    = -1
	stderrSuppressed bool
	suppressMutex    sync.Mutex
)

// suppressStderr redirects stderr (fd 2) to /dev/null to silence llama.cpp's verbose output
func suppressStderr() {
	suppressMutex.Lock()
	defer suppressMutex.Unlock()

	if stderrSuppressed {
		return
	}

	// Open /dev/null
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return // If we can't open /dev/null, just continue with normal stderr
	}

	// Save the original stderr file descriptor by duplicating it
	savedStderrFd, err = unix.Dup(int(os.Stderr.Fd()))
	if err != nil {
		devNull.Close()
		return
	}

	// Redirect stderr (fd 2) to /dev/null using dup2
	// unix.Dup2 works on all Unix platforms including Linux arm64
	err = unix.Dup2(int(devNull.Fd()), int(os.Stderr.Fd()))
	if err != nil {
		unix.Close(savedStderrFd)
		devNull.Close()
		return
	}

	devNull.Close() // We can close devNull now, the fd is duplicated to stderr
	stderrSuppressed = true
}

// restoreStderr restores the original stderr file descriptor
func restoreStderr() {
	suppressMutex.Lock()
	defer suppressMutex.Unlock()

	if !stderrSuppressed || savedStderrFd < 0 {
		return
	}

	// Restore stderr by duplicating the saved fd back to fd 2
	_ = unix.Dup2(savedStderrFd, int(os.Stderr.Fd()))

	// Close the saved fd
	unix.Close(savedStderrFd)
	savedStderrFd = -1
	stderrSuppressed = false
}
