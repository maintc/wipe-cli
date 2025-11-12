package carbon

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/maintc/wipe-cli/internal/discord"
)

const (
	CarbonAPIURL     = "https://api.carbonmod.gg/meta/carbon/changelogs.json"
	CarbonBase       = "/opt/carbon"
	CarbonMainURL    = "https://github.com/CarbonCommunity/Carbon/releases/download/production_build/Carbon.Linux.Release.tar.gz"
	CarbonStagingURL = "https://github.com/CarbonCommunity/Carbon/releases/download/rustbeta_staging_build/Carbon.Linux.Debug.tar.gz"
	RustEditURL      = "https://github.com/k1lly0u/Oxide.Ext.RustEdit/raw/master/Oxide.Ext.RustEdit.dll"
)

var (
	// installingMutex prevents concurrent Carbon installations
	installingMutex    sync.Mutex
	installingBranches = make(map[string]bool)
)

// CarbonRelease represents a Carbon release from the API
type CarbonRelease struct {
	Date      string `json:"Date"`
	Version   string `json:"Version"`
	CommitURL string `json:"CommitUrl"`
}

// getCarbonPath returns the installation path for a branch
func getCarbonPath(branch string) string {
	if branch == "" || branch == "main" {
		return CarbonBase
	}
	return filepath.Join(CarbonBase + "-" + branch)
}

// isCarbonInstalled checks if Carbon is installed
func isCarbonInstalled(path string) bool {
	carbonDLL := filepath.Join(path, "carbon", "managed", "Carbon.dll")
	_, err := os.Stat(carbonDLL)
	return err == nil
}

// CheckForCarbonUpdates checks if Carbon has updates available
func CheckForCarbonUpdates(branch, webhookURL string) (bool, string, error) {
	installPath := getCarbonPath(branch)

	// Check if Carbon is installed
	if !isCarbonInstalled(installPath) {
		return false, "", nil
	}

	// Get current installed version
	versionPath := filepath.Join(installPath, "version.txt")
	currentVersionData, err := os.ReadFile(versionPath)
	if err != nil {
		log.Printf("Warning: Could not read current Carbon version for %s: %v", branch, err)
		return false, "", nil
	}
	currentVersion := strings.TrimSpace(string(currentVersionData))

	// Get latest version from Carbon API
	latestVersion, err := getLatestCarbonVersion()
	if err != nil {
		log.Printf("Error checking for Carbon updates: %v", err)
		return false, "", err
	}

	// Compare versions
	if currentVersion != latestVersion {
		log.Printf("Carbon update available for branch %s: %s -> %s", branch, currentVersion, latestVersion)

		// Send notification
		discord.SendInfo(webhookURL, "Carbon Update Available",
			fmt.Sprintf("Carbon has an update available\n\nCurrent: **%s**\nAvailable: **%s**",
				currentVersion, latestVersion))

		return true, latestVersion, nil
	}

	return false, currentVersion, nil
}

// getLatestCarbonVersion queries the Carbon API for the latest version
func getLatestCarbonVersion() (string, error) {
	resp, err := http.Get(CarbonAPIURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch Carbon API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("carbon API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read Carbon API response: %w", err)
	}

	var releases []CarbonRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return "", fmt.Errorf("failed to parse Carbon API response: %w", err)
	}

	if len(releases) == 0 {
		return "", fmt.Errorf("no Carbon releases found")
	}

	// First entry is the latest
	return releases[0].Version, nil
}

// GetCarbonDownloadURL returns the download URL for a Carbon branch
func GetCarbonDownloadURL(branch string) string {
	if branch == "" || branch == "main" {
		return CarbonMainURL
	}
	if branch == "staging" {
		return CarbonStagingURL
	}
	// Default to main for unknown branches
	log.Printf("Warning: Unknown Carbon branch '%s', defaulting to main", branch)
	return CarbonMainURL
}

// InstallCarbon installs Carbon for a specific branch
func InstallCarbon(branch, webhookURL string) error {
	// Check if this branch is already being installed
	installingMutex.Lock()
	if installingBranches[branch] {
		installingMutex.Unlock()
		log.Printf("Carbon for branch '%s' is already being installed, skipping", branch)
		return nil
	}
	installingBranches[branch] = true
	installingMutex.Unlock()

	// Ensure we mark installation as complete when done
	defer func() {
		installingMutex.Lock()
		delete(installingBranches, branch)
		installingMutex.Unlock()
	}()

	installPath := getCarbonPath(branch)
	downloadURL := GetCarbonDownloadURL(branch)

	log.Printf("Installing Carbon for branch '%s' to %s", branch, installPath)

	// Create Carbon directory
	if err := os.MkdirAll(installPath, 0755); err != nil {
		errMsg := fmt.Sprintf("failed to create Carbon directory: %v", err)
		discord.SendError(webhookURL, "Carbon Installation Failed",
			fmt.Sprintf("Failed to install Carbon for branch **%s**\n\n%s", branch, errMsg))
		return fmt.Errorf("%s", errMsg)
	}

	// Download Carbon
	tarPath := filepath.Join(installPath, "carbon.tar.gz")
	log.Printf("Downloading Carbon from %s...", downloadURL)

	if err := downloadFile(downloadURL, tarPath); err != nil {
		errMsg := fmt.Sprintf("failed to download Carbon: %v", err)
		discord.SendError(webhookURL, "Carbon Installation Failed",
			fmt.Sprintf("Failed to install Carbon for branch **%s**\n\n%s", branch, errMsg))
		return fmt.Errorf("%s", errMsg)
	}

	// Extract Carbon
	log.Printf("Extracting Carbon...")
	if err := extractTarGz(tarPath, installPath); err != nil {
		errMsg := fmt.Sprintf("failed to extract Carbon: %v", err)
		discord.SendError(webhookURL, "Carbon Installation Failed",
			fmt.Sprintf("Failed to install Carbon for branch **%s**\n\n%s", branch, errMsg))
		return fmt.Errorf("%s", errMsg)
	}

	// Download RustEdit extension
	log.Printf("Downloading RustEdit extension...")
	rustEditPath := filepath.Join(installPath, "carbon", "extensions", "Oxide.Ext.RustEdit.dll")
	if err := os.MkdirAll(filepath.Dir(rustEditPath), 0755); err == nil {
		if err := downloadFile(RustEditURL, rustEditPath); err != nil {
			log.Printf("Warning: Failed to download RustEdit extension: %v", err)
			// Not critical, continue
		}
	}

	// Get latest version from API and save it
	version, err := getLatestCarbonVersion()
	if err != nil {
		log.Printf("Warning: Could not get Carbon version: %v", err)
		version = "unknown"
	}

	versionPath := filepath.Join(installPath, "version.txt")
	if err := os.WriteFile(versionPath, []byte(version), 0644); err != nil {
		log.Printf("Warning: Could not write version file: %v", err)
	}

	// Clean up tar file
	os.Remove(tarPath)

	log.Printf("✓ Successfully installed Carbon for branch '%s' (version: %s)", branch, version)
	discord.SendSuccess(webhookURL, "Carbon Installation Complete",
		fmt.Sprintf("Carbon for branch **%s** installed successfully\n\nVersion: **%s**", branch, version))

	return nil
}

// EnsureCarbonInstalled checks if Carbon is installed and installs it if not
func EnsureCarbonInstalled(branch, webhookURL string) error {
	installPath := getCarbonPath(branch)

	// Check if Carbon is already installed
	if isCarbonInstalled(installPath) {
		log.Printf("Carbon for branch '%s' already installed at %s", branch, installPath)
		return nil
	}

	log.Printf("Carbon for branch '%s' not found at %s, installing...", branch, installPath)
	return InstallCarbon(branch, webhookURL)
}

// downloadFile downloads a file from a URL
func downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// extractTarGz extracts a tar.gz file to a destination
func extractTarGz(tarPath, destPath string) error {
	// Use tar command to extract
	cmd := exec.Command("tar", "-xzf", tarPath, "-C", destPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar extraction failed: %w\nOutput: %s", err, output)
	}
	return nil
}
