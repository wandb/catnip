package config

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetClaudeConfigDir(t *testing.T) {
	homeDir := "/home/testuser"

	t.Run("on non-Linux systems uses ~/.claude", func(t *testing.T) {
		if runtime.GOOS == "linux" {
			t.Skip("skipping test on Linux")
		}
		result := getClaudeConfigDir(homeDir)
		assert.Equal(t, filepath.Join(homeDir, ".claude"), result)
	})

	t.Run("on Linux with XDG_CONFIG_HOME uses $XDG_CONFIG_HOME/claude", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("skipping test on non-Linux systems")
		}

		// t.Setenv automatically restores the original value after the test
		t.Setenv("XDG_CONFIG_HOME", "/custom/config")
		result := getClaudeConfigDir(homeDir)
		assert.Equal(t, "/custom/config/claude", result)
	})

	t.Run("on Linux without XDG_CONFIG_HOME uses ~/.config/claude", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("skipping test on non-Linux systems")
		}

		// t.Setenv with empty string effectively unsets for our purposes
		t.Setenv("XDG_CONFIG_HOME", "")
		result := getClaudeConfigDir(homeDir)
		assert.Equal(t, filepath.Join(homeDir, ".config", "claude"), result)
	})
}

func TestGetClaudeProjectsDir(t *testing.T) {
	t.Run("returns ClaudeConfigDir/projects", func(t *testing.T) {
		rc := &RuntimeConfig{
			ClaudeConfigDir: "/home/user/.config/claude",
		}
		result := rc.GetClaudeProjectsDir()
		assert.Equal(t, "/home/user/.config/claude/projects", result)
	})
}
