package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewContainerServiceWithRuntime(t *testing.T) {
	tests := []struct {
		name             string
		preferredRuntime string
		expectError      bool
		expectedRuntime  ContainerRuntime
	}{
		{
			name:             "docker runtime",
			preferredRuntime: "docker",
			expectError:      false,
			expectedRuntime:  RuntimeDocker,
		},
		{
			name:             "container runtime",
			preferredRuntime: "container",
			expectError:      false,
			expectedRuntime:  RuntimeApple,
		},
		{
			name:             "apple runtime alias",
			preferredRuntime: "apple",
			expectError:      false,
			expectedRuntime:  RuntimeApple,
		},
		{
			name:             "invalid runtime",
			preferredRuntime: "invalid",
			expectError:      true,
		},
		{
			name:             "empty runtime auto-detects",
			preferredRuntime: "",
			expectError:      false, // Will auto-detect available runtime
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip tests for runtimes that don't exist on this system
			if tt.preferredRuntime == "docker" && !commandExists("docker") {
				t.Skip("Docker not available on this system")
			}
			if (tt.preferredRuntime == "container" || tt.preferredRuntime == "apple") && !commandExists("container") {
				t.Skip("Apple Container SDK not available on this system")
			}

			// For auto-detection test, skip if no container runtime is available
			if tt.name == "empty runtime auto-detects" && !commandExists("docker") && !commandExists("container") {
				t.Skip("No container runtime available for auto-detection")
			}

			cs, err := NewContainerServiceWithRuntime(tt.preferredRuntime)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if cs == nil {
				t.Errorf("Expected ContainerService but got nil")
				return
			}

			if tt.preferredRuntime != "" && cs.runtime != tt.expectedRuntime {
				t.Errorf("Expected runtime %v, got %v", tt.expectedRuntime, cs.runtime)
			}
		})
	}
}

func TestDetectContainerRuntime(t *testing.T) {
	runtime, err := detectContainerRuntime()

	// Should not error if either docker or container is available
	if err != nil {
		// Only check if both are missing
		if !commandExists("docker") && !commandExists("container") {
			t.Logf("No container runtime available: %v", err)
			return
		}
		t.Errorf("Unexpected error when runtime should be available: %v", err)
		return
	}

	// Should prefer docker over container
	if commandExists("docker") && runtime != RuntimeDocker {
		t.Errorf("Expected Docker runtime when docker command is available, got %v", runtime)
	} else if !commandExists("docker") && commandExists("container") && runtime != RuntimeApple {
		t.Errorf("Expected Apple Container runtime when only container is available, got %v", runtime)
	}
}

func TestGetRuntime(t *testing.T) {
	tests := []struct {
		name     string
		runtime  ContainerRuntime
		expected ContainerRuntime
	}{
		{
			name:     "docker runtime",
			runtime:  RuntimeDocker,
			expected: RuntimeDocker,
		},
		{
			name:     "apple container runtime",
			runtime:  RuntimeApple,
			expected: RuntimeApple,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &ContainerService{runtime: tt.runtime}
			result := cs.GetRuntime()
			if result != tt.expected {
				t.Errorf("Expected runtime %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tilde expansion",
			input:    "~/test/path",
			expected: filepath.Join(os.Getenv("HOME"), "test/path"),
		},
		{
			name:     "no tilde",
			input:    "/absolute/path",
			expected: "/absolute/path",
		},
		{
			name:     "relative path",
			input:    "relative/path",
			expected: "relative/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandPath(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCopyFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tempDir, "source.txt")
	srcContent := "test content"
	err := os.WriteFile(srcPath, []byte(srcContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Copy file
	dstPath := filepath.Join(tempDir, "destination.txt")
	err = copyFile(srcPath, dstPath)
	if err != nil {
		t.Errorf("Failed to copy file: %v", err)
		return
	}

	// Verify destination file exists and has correct content
	if _, err := os.Stat(dstPath); os.IsNotExist(err) {
		t.Errorf("Destination file was not created")
		return
	}

	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Errorf("Failed to read destination file: %v", err)
		return
	}

	if string(dstContent) != srcContent {
		t.Errorf("Expected content %q, got %q", srcContent, string(dstContent))
	}

	// Verify permissions were copied
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		t.Errorf("Failed to get source file info: %v", err)
		return
	}

	dstInfo, err := os.Stat(dstPath)
	if err != nil {
		t.Errorf("Failed to get destination file info: %v", err)
		return
	}

	if srcInfo.Mode() != dstInfo.Mode() {
		t.Errorf("Expected permissions %v, got %v", srcInfo.Mode(), dstInfo.Mode())
	}
}

func TestContainerRuntimeConstants(t *testing.T) {
	// Verify that runtime constants have expected values
	if RuntimeDocker != "docker" {
		t.Errorf("Expected RuntimeDocker to be 'docker', got %q", RuntimeDocker)
	}

	if RuntimeApple != "container" {
		t.Errorf("Expected RuntimeApple to be 'container', got %q", RuntimeApple)
	}
}
