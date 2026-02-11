package steamcmd

import (
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
	SteamCMDURL     = "https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz"
	RustAppID       = "258550"
	RustInstallBase = "/opt/rust"
	SteamCMDBase    = "/opt/rust/steamcmd"
)

var (
	// installMutex prevents concurrent steamcmd operations
	installMutex sync.Mutex
	// installingBranches tracks which branches are currently being installed/updated
	installingBranches = make(map[string]bool)
	installingMutex    sync.Mutex
	// branchLocks provides per-branch RW locks to coordinate installs vs syncs
	branchLocks = make(map[string]*sync.RWMutex)
	branchMutex sync.Mutex
)

// EnsureRustBranchInstalled checks if a Rust branch is installed and installs it if not
func EnsureRustBranchInstalled(branch, webhookURL string) error {
	installPath := getRustInstallPath(branch)

	// Check if branch is already installed
	if isRustInstalled(installPath) {
		log.Printf("Rust branch '%s' already installed at %s", branch, installPath)
		return nil
	}

	log.Printf("Rust branch '%s' not found at %s, installing...", branch, installPath)
	return InstallRustBranch(branch, webhookURL)
}

// InstallRustBranch installs a Rust branch using steamcmd
func InstallRustBranch(branch, webhookURL string) error {
	// Check if this branch is already being installed
	installingMutex.Lock()
	if installingBranches[branch] {
		installingMutex.Unlock()
		log.Printf("Branch '%s' is already being installed, skipping", branch)
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

	// Acquire WRITE lock for this branch to block syncServer reads during install
	branchLock := getBranchLock(branch)
	branchLock.Lock()
	defer branchLock.Unlock()

	// Acquire global install mutex to prevent concurrent steamcmd operations
	installMutex.Lock()
	defer installMutex.Unlock()

	installPath := getRustInstallPath(branch)

	log.Printf("Installing Rust branch '%s' to %s", branch, installPath)

	// Read old buildid BEFORE wiping the directory
	oldBuildID := ""
	buildidPath := filepath.Join(installPath, "buildid")
	if data, err := os.ReadFile(buildidPath); err == nil {
		oldBuildID = strings.TrimSpace(string(data))
	}

	// Create base rust directory
	if err := os.MkdirAll(RustInstallBase, 0755); err != nil {
		errMsg := fmt.Sprintf("failed to create rust base directory: %v", err)
		discord.SendError(webhookURL, "Rust Installation Failed", fmt.Sprintf("Failed to install Rust branch **%s**\n\n%s", branch, errMsg))
		return fmt.Errorf("%s", errMsg)
	}

	// Remove old branch directory to avoid stale files from previous versions
	if err := os.RemoveAll(installPath); err != nil {
		errMsg := fmt.Sprintf("failed to remove old branch directory: %v", err)
		discord.SendError(webhookURL, "Rust Installation Failed", fmt.Sprintf("Failed to install Rust branch **%s**\n\n%s", branch, errMsg))
		return fmt.Errorf("%s", errMsg)
	}

	// Create fresh branch install directory
	if err := os.MkdirAll(installPath, 0755); err != nil {
		errMsg := fmt.Sprintf("failed to create branch directory: %v", err)
		discord.SendError(webhookURL, "Rust Installation Failed", fmt.Sprintf("Failed to install Rust branch **%s**\n\n%s", branch, errMsg))
		return fmt.Errorf("%s", errMsg)
	}

	// Setup steamcmd (shared across all branches)
	if err := setupSteamCMD(); err != nil {
		errMsg := fmt.Sprintf("failed to setup steamcmd: %v", err)
		discord.SendError(webhookURL, "Rust Installation Failed", fmt.Sprintf("Failed to install Rust branch **%s**\n\n%s", branch, errMsg))
		return fmt.Errorf("%s", errMsg)
	}

	// Install/update the branch
	if err := updateRustBranch(branch, installPath); err != nil {
		errMsg := fmt.Sprintf("failed to update Rust branch: %v", err)
		discord.SendError(webhookURL, "Rust Installation Failed", fmt.Sprintf("Failed to install Rust branch **%s**\n\n%s", branch, errMsg))
		return fmt.Errorf("%s", errMsg)
	}

	// Read new buildid
	newBuildID := ""
	if data, err := os.ReadFile(buildidPath); err == nil {
		newBuildID = string(data)
	}

	// Send success notification
	log.Printf("✓ Successfully installed Rust branch '%s'", branch)
	if oldBuildID == "" {
		discord.SendSuccess(webhookURL, "Rust Installation Complete",
			fmt.Sprintf("Rust branch **%s** installed successfully\n\nBuild ID: **%s**", branch, newBuildID))
	} else if oldBuildID != newBuildID {
		discord.SendSuccess(webhookURL, "Rust Update Complete",
			fmt.Sprintf("Rust branch **%s** updated\n\nFrom: **%s**\nTo: **%s**", branch, oldBuildID, newBuildID))
	}

	return nil
}

// getBranchLock gets or creates an RWMutex for a specific branch
func getBranchLock(branch string) *sync.RWMutex {
	branchMutex.Lock()
	defer branchMutex.Unlock()

	if lock, exists := branchLocks[branch]; exists {
		return lock
	}

	lock := &sync.RWMutex{}
	branchLocks[branch] = lock
	return lock
}

// AcquireReadLock acquires a read lock for a branch (used by syncServer)
// Returns an unlock function that must be called when done reading
func AcquireReadLock(branch string) func() {
	if branch == "" {
		branch = "main"
	}
	lock := getBranchLock(branch)
	lock.RLock()
	log.Printf("Acquired read lock for branch '%s'", branch)
	return func() {
		lock.RUnlock()
		log.Printf("Released read lock for branch '%s'", branch)
	}
}

// getRustInstallPath returns the installation path for a branch
func getRustInstallPath(branch string) string {
	return filepath.Join(RustInstallBase, branch)
}

// setupSteamCMD downloads and extracts steamcmd (shared installation)
func setupSteamCMD() error {
	// Check if steamcmd already exists
	steamcmdBinary := filepath.Join(SteamCMDBase, "steamcmd.sh")
	if _, err := os.Stat(steamcmdBinary); err == nil {
		log.Println("SteamCMD already installed")
		return nil
	}

	log.Println("Downloading SteamCMD...")

	// Create steamcmd directory
	if err := os.MkdirAll(SteamCMDBase, 0755); err != nil {
		return fmt.Errorf("failed to create steamcmd directory: %w", err)
	}

	// Download steamcmd
	tarPath := filepath.Join(RustInstallBase, "steamcmd_linux.tar.gz")
	if err := downloadFile(SteamCMDURL, tarPath); err != nil {
		return fmt.Errorf("failed to download steamcmd: %w", err)
	}

	log.Println("Extracting SteamCMD...")

	// Extract steamcmd
	cmd := exec.Command("tar", "-xzf", tarPath, "-C", SteamCMDBase)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to extract steamcmd: %w\nOutput: %s", err, output)
	}

	// Clean up tar file
	os.Remove(tarPath)

	log.Println("✓ SteamCMD installed")
	return nil
}

// updateRustBranch runs steamcmd to install/update Rust
func updateRustBranch(branch, installPath string) error {
	steamcmdBinary := filepath.Join(SteamCMDBase, "steamcmd.sh")

	// Determine branch options
	branchOpts := getBranchOpts(branch)

	log.Printf("Running steamcmd to install Rust (branch: %s)...", branch)

	// Run command with retries
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		log.Printf("Attempt %d/%d...", i+1, maxRetries)

		// Build steamcmd command fresh each attempt (exec.Cmd cannot be reused)
		// +force_install_dir <path> +login anonymous +app_update 258550 <branch_opts> validate +quit
		cmd := exec.Command(steamcmdBinary,
			"+force_install_dir", installPath,
			"+login", "anonymous",
			"+app_update", RustAppID)

		// Add branch opts if any
		if branchOpts != "" {
			cmd.Args = append(cmd.Args, strings.Fields(branchOpts)...)
		}

		cmd.Args = append(cmd.Args, "validate", "+quit")

		// Set environment to avoid terminal issues
		cmd.Env = append(os.Environ(), "TERM=xterm")

		output, err := cmd.CombinedOutput()
		if err == nil {
			log.Println("✓ Rust branch update complete")
			return trackBuildID(installPath)
		}

		log.Printf("Attempt %d failed: %v", i+1, err)
		if i < maxRetries-1 {
			log.Println("Retrying...")
		} else {
			return fmt.Errorf("failed to update branch after %d attempts: %w\nOutput: %s", maxRetries, err, output)
		}
	}

	return nil
}

// getBranchOpts returns steamcmd branch options based on branch name
func getBranchOpts(branch string) string {
	if branch == "" || branch == "main" {
		return "-beta public"
	}
	return fmt.Sprintf("-beta %s", branch)
}

// trackBuildID reads and stores the current build ID
func trackBuildID(installPath string) error {
	manifestPath := filepath.Join(installPath, "steamapps", "appmanifest_258550.acf")
	buildidPath := filepath.Join(installPath, "buildid")

	// Read manifest to get build ID
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Printf("Warning: Could not read manifest file: %v", err)
		return nil // Not critical
	}

	// Extract buildid from manifest
	// Format: "buildid"		"12345678"
	lines := strings.Split(string(data), "\n")
	var buildid string
	for _, line := range lines {
		if strings.Contains(line, "buildid") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				buildid = strings.Trim(parts[1], "\"")
				break
			}
		}
	}

	if buildid == "" {
		log.Println("Warning: Could not extract buildid from manifest")
		return nil
	}

	// Write buildid
	if err := os.WriteFile(buildidPath, []byte(buildid), 0644); err != nil {
		log.Printf("Warning: Could not write buildid: %v", err)
	} else {
		log.Printf("Build ID: %s", buildid)
	}

	return nil
}

// isRustInstalled checks if a Rust installation exists
func isRustInstalled(path string) bool {
	// Check if RustDedicated binary exists
	rustBinary := filepath.Join(path, "RustDedicated")
	_, err := os.Stat(rustBinary)
	return err == nil
}

// CheckForUpdates checks if a branch has updates available
func CheckForUpdates(branch, webhookURL string) (bool, string, error) {
	installPath := getRustInstallPath(branch)

	// Check if branch is installed
	if !isRustInstalled(installPath) {
		return false, "", nil
	}

	// Get current installed build ID
	buildidPath := filepath.Join(installPath, "buildid")
	currentBuildData, err := os.ReadFile(buildidPath)
	if err != nil {
		log.Printf("Warning: Could not read current buildid for %s: %v", branch, err)
		return false, "", nil
	}
	currentBuildID := strings.TrimSpace(string(currentBuildData))

	// Get latest build ID from Steam
	latestBuildID, err := getLatestBuildID(branch)
	if err != nil {
		log.Printf("Error checking for updates for branch %s: %v", branch, err)
		return false, "", err
	}

	// Compare build IDs
	if currentBuildID != latestBuildID {
		log.Printf("Update available for branch %s: %s -> %s", branch, currentBuildID, latestBuildID)

		// Send notification
		discord.SendInfo(webhookURL, "Rust Update Available",
			fmt.Sprintf("Rust branch **%s** has an update available\n\nCurrent: **%s**\nAvailable: **%s**",
				branch, currentBuildID, latestBuildID))

		return true, latestBuildID, nil
	}

	return false, currentBuildID, nil
}

// getLatestBuildID queries Steam for the latest build ID of a branch
func getLatestBuildID(branch string) (string, error) {
	steamcmdBinary := filepath.Join(SteamCMDBase, "steamcmd.sh")

	// Determine branch parameter for steamcmd
	branchParam := "public"
	if branch != "" && branch != "main" {
		branchParam = branch
	}

	// Run: steamcmd +login anonymous +app_info_update 1 +app_info_print 258550 +quit
	cmd := exec.Command(steamcmdBinary,
		"+login", "anonymous",
		"+app_info_update", "1",
		"+app_info_print", RustAppID,
		"+quit")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run steamcmd: %w", err)
	}

	// Parse output to find buildid for the specific branch
	buildID, err := parseBuildIDFromAppInfo(string(output), branchParam)
	if err != nil {
		return "", fmt.Errorf("failed to parse buildid: %w", err)
	}

	return buildID, nil
}

// parseBuildIDFromAppInfo extracts the build ID for a specific branch from app_info_print output
func parseBuildIDFromAppInfo(output, branch string) (string, error) {
	lines := strings.Split(output, "\n")

	// Look for the branch section and extract buildid
	// Format is nested like:
	// "branches"
	// {
	//   "public"
	//   {
	//     "buildid"    "12345678"
	//   }
	// }

	inBranches := false
	inTargetBranch := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Find branches section
		if strings.Contains(trimmed, `"branches"`) {
			inBranches = true
			continue
		}

		// Find our specific branch
		if inBranches && strings.Contains(trimmed, fmt.Sprintf(`"%s"`, branch)) {
			inTargetBranch = true
			continue
		}

		// Extract buildid from the branch section
		if inTargetBranch && strings.Contains(trimmed, `"buildid"`) {
			// Parse: "buildid"    "12345678"
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				buildID := strings.Trim(parts[1], `"`)
				return buildID, nil
			}
		}

		// Exit branch section when we hit a closing brace at the same level
		if inTargetBranch && trimmed == "}" {
			// Check if next non-empty line is also a brace (end of branches section)
			for j := i + 1; j < len(lines); j++ {
				nextTrimmed := strings.TrimSpace(lines[j])
				if nextTrimmed != "" {
					if nextTrimmed == "}" {
						inBranches = false
					}
					break
				}
			}
			inTargetBranch = false
		}
	}

	return "", fmt.Errorf("buildid not found for branch %s", branch)
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
