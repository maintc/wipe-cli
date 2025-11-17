package executor

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/maintc/wipe-cli/internal/config"
	"github.com/maintc/wipe-cli/internal/discord"
)

var (
	HookScriptPath         = "/opt/wiped/pre-start-hook.sh"
	StopServersScriptPath  = "/opt/wiped/stop-servers.sh"
	StartServersScriptPath = "/opt/wiped/start-servers.sh"
	GenerateMapsScriptPath = "/opt/wiped/generate-maps.sh"
)

// EnsureHookScript creates the pre-start hook script if it doesn't exist
func EnsureHookScript() error {
	hookDir := filepath.Dir(HookScriptPath)
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		return fmt.Errorf("failed to create hook directory: %w", err)
	}

	// Check if script already exists
	if _, err := os.Stat(HookScriptPath); err == nil {
		return nil
	}

	content := `#!/bin/bash
# Pre-start Hook Script
# 
# This script is executed once after all servers have been synced
# but before any servers are started back up.
#
# Arguments passed to this script:
#   $@ - Space-separated list of server paths involved in this event
#
# Example:
#   /var/www/servers/us-weekly /var/www/servers/eu-monthly
#
# You can add any custom logic here that should run before servers start.
# For example: clearing caches, updating plugins, sending notifications, etc.

SERVER_PATHS="$@"

echo "Pre-start hook executed for servers: $SERVER_PATHS"

# Add your custom logic below this line
# ...
`

	if err := os.WriteFile(HookScriptPath, []byte(content), 0755); err != nil {
		return fmt.Errorf("failed to write hook script: %w", err)
	}

	log.Printf("Created pre-start hook script at %s", HookScriptPath)
	return nil
}

// EnsureWipeScripts creates the wipe management scripts if they don't exist
func EnsureWipeScripts() error {
	scriptsDir := filepath.Dir(StopServersScriptPath)
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		return fmt.Errorf("failed to create scripts directory: %w", err)
	}

	// Ensure stop-servers.sh
	if err := ensureStopServersScript(); err != nil {
		return err
	}

	// Ensure start-servers.sh
	if err := ensureStartServersScript(); err != nil {
		return err
	}

	// Ensure generate-maps.sh
	if err := ensureGenerateMapsScript(); err != nil {
		return err
	}

	return nil
}

func ensureStopServersScript() error {
	// Check if script already exists
	if _, err := os.Stat(StopServersScriptPath); err == nil {
		return nil
	}

	content := `#!/bin/bash
# Stop Servers Script
#
# This script is called to stop Rust servers before performing updates/wipes.
#
# Arguments passed to this script:
#   $@ - Space-separated list of server paths
#
# Example:
#   /var/www/servers/us-weekly /var/www/servers/eu-monthly
#
# Customize this script to match your server management approach.

SERVER_PATHS="$@"

echo "Stopping servers for paths: $SERVER_PATHS"

for SERVER_PATH in $SERVER_PATHS; do
    # Extract server identity from path (e.g., us-weekly from /var/www/servers/us-weekly)
    IDENTITY=$(basename "$SERVER_PATH")
    
    echo "Stopping server: $IDENTITY (path: $SERVER_PATH)"
    
    # Add your server stop logic here
    # Examples:
    #   - systemctl stop rs-${IDENTITY}
    #   - docker stop ${IDENTITY}
    #   - kill $(cat ${SERVER_PATH}/server.pid)
    #   - your custom stop command
done

echo "✓ All servers stopped"
`

	if err := os.WriteFile(StopServersScriptPath, []byte(content), 0755); err != nil {
		return fmt.Errorf("failed to write stop-servers script: %w", err)
	}

	log.Printf("Created stop-servers script at %s", StopServersScriptPath)
	return nil
}

func ensureStartServersScript() error {
	// Check if script already exists
	if _, err := os.Stat(StartServersScriptPath); err == nil {
		return nil
	}

	content := `#!/bin/bash
# Start Servers Script
#
# This script is called to start Rust servers after performing updates/wipes.
#
# Arguments passed to this script:
#   $@ - Space-separated list of server paths
#
# Example:
#   /var/www/servers/us-weekly /var/www/servers/eu-monthly
#
# Customize this script to match your server management approach.

SERVER_PATHS="$@"

echo "Starting servers for paths: $SERVER_PATHS"

for SERVER_PATH in $SERVER_PATHS; do
    # Extract server identity from path (e.g., us-weekly from /var/www/servers/us-weekly)
    IDENTITY=$(basename "$SERVER_PATH")
    
    echo "Starting server: $IDENTITY (path: $SERVER_PATH)"
    
    # Add your server start logic here
    # Examples:
    #   - systemctl start rs-${IDENTITY}
    #   - docker start ${IDENTITY}
    #   - ${SERVER_PATH}/start.sh
    #   - your custom start command
done

echo "✓ All servers started"
`

	if err := os.WriteFile(StartServersScriptPath, []byte(content), 0755); err != nil {
		return fmt.Errorf("failed to write start-servers script: %w", err)
	}

	log.Printf("Created start-servers script at %s", StartServersScriptPath)
	return nil
}

func ensureGenerateMapsScript() error {
	// Check if script already exists
	if _, err := os.Stat(GenerateMapsScriptPath); err == nil {
		return nil
	}

	content := `#!/bin/bash
# Generate Maps Script
#
# This script is called to prepare maps for Rust servers before wipes.
# It runs 22 hours before a wipe event (configurable via map_generation_hours).
#
# Arguments passed to this script:
#   $@ - Space-separated list of server paths that need maps prepared
#
# Example:
#   /var/www/servers/us-weekly /var/www/servers/eu-monthly
#
# YOUR RESPONSIBILITIES:
#   1. Pick or generate a map (seed/size, custom map, etc.)
#   2. Update the server's server.cfg file with map settings:
#      - server.seed and server.size (for procedural maps)
#      - OR server.levelurl (for custom map providers)
#   3. Handle any map-related files as needed
#   4. Clean up any temporary files after the wipe completes
#   5. Exit with non-zero status on failure
#
# NOTE: This script is called BEFORE the wipe. The actual wipe process will:
#   - Stop servers
#   - Sync Rust/Carbon
#   - Delete map/save files
#   - Run pre-start-hook.sh
#   - Start servers
#
# You are responsible for updating server.cfg BEFORE the wipe or in pre-start-hook.sh

SERVER_PATHS="$@"

echo "Map preparation requested for paths: $SERVER_PATHS"

for SERVER_PATH in $SERVER_PATHS; do
    # Extract server identity from path (e.g., us-weekly from /var/www/servers/us-weekly)
    IDENTITY=$(basename "$SERVER_PATH")
    
    echo "Preparing map for: $IDENTITY (path: $SERVER_PATH)"
    
    # Add your map preparation logic here
    # Examples:
    #
    # Option 1: Pick random seed/size and update server.cfg
    #   SEED=$RANDOM
    #   SIZE=4250
    #   echo "server.seed \"$SEED\"" >> ${SERVER_PATH}/server/${IDENTITY}/cfg/server.cfg
    #   echo "server.size $SIZE" >> ${SERVER_PATH}/server/${IDENTITY}/cfg/server.cfg
    #
    # Option 2: Generate with a custom map generator and update server.cfg
    #   /usr/local/bin/map-generator --seed $SEED --size $SIZE --output ${SERVER_PATH}/maps
    #   LEVELURL=$(cat ${SERVER_PATH}/maps/level_url.txt)
    #   echo "server.levelurl \"$LEVELURL\"" >> ${SERVER_PATH}/server/${IDENTITY}/cfg/server.cfg
    #
    # Option 3: Do nothing, let server use default map
    #   echo "Using default map for $IDENTITY"
done

echo "✓ Map preparation complete"
`

	if err := os.WriteFile(GenerateMapsScriptPath, []byte(content), 0755); err != nil {
		return fmt.Errorf("failed to write generate-maps script: %w", err)
	}

	log.Printf("Created generate-maps script at %s", GenerateMapsScriptPath)
	return nil
}

// ExecuteEventBatch processes multiple servers together (mix of restarts and wipes)
func ExecuteEventBatch(servers []config.Server, wipeServers map[string]bool, webhookURL string, eventDelay int) error {
	wipeCount := len(wipeServers)
	restartCount := len(servers) - wipeCount

	log.Printf("Executing batch event for %d server(s): %d restart(s), %d wipe(s)", len(servers), restartCount, wipeCount)

	// Wait for configured delay
	if eventDelay > 0 {
		log.Printf("Waiting %d seconds before executing...", eventDelay)
		time.Sleep(time.Duration(eventDelay) * time.Second)
	}

	// Send Discord notification: Starting
	serverNames := make([]string, len(servers))
	for i, s := range servers {
		serverNames[i] = s.Name
	}
	discord.SendInfo(webhookURL, "Batch Event Starting",
		fmt.Sprintf("Starting batch event for **%d** server(s):\n• %s\n\n**%d restart(s), %d wipe(s)**",
			len(servers), strings.Join(serverNames, "\n• "), restartCount, wipeCount))

	// Step 1: Stop all servers at once
	serverPaths := make([]string, len(servers))
	for i, s := range servers {
		serverPaths[i] = s.Path
	}

	log.Printf("Stopping %d server(s)...", len(servers))
	if err := stopServers(serverPaths); err != nil {
		errMsg := fmt.Sprintf("Failed to stop servers: %v", err)
		log.Printf("Error: %s", errMsg)
		discord.SendError(webhookURL, "Batch Event Failed", errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	// Step 2: Update Rust and Carbon for all servers (in parallel)
	log.Printf("Updating Rust and Carbon on servers...")
	if err := SyncServers(servers); err != nil {
		errMsg := fmt.Sprintf("Failed to update servers: %v", err)
		log.Printf("Error: %s", errMsg)
		discord.SendError(webhookURL, "Batch Event Failed", errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	// Step 3: Wipe data for wipe-servers only
	if len(wipeServers) > 0 {
		log.Printf("Performing wipe cleanup for %d server(s)...", len(wipeServers))
		for _, server := range servers {
			if wipeServers[server.Path] {
				log.Printf("  Wiping data for %s", server.Name)
				if err := wipeServerData(server); err != nil {
					errMsg := fmt.Sprintf("Failed to wipe data for server %s: %v", server.Name, err)
					log.Printf("Error: %s", errMsg)
					discord.SendError(webhookURL, "Batch Event Failed", errMsg)
					return fmt.Errorf("%s", errMsg)
				}
			}
		}
	}

	// Step 4: Run pre-start hook once with all server paths
	if err := runPreStartHook(serverPaths); err != nil {
		log.Printf("Warning: Pre-start hook failed: %v", err)
		// Don't fail the entire operation if hook fails
	}

	// Step 5: Start all servers at once
	log.Printf("Starting %d server(s)...", len(servers))
	if err := startServers(serverPaths); err != nil {
		errMsg := fmt.Sprintf("Failed to start servers: %v", err)
		log.Printf("Error: %s", errMsg)
		discord.SendError(webhookURL, "Batch Event Failed", errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	// Success notification
	discord.SendSuccess(webhookURL, "Batch Event Complete",
		fmt.Sprintf("Successfully completed batch event for **%d** server(s):\n• %s\n\n**%d restart(s), %d wipe(s)**",
			len(servers), strings.Join(serverNames, "\n• "), restartCount, wipeCount))

	log.Printf("✓ Batch event completed successfully")
	return nil
}

// stopServers stops servers via stop-servers.sh
func stopServers(serverPaths []string) error {
	// Check if script exists
	if _, err := os.Stat(StopServersScriptPath); err != nil {
		return fmt.Errorf("stop-servers.sh not found at %s", StopServersScriptPath)
	}

	cmd := exec.Command(StopServersScriptPath, serverPaths...)
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("stop script failed: %w", err)
	}

	return nil
}

// startServers starts servers via start-servers.sh
func startServers(serverPaths []string) error {
	// Check if script exists
	if _, err := os.Stat(StartServersScriptPath); err != nil {
		return fmt.Errorf("start-servers.sh not found at %s", StartServersScriptPath)
	}

	cmd := exec.Command(StartServersScriptPath, serverPaths...)
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("start script failed: %w", err)
	}

	return nil
}

// SyncServers updates Rust and Carbon installations on multiple servers in parallel
func SyncServers(servers []config.Server) error {
	type result struct {
		server config.Server
		err    error
	}

	results := make(chan result, len(servers))
	var wg sync.WaitGroup

	// Launch parallel sync operations
	for _, server := range servers {
		wg.Add(1)
		go func(s config.Server) {
			defer wg.Done()
			err := syncServer(s)
			results <- result{server: s, err: err}
		}(server)
	}

	// Wait for all syncs to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results and check for errors
	var errors []string
	for res := range results {
		if res.err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", res.server.Name, res.err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to update servers:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}

// syncServer updates Rust and Carbon installations on the server
func syncServer(server config.Server) error {
	log.Printf("Updating server: %s", server.Name)

	// Determine source paths based on branch
	rustSource := filepath.Join("/opt/rust", server.Branch)
	carbonSource := filepath.Join("/opt/carbon", server.Branch)
	if server.Branch == "" {
		rustSource = filepath.Join("/opt/rust", "main")
		carbonSource = filepath.Join("/opt/carbon", "main")
	}

	// Update Rust
	log.Printf("  Updating Rust from %s to %s", rustSource, server.Path)

	// Remove old Rust files first
	rustCleanupDirs := []string{
		filepath.Join(server.Path, "RustDedicated_Data"),
		filepath.Join(server.Path, "Bundles"),
		filepath.Join(server.Path, "steamapps"),
		filepath.Join(server.Path, "steamcmd"),
	}
	for _, dir := range rustCleanupDirs {
		if err := os.RemoveAll(dir); err != nil {
			log.Printf("  Warning: Failed to remove %s: %v", dir, err)
		}
	}

	// Rsync Rust (safe mode: uses temp files for atomic updates)
	rsyncCmd := exec.Command("rsync", "-a", fmt.Sprintf("%s/", rustSource), fmt.Sprintf("%s/", server.Path))
	output, err := rsyncCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rust rsync failed: %w\nOutput: %s", err, output)
	}

	// Update Carbon
	log.Printf("  Updating Carbon from %s to %s", carbonSource, server.Path)

	// Remove old Carbon files first
	carbonCleanupDirs := []string{
		filepath.Join(server.Path, "carbon", "native"),
		filepath.Join(server.Path, "carbon", "managed"),
		filepath.Join(server.Path, "carbon", "tools"),
	}
	for _, dir := range carbonCleanupDirs {
		if err := os.RemoveAll(dir); err != nil {
			log.Printf("  Warning: Failed to remove %s: %v", dir, err)
		}
	}

	// Rsync Carbon (safe mode: uses temp files for atomic updates)
	rsyncCmd = exec.Command("rsync", "-a", fmt.Sprintf("%s/", carbonSource), fmt.Sprintf("%s/", server.Path))
	output, err = rsyncCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("carbon rsync failed: %w\nOutput: %s", err, output)
	}

	log.Printf("  ✓ Updated %s", server.Name)
	return nil
}

// wipeServerData deletes map/save files for a wipe event
func wipeServerData(server config.Server) error {
	log.Printf("Wiping data for server: %s", server.Name)

	// Extract server identity from path (last component)
	identity := filepath.Base(server.Path)
	serverDataPath := filepath.Join(server.Path, "server", identity)

	log.Printf("  Server data path: %s", serverDataPath)

	// Patterns to delete
	patterns := []string{
		"*.map",
		"*.sav*",
		"player.states.*.db*",
		"sv.files.*.db*",
	}

	// Conditionally add blueprints
	if server.WipeBlueprints {
		log.Printf("  Including blueprints in wipe")
		patterns = append(patterns, "player.blueprints.*")
	}

	// Delete matching files
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(serverDataPath, pattern))
		if err != nil {
			log.Printf("  Warning: Failed to glob pattern %s: %v", pattern, err)
			continue
		}

		for _, match := range matches {
			log.Printf("  Deleting: %s", match)
			if err := os.Remove(match); err != nil {
				log.Printf("  Warning: Failed to delete %s: %v", match, err)
			}
		}
	}

	log.Printf("  ✓ Wiped data for %s", server.Name)
	return nil
}

// runPreStartHook executes the pre-start hook script with server paths as arguments
func runPreStartHook(serverPaths []string) error {
	log.Printf("Running pre-start hook: %s", HookScriptPath)

	cmd := exec.Command(HookScriptPath, serverPaths...)
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hook script failed: %w", err)
	}

	return nil
}
