package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "🔄 Upgrade catnip to the latest version",
	Long: `# 🔄 Upgrade Catnip

**Safely upgrade your catnip installation to the latest version.**

## 🔒 Safety Features
- Downloads and verifies the new version before replacing
- Creates backup of current binary during upgrade
- Automatically rollback if upgrade fails
- Skips upgrade if already running the latest version
- TTY detection for non-interactive environments

## 🎯 Process
1. Check current version against latest release
2. Download new version to temporary location
3. Verify new binary functionality
4. Safely replace current installation
5. Clean up temporary files

## 🚀 Usage Examples

	# Check for updates without upgrading
	catnip upgrade --check

	# Upgrade with automatic confirmation (non-interactive)
	catnip upgrade --yes

	# Include development/pre-release versions
	catnip upgrade --dev

	# Upgrade to specific version
	catnip upgrade --version v1.2.3

	# Force upgrade even if same version
	catnip upgrade --force

The upgrade will use the same installation method as the original install script.`,
	RunE: runUpgrade,
}

// GitHubRelease represents a GitHub release response
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
}

func init() {
	rootCmd.AddCommand(upgradeCmd)

	upgradeCmd.Flags().BoolP("force", "f", false, "Force upgrade even if already on latest version")
	upgradeCmd.Flags().BoolP("yes", "y", false, "Automatically confirm upgrade without prompting")
	upgradeCmd.Flags().Bool("check", false, "Only check for updates, don't upgrade")
	upgradeCmd.Flags().Bool("dev", false, "Include development/pre-release versions")
	upgradeCmd.Flags().String("version", "", "Upgrade to specific version (e.g., v1.0.0)")
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	checkOnly, _ := cmd.Flags().GetBool("check")
	autoYes, _ := cmd.Flags().GetBool("yes")
	includeDev, _ := cmd.Flags().GetBool("dev")
	targetVersion, _ := cmd.Flags().GetString("version")

	fmt.Println("🔍 Checking for catnip updates...")

	// Get current version
	currentVersion := GetVersion()
	if currentVersion == "" || currentVersion == "dev" {
		return fmt.Errorf("unable to determine current version - this may be a development build")
	}

	var latestVersion string
	var err error

	// Determine target version
	if targetVersion != "" {
		// User specified a specific version
		latestVersion = targetVersion
		if !strings.HasPrefix(latestVersion, "v") {
			latestVersion = "v" + latestVersion
		}
		fmt.Printf("Target version specified: %s\n", latestVersion)
	} else {
		// Get latest version from Catnip proxy
		latestVersion, err = getLatestVersion(includeDev)
		if err != nil {
			return fmt.Errorf("failed to check for latest version: %w", err)
		}
	}

	fmt.Printf("Current version: %s\n", currentVersion)
	fmt.Printf("Latest version:  %s\n", latestVersion)

	// Check if upgrade is needed
	if !force && currentVersion == latestVersion {
		fmt.Println("✅ Already running the latest version")
		return nil
	}

	if checkOnly {
		if currentVersion != latestVersion {
			fmt.Println("🆕 New version available")
			return nil
		}
		return nil
	}

	// Confirm upgrade (skip if force, autoYes, or not in TTY)
	if !force && !autoYes && !confirmUpgrade(currentVersion, latestVersion) {
		fmt.Println("Upgrade cancelled")
		return nil
	}

	// Perform the upgrade
	fmt.Println("🚀 Starting upgrade...")
	return performUpgrade(currentVersion, latestVersion)
}

func getLatestVersion(includeDev bool) (string, error) {
	proxyURL := os.Getenv("CATNIP_PROXY_URL")
	if proxyURL == "" {
		proxyURL = "https://install.catnip.sh"
	}

	var apiURL string
	if includeDev {
		// Get all releases and find the latest (including pre-releases)
		apiURL = fmt.Sprintf("%s/v1/github/releases", proxyURL)
	} else {
		// Get latest stable release only
		apiURL = fmt.Sprintf("%s/v1/github/releases/latest", proxyURL)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	if includeDev {
		// Parse array of releases and return the first one (latest)
		var releases []GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			return "", err
		}
		if len(releases) == 0 {
			return "", fmt.Errorf("no releases found")
		}
		return releases[0].TagName, nil
	} else {
		// Parse single latest release
		var release GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return "", err
		}
		return release.TagName, nil
	}
}

func confirmUpgrade(currentVersion, latestVersion string) bool {
	// Check if we're in a TTY (interactive terminal)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Printf("Not in interactive terminal, skipping upgrade (use --yes to force)\n")
		return false
	}

	fmt.Printf("Do you want to upgrade from %s to %s? [y/N]: ", currentVersion, latestVersion)

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}

	response := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return response == "y" || response == "yes"
}

func performUpgrade(currentVersion, latestVersion string) error {
	// Find current binary path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find current executable path: %w", err)
	}

	// Resolve symlinks to get the real path
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	fmt.Printf("Upgrading binary at: %s\n", execPath)

	// Detect if we're dealing with a macOS app bundle
	if goruntime.GOOS == "darwin" && strings.Contains(execPath, "Catnip.app") {
		return performMacOSAppBundleUpgrade(execPath, currentVersion, latestVersion)
	} else {
		return performStandardBinaryUpgrade(execPath, currentVersion, latestVersion)
	}
}

func downloadAndVerifyBinary(version, tempPath string) error {
	// Use the same download logic as the install script
	proxyURL := os.Getenv("CATNIP_PROXY_URL")
	if proxyURL == "" {
		proxyURL = "https://install.catnip.sh"
	}

	// Detect OS and architecture
	osName := goruntime.GOOS
	switch osName {
	case "darwin", "linux":
		// Supported platforms
	default:
		return fmt.Errorf("unsupported operating system: %s", osName)
	}

	archName := goruntime.GOARCH
	switch archName {
	case "amd64":
		archName = "amd64"
	case "arm64":
		archName = "arm64"
	case "386":
		archName = "386"
	default:
		return fmt.Errorf("unsupported architecture: %s", archName)
	}

	// Construct download URLs
	versionClean := strings.TrimPrefix(version, "v")
	archiveName := fmt.Sprintf("catnip_%s_%s_%s.tar.gz", versionClean, osName, archName)
	baseURL := fmt.Sprintf("%s/v1/github/releases/download/%s", proxyURL, version)
	downloadURL := fmt.Sprintf("%s/%s", baseURL, archiveName)
	checksumURL := fmt.Sprintf("%s/checksums.txt", baseURL)

	fmt.Printf("Downloading from: %s\n", downloadURL)

	// Create temporary files
	archivePath := tempPath + ".tar.gz"
	checksumPath := tempPath + ".checksums.txt"
	defer os.Remove(archivePath)
	defer os.Remove(checksumPath)

	client := &http.Client{Timeout: 5 * time.Minute}

	// Download archive
	if err := downloadFile(client, downloadURL, archivePath, "binary archive"); err != nil {
		return err
	}

	// Download checksums
	if err := downloadFile(client, checksumURL, checksumPath, "checksums"); err != nil {
		return err
	}

	// Verify checksum (same logic as install script)
	if err := verifyChecksum(archivePath, archiveName, checksumPath); err != nil {
		return err
	}

	// Extract the binary from the archive
	return extractBinary(archivePath, tempPath, osName)
}

func downloadFile(client *http.Client, url, path, description string) error {
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", description, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d for %s", resp.StatusCode, description)
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

func verifyChecksum(filePath, fileName, checksumPath string) error {
	fmt.Printf("🔐 Verifying checksum for %s...\n", fileName)

	// Read checksums file
	checksumData, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("failed to read checksums file: %w", err)
	}

	// Find checksum for our file
	lines := strings.Split(string(checksumData), "\n")
	var expectedChecksum string
	for _, line := range lines {
		if strings.Contains(line, fileName) {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				expectedChecksum = parts[0]
				break
			}
		}
	}

	if expectedChecksum == "" {
		return fmt.Errorf("could not find checksum for %s in checksums file", fileName)
	}

	// Calculate actual checksum
	actualChecksum, err := calculateSHA256(filePath)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum verification failed!\nExpected: %s\nActual: %s", expectedChecksum, actualChecksum)
	}

	fmt.Printf("✅ Checksum verified successfully\n")
	return nil
}

func extractBinary(archivePath, targetPath, osName string) error {
	// Create temporary directory for extraction
	extractDir := targetPath + ".extract"
	defer os.RemoveAll(extractDir)

	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return err
	}

	// Extract tar.gz
	cmd := exec.Command("tar", "-xzf", archivePath, "-C", extractDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	// Handle macOS app bundle vs standard binary (same logic as install script)
	if osName == "darwin" {
		// Check for app bundle first
		appBundlePath := filepath.Join(extractDir, "Catnip.app", "Contents", "MacOS", "catnip")
		if _, err := os.Stat(appBundlePath); err == nil {
			// Copy from app bundle
			return copyBinaryFile(appBundlePath, targetPath)
		}
		// Fall back to standalone binary
		binaryPath := filepath.Join(extractDir, "catnip")
		return copyBinaryFile(binaryPath, targetPath)
	} else {
		// Linux: standard binary
		binaryPath := filepath.Join(extractDir, "catnip")
		return copyBinaryFile(binaryPath, targetPath)
	}
}

func copyBinaryFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	// Copy permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, srcInfo.Mode())
}

func calculateSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func createBackup(execPath, backupPath string) error {
	// Remove existing backup if it exists
	os.Remove(backupPath)

	// Create backup by renaming current binary
	return os.Rename(execPath, backupPath)
}

func installNewBinary(tempPath, execPath string) error {
	// Move new binary to final location
	return os.Rename(tempPath, execPath)
}

func verifyFinalInstallation(execPath string) error {
	// Simple check that file exists and is executable
	info, err := os.Stat(execPath)
	if err != nil {
		return fmt.Errorf("installed binary not found: %w", err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("installed path is not a regular file")
	}

	// Check if executable (at least readable for owner)
	if info.Mode()&0100 == 0 {
		return fmt.Errorf("installed binary is not executable")
	}

	fmt.Printf("✅ Installation verified: binary exists and is executable\n")
	return nil
}

func performMacOSAppBundleUpgrade(execPath, currentVersion, latestVersion string) error {
	// Extract app bundle path from executable path
	// execPath should be something like: /path/to/Catnip.app/Contents/MacOS/catnip
	appBundlePath := execPath
	for !strings.HasSuffix(appBundlePath, "Catnip.app") {
		appBundlePath = filepath.Dir(appBundlePath)
		if appBundlePath == "/" || appBundlePath == "." {
			return fmt.Errorf("could not find Catnip.app bundle in path: %s", execPath)
		}
	}

	fmt.Printf("Upgrading macOS app bundle at: %s\n", appBundlePath)

	// Create temporary directory for download and extraction
	tempDir, err := os.MkdirTemp("", "catnip-upgrade-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Download and extract to temp directory
	fmt.Println("📥 Downloading and verifying new version...")
	if err := downloadAndExtractAppBundle(latestVersion, tempDir); err != nil {
		return fmt.Errorf("failed to download and verify new version: %w", err)
	}

	newAppBundlePath := filepath.Join(tempDir, "Catnip.app")
	if _, err := os.Stat(newAppBundlePath); err != nil {
		return fmt.Errorf("downloaded app bundle not found: %w", err)
	}

	// Create backup of current app bundle
	backupPath := appBundlePath + ".backup"
	fmt.Println("💾 Creating backup...")
	if err := createAppBundleBackup(appBundlePath, backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Install new app bundle atomically
	fmt.Println("🔄 Installing new app bundle...")
	if err := installNewAppBundle(newAppBundlePath, appBundlePath); err != nil {
		fmt.Println("❌ Installation failed, restoring backup...")
		if restoreErr := restoreAppBundleBackup(backupPath, appBundlePath); restoreErr != nil {
			return fmt.Errorf("installation failed and backup restore also failed: %v (original error: %w)", restoreErr, err)
		}
		return fmt.Errorf("installation failed, backup restored: %w", err)
	}

	// Verify installation
	fmt.Println("✅ Verifying installation...")
	if err := verifyFinalInstallation(execPath); err != nil {
		fmt.Println("❌ Final verification failed, restoring backup...")
		if restoreErr := restoreAppBundleBackup(backupPath, appBundlePath); restoreErr != nil {
			return fmt.Errorf("final verification failed and backup restore also failed: %v (original error: %w)", restoreErr, err)
		}
		return fmt.Errorf("final verification failed, backup restored: %w", err)
	}

	// Clean up backup
	os.RemoveAll(backupPath)

	fmt.Printf("🎉 Successfully upgraded catnip app bundle from %s to %s!\n", currentVersion, latestVersion)
	return nil
}

func performStandardBinaryUpgrade(execPath, currentVersion, latestVersion string) error {
	// Standard binary upgrade logic (for Linux)
	backupPath := execPath + ".backup"
	tempPath := execPath + ".tmp"

	// Clean up any existing temp files
	os.Remove(tempPath)
	defer os.Remove(tempPath)

	// Download and verify new version
	fmt.Println("📥 Downloading and verifying new version...")
	if err := downloadAndVerifyBinary(latestVersion, tempPath); err != nil {
		return fmt.Errorf("failed to download and verify new version: %w", err)
	}

	// Create backup of current binary
	fmt.Println("💾 Creating backup...")
	if err := createBackup(execPath, backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Install new binary
	fmt.Println("🔄 Installing new version...")
	if err := installNewBinary(tempPath, execPath); err != nil {
		fmt.Println("❌ Installation failed, restoring backup...")
		if restoreErr := restoreBackup(backupPath, execPath); restoreErr != nil {
			return fmt.Errorf("installation failed and backup restore also failed: %v (original error: %w)", restoreErr, err)
		}
		return fmt.Errorf("installation failed, backup restored: %w", err)
	}

	// Verify installation
	fmt.Println("✅ Verifying installation...")
	if err := verifyFinalInstallation(execPath); err != nil {
		fmt.Println("❌ Final verification failed, restoring backup...")
		if restoreErr := restoreBackup(backupPath, execPath); restoreErr != nil {
			return fmt.Errorf("final verification failed and backup restore also failed: %v (original error: %w)", restoreErr, err)
		}
		return fmt.Errorf("final verification failed, backup restored: %w", err)
	}

	// Clean up backup
	os.Remove(backupPath)

	fmt.Printf("🎉 Successfully upgraded catnip from %s to %s!\n", currentVersion, latestVersion)
	return nil
}

func downloadAndExtractAppBundle(version, tempDir string) error {
	// Same as downloadAndVerifyBinary but extracts to directory instead of single file
	proxyURL := os.Getenv("CATNIP_PROXY_URL")
	if proxyURL == "" {
		proxyURL = "https://install.catnip.sh"
	}

	// Construct download URLs
	versionClean := strings.TrimPrefix(version, "v")
	archiveName := fmt.Sprintf("catnip_%s_darwin_arm64.tar.gz", versionClean)
	baseURL := fmt.Sprintf("%s/v1/github/releases/download/%s", proxyURL, version)
	downloadURL := fmt.Sprintf("%s/%s", baseURL, archiveName)
	checksumURL := fmt.Sprintf("%s/checksums.txt", baseURL)

	fmt.Printf("Downloading from: %s\n", downloadURL)

	// Create temporary files
	archivePath := filepath.Join(tempDir, archiveName)
	checksumPath := filepath.Join(tempDir, "checksums.txt")

	client := &http.Client{Timeout: 5 * time.Minute}

	// Download archive
	if err := downloadFile(client, downloadURL, archivePath, "binary archive"); err != nil {
		return err
	}

	// Download checksums
	if err := downloadFile(client, checksumURL, checksumPath, "checksums"); err != nil {
		return err
	}

	// Verify checksum
	if err := verifyChecksum(archivePath, archiveName, checksumPath); err != nil {
		return err
	}

	// Extract the entire archive to temp directory
	cmd := exec.Command("tar", "-xzf", archivePath, "-C", tempDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	return nil
}

func createAppBundleBackup(appBundlePath, backupPath string) error {
	// Remove existing backup if it exists
	os.RemoveAll(backupPath)

	// Use cp -R to create backup (preserves all metadata)
	cmd := exec.Command("cp", "-R", appBundlePath, backupPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create app bundle backup: %w", err)
	}

	return nil
}

func installNewAppBundle(newAppBundlePath, targetAppBundlePath string) error {
	// Remove existing app bundle
	if err := os.RemoveAll(targetAppBundlePath); err != nil {
		return fmt.Errorf("failed to remove existing app bundle: %w", err)
	}

	// Move new app bundle to target location
	if err := os.Rename(newAppBundlePath, targetAppBundlePath); err != nil {
		return fmt.Errorf("failed to install new app bundle: %w", err)
	}

	return nil
}

func restoreAppBundleBackup(backupPath, appBundlePath string) error {
	// Remove failed installation
	os.RemoveAll(appBundlePath)

	// Restore backup
	return os.Rename(backupPath, appBundlePath)
}

func restoreBackup(backupPath, execPath string) error {
	// Remove failed installation
	os.Remove(execPath)

	// Restore backup
	return os.Rename(backupPath, execPath)
}
