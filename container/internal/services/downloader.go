package services

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ModelDownloader handles downloading and verifying GGUF models
type ModelDownloader struct {
	cacheDir string
}

// NewModelDownloader creates a new model downloader instance
func NewModelDownloader() (*ModelDownloader, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".catnip", "models")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &ModelDownloader{
		cacheDir: cacheDir,
	}, nil
}

// DownloadModel downloads a model from the given URL to the cache directory
// Returns the path to the downloaded model file
func (d *ModelDownloader) DownloadModel(url, filename, expectedChecksum string) (string, error) {
	destPath := filepath.Join(d.cacheDir, filename)

	// Check if model already exists and is valid
	if _, err := os.Stat(destPath); err == nil {
		// File exists, verify checksum
		if expectedChecksum != "" {
			if valid, err := d.verifyChecksum(destPath, expectedChecksum); err == nil && valid {
				return destPath, nil
			}
		} else {
			// No checksum to verify, assume file is good
			return destPath, nil
		}
	}

	// Download to temporary file
	tmpPath := destPath + ".tmp"
	if err := d.downloadFile(url, tmpPath); err != nil {
		os.Remove(tmpPath) // Clean up on error
		return "", fmt.Errorf("failed to download model: %w", err)
	}

	// Verify checksum if provided
	if expectedChecksum != "" {
		valid, err := d.verifyChecksum(tmpPath, expectedChecksum)
		if err != nil {
			os.Remove(tmpPath)
			return "", fmt.Errorf("failed to verify checksum: %w", err)
		}
		if !valid {
			os.Remove(tmpPath)
			return "", fmt.Errorf("checksum verification failed")
		}
	}

	// Atomic rename
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to save model: %w", err)
	}

	return destPath, nil
}

// downloadFile downloads a file from the given URL with progress reporting
func (d *ModelDownloader) downloadFile(url, destPath string) error {
	// Create the file
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url) //nolint:gosec // URL comes from trusted config
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Writer with progress reporting
	totalBytes := resp.ContentLength

	// Create a reader that reports progress
	reader := &progressReader{
		reader: resp.Body,
		total:  totalBytes,
		onProgress: func(current, total int64) {
			if total > 0 {
				percent := float64(current) / float64(total) * 100
				fmt.Printf("\rDownloading model: %.1f%% (%d/%d MB)",
					percent,
					current/(1024*1024),
					total/(1024*1024))
			}
		},
	}

	// Write the body to file
	_, err = io.Copy(out, reader)
	if err != nil {
		return err
	}

	fmt.Println() // New line after progress
	return nil
}

// verifyChecksum verifies the SHA256 checksum of a file
func (d *ModelDownloader) verifyChecksum(filePath, expectedChecksum string) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, err
	}

	actualChecksum := hex.EncodeToString(hash.Sum(nil))
	return actualChecksum == expectedChecksum, nil
}

// GetModelPath returns the path where a model with the given filename would be stored
func (d *ModelDownloader) GetModelPath(filename string) string {
	return filepath.Join(d.cacheDir, filename)
}

// progressReader wraps an io.Reader to report progress
type progressReader struct {
	reader     io.Reader
	total      int64
	current    int64
	onProgress func(current, total int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.current += int64(n)
	if pr.onProgress != nil {
		pr.onProgress(pr.current, pr.total)
	}
	return n, err
}

// LibraryDownloader handles downloading llama.cpp libraries
type LibraryDownloader struct {
	libDir string
}

// NewLibraryDownloader creates a new library downloader instance
func NewLibraryDownloader() (*LibraryDownloader, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	libDir := filepath.Join(homeDir, ".catnip", "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create lib directory: %w", err)
	}

	return &LibraryDownloader{
		libDir: libDir,
	}, nil
}

// DownloadLibrary downloads the llama.cpp library for the current platform
// Returns the path to the main library file (libllama.dylib, libllama.so, etc.)
func (d *LibraryDownloader) DownloadLibrary() (string, error) {
	// Determine platform-specific details
	osName, archName, libExt, err := d.getPlatformInfo()
	if err != nil {
		return "", err
	}

	// Check if library already exists
	libPath := filepath.Join(d.libDir, osName, archName, "libllama"+libExt)
	if _, err := os.Stat(libPath); err == nil {
		// Library already exists
		return libPath, nil
	}

	// Get latest llama.cpp release info
	releaseTag, downloadURL, err := d.getLlamaCppRelease(osName, archName)
	if err != nil {
		return "", fmt.Errorf("failed to get llama.cpp release: %w", err)
	}

	fmt.Printf("📦 Downloading llama.cpp %s for %s/%s...\n", releaseTag, osName, archName)

	// Download archive
	tmpFile := filepath.Join(d.libDir, "llama-cpp-tmp.zip")
	if err := d.downloadFileWithProgress(downloadURL, tmpFile); err != nil {
		os.Remove(tmpFile)
		return "", fmt.Errorf("failed to download library: %w", err)
	}
	defer os.Remove(tmpFile)

	// Extract to platform-specific directory
	extractDir := filepath.Join(d.libDir, osName, archName)
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create extract directory: %w", err)
	}

	if err := d.extractZip(tmpFile, extractDir); err != nil {
		return "", fmt.Errorf("failed to extract archive: %w", err)
	}

	fmt.Println("✅ llama.cpp libraries installed successfully")

	// Return path to main library
	return libPath, nil
}

// getPlatformInfo returns OS name, architecture, and library extension for the current platform
func (d *LibraryDownloader) getPlatformInfo() (osName, archName, libExt string, err error) {
	switch runtime.GOOS {
	case "darwin":
		osName = "macos"
		libExt = ".dylib"
	case "linux":
		osName = "ubuntu" // llama.cpp releases use "ubuntu" for Linux
		libExt = ".so"
	case "windows":
		osName = "win"
		libExt = ".dll"
	default:
		return "", "", "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	switch runtime.GOARCH {
	case "amd64":
		archName = "x64"
	case "arm64":
		archName = "arm64"
	default:
		return "", "", "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}

	return osName, archName, libExt, nil
}

// getLlamaCppRelease fetches the latest llama.cpp release info from GitHub
func (d *LibraryDownloader) getLlamaCppRelease(osName, archName string) (tag, downloadURL string, err error) {
	// Get latest release from GitHub API
	resp, err := http.Get("https://api.github.com/repos/ggml-org/llama.cpp/releases/latest")
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GitHub API returned status: %s", resp.Status)
	}

	// Parse response to find the right asset
	// We're looking for patterns like:
	// - llama-{tag}-bin-macos-arm64.zip
	// - llama-{tag}-bin-ubuntu-x64.zip
	// - llama-{tag}-bin-win-cpu-x64.zip

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	bodyStr := string(body)

	// Extract tag_name
	tagStart := strings.Index(bodyStr, `"tag_name":"`)
	if tagStart == -1 {
		return "", "", fmt.Errorf("could not find tag_name in GitHub response")
	}
	tagStart += len(`"tag_name":"`)
	tagEnd := strings.Index(bodyStr[tagStart:], `"`)
	if tagEnd == -1 {
		return "", "", fmt.Errorf("could not parse tag_name")
	}
	tag = bodyStr[tagStart : tagStart+tagEnd]

	// Build expected filename pattern
	var pattern string
	switch osName {
	case "macos":
		pattern = fmt.Sprintf("llama-%s-bin-macos-%s.zip", tag, archName)
	case "ubuntu":
		pattern = fmt.Sprintf("llama-%s-bin-ubuntu-%s.zip", tag, archName)
	case "win":
		pattern = fmt.Sprintf("llama-%s-bin-win-cpu-%s.zip", tag, archName)
	}

	// Find download URL in browser_download_url fields
	searchStr := fmt.Sprintf(`"browser_download_url":"https://github.com/ggml-org/llama.cpp/releases/download/%s/%s"`, tag, pattern)
	urlStart := strings.Index(bodyStr, searchStr)
	if urlStart == -1 {
		return "", "", fmt.Errorf("could not find download URL for %s", pattern)
	}

	urlStart += len(`"browser_download_url":"`)
	urlEnd := strings.Index(bodyStr[urlStart:], `"`)
	downloadURL = bodyStr[urlStart : urlStart+urlEnd]

	return tag, downloadURL, nil
}

// downloadFileWithProgress downloads a file with progress reporting
func (d *LibraryDownloader) downloadFileWithProgress(url, destPath string) error {
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url) //nolint:gosec // URL from trusted GitHub API
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	totalBytes := resp.ContentLength
	reader := &progressReader{
		reader: resp.Body,
		total:  totalBytes,
		onProgress: func(current, total int64) {
			if total > 0 {
				percent := float64(current) / float64(total) * 100
				fmt.Printf("\rDownloading: %.1f%% (%d/%d MB)",
					percent,
					current/(1024*1024),
					total/(1024*1024))
			}
		},
	}

	_, err = io.Copy(out, reader)
	if err != nil {
		return err
	}

	fmt.Println() // New line after progress
	return nil
}

// extractZip extracts a zip file to the destination directory
func (d *LibraryDownloader) extractZip(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		// Only extract files in build/bin/ directory (where the libraries are)
		if !strings.Contains(file.Name, "build/bin/") {
			continue
		}

		// Get the filename relative to build/bin/
		parts := strings.Split(file.Name, "build/bin/")
		if len(parts) != 2 {
			continue
		}
		filename := parts[1]

		// Skip directories and non-library files
		if file.FileInfo().IsDir() || filename == "" {
			continue
		}

		// Only extract .dylib, .so, .dll files
		if !strings.HasSuffix(filename, ".dylib") &&
			!strings.HasSuffix(filename, ".so") &&
			!strings.HasSuffix(filename, ".dll") {
			continue
		}

		// Create destination path
		destPath := filepath.Join(destDir, filename)

		// Extract file
		if err := d.extractFile(file, destPath); err != nil {
			return fmt.Errorf("failed to extract %s: %w", filename, err)
		}
	}

	return nil
}

// extractFile extracts a single file from a zip archive
func (d *LibraryDownloader) extractFile(file *zip.File, destPath string) error {
	// Open source file
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	// Create destination file
	dest, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return err
	}
	defer dest.Close()

	// Copy contents
	// We only extract .dylib/.so/.dll files from trusted GitHub releases
	_, err = io.Copy(dest, src) //nolint:gosec // Trusted source (GitHub llama.cpp releases)
	return err
}

// GetLibraryPath returns the path where the library for the current platform would be stored
func (d *LibraryDownloader) GetLibraryPath() (string, error) {
	osName, archName, libExt, err := d.getPlatformInfo()
	if err != nil {
		return "", err
	}

	return filepath.Join(d.libDir, osName, archName, "libllama"+libExt), nil
}

// StatFile is a helper function to get file info (exported for use in cmd package)
func StatFile(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
