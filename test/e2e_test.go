package test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/maintc/wipe-cli/internal/carbon"
	"github.com/maintc/wipe-cli/internal/config"
	"github.com/maintc/wipe-cli/internal/daemon"
	"github.com/maintc/wipe-cli/internal/steamcmd"
	"github.com/spf13/viper"
)

// TestE2E_FullIntegration is a complete end-to-end test that:
// - Downloads real Rust and Carbon installations
// - Creates real production server directories (us-weekly, us-long, us-build, train)
// - Runs a local calendar server
// - Executes the full daemon lifecycle
// - Verifies mixed restart/wipe batch execution
// - Tests: us-weekly & us-long (restart), us-build & train (wipe)
//
// This test requires:
// - Internet connection (to download Rust/Carbon)
// - ~15GB disk space (for Rust installation)
// - Write access to E2E_SERVER_PATH (default: /var/www/servers)
// - Root/sudo access (for systemd operations)
// - ~5-10 minutes to complete
//
// Environment variables:
// - E2E_TEST=1 (required to run)
// - E2E_SERVER_PATH (default: /var/www/servers)
// - E2E_DISCORD_WEBHOOK (optional)
func TestE2E_FullIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Check for E2E flag
	if os.Getenv("E2E_TEST") != "1" {
		t.Skip("Skipping E2E test. Set E2E_TEST=1 to run")
	}

	// Load .env file from test directory if it exists (ignore errors)
	_ = godotenv.Load(filepath.Join(".", ".env"))

	t.Log("=== Starting Full E2E Integration Test ===")

	// Setup test environment
	testDir, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Create or connect to calendar server
	calendarBaseURL := os.Getenv("E2E_CALENDAR_URL")
	var calendarServer *CalendarServer
	var ownCalendarServer bool

	if calendarBaseURL != "" {
		// Use existing calendar server
		t.Logf("Using existing calendar server at: %s", calendarBaseURL)
		// Create a wrapper that points to the existing server
		calendarServer = NewRemoteCalendarServer(t, calendarBaseURL)
		ownCalendarServer = false
	} else {
		// Create new calendar server for this test
		calendarServer = NewCalendarServer(t)
		ownCalendarServer = true
		defer calendarServer.Close()
		t.Logf("Created calendar server at: %s", calendarServer.BaseURL())
	}

	// Get Discord webhook from env (optional)
	discordWebhook := os.Getenv("E2E_DISCORD_WEBHOOK")
	if discordWebhook == "" {
		t.Logf("No Discord webhook configured (E2E_DISCORD_WEBHOOK not set) - notifications disabled")
	}

	// Get server base path from env or use default
	serverBasePath := os.Getenv("E2E_SERVER_PATH")
	if serverBasePath == "" {
		serverBasePath = "/var/www/servers"
		t.Logf("Using default server base path: %s", serverBasePath)
	}

	// Install Rust and Carbon (one-time setup)
	ensureRustInstalled(t)
	ensureCarbonInstalled(t)

	// Create test servers
	servers := createTestServers(t, serverBasePath, calendarServer)

	// Create test config
	configPath := createTestConfig(t, testDir, servers, discordWebhook)

	t.Logf("Test config created at: %s", configPath)

	// Set custom config path
	config.CustomConfigPath = configPath
	config.InitConfig()

	// Set locale environment variables for Ansible compatibility in hook scripts
	// Use C.UTF-8 which is available on this system
	os.Setenv("LANG", "C.UTF-8")
	os.Setenv("LC_ALL", "C.UTF-8")

	// Create daemon instance
	d := daemon.New()

	// Start daemon in background
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	daemonDone := make(chan error, 1)
	go func() {
		t.Log("Starting daemon...")
		daemonDone <- d.Run(ctx)
	}()

	// Wait for daemon to initialize
	time.Sleep(5 * time.Second)
	t.Log("Daemon initialized")

	// Clear any existing events if using remote server
	if !ownCalendarServer {
		t.Log("Clearing existing events from remote calendar server...")
		calendarServer.ClearAllEvents()
	}

	// Add events to calendar (all events at T+90s)
	scheduleTestEvents(t, calendarServer, servers)

	// SIMULATE_CLOSE_RUST_UPDATE: Delete Rust and trigger update IMMEDIATELY to test race condition
	if os.Getenv("SIMULATE_CLOSE_RUST_UPDATE") == "1" {
		t.Log("=== SIMULATING CLOSE RUST UPDATE (Race Condition Test) ===")
		t.Logf("Deleting /opt/rust/main and steamcmd cache to force full re-download...")
		if err := os.RemoveAll("/opt/rust/main"); err != nil {
			t.Fatalf("Failed to delete /opt/rust/main: %v", err)
		}
		// Also delete steamcmd cache to force full re-download (~8GB, takes 1-2 minutes)
		if err := os.RemoveAll("/opt/rust/steamcmd"); err != nil {
			t.Fatalf("Failed to delete /opt/rust/steamcmd: %v", err)
		}

		t.Logf("Triggering Rust update check (this will take >1 minute and overlap with event execution)...")
		go func() {
			// Trigger update in background - this simulates the daemon's checkForUpdates
			if err := steamcmd.InstallRustBranch("main", ""); err != nil {
				t.Logf("Warning: Rust install failed during simulation: %v", err)
			} else {
				t.Logf("✓ Simulated Rust update completed")
			}
		}()

		// Give it a moment to start acquiring the write lock
		time.Sleep(2 * time.Second)
		t.Logf("Rust update started and holding WRITE LOCK on branch 'main'")
		t.Logf("Event will fire in ~90s while update is still in progress...")
		t.Logf("When syncServer runs, it should BLOCK waiting for the write lock to release")
	}

	// Wait for events to be detected (20s buffer for daemon to pick them up)
	t.Log("Waiting for events to be scheduled...")
	time.Sleep(20 * time.Second)

	// Wait for batch event to execute (restart + wipe together)
	// 90s until event + 30s for execution to complete = 120s total, minus 22s already waited = 98s
	t.Log("Waiting for batch event to execute (2 restart, 2 wipe)...")
	t.Log("Events scheduled for ~90s from start, waiting...")
	time.Sleep(98 * time.Second)

	// Verify all servers were updated
	t.Log("Verifying all servers were updated...")
	verifyServersUpdated(t, servers)

	// Verify wipe servers had files deleted
	t.Log("Verifying wipe files were deleted...")
	verifyServersWiped(t, servers)

	// Verify future events are still in calendar (not affected by current event execution)
	t.Log("Verifying future events still exist in calendar...")
	verifyFutureEvents(t, calendarServer)

	// Wait a bit longer for Ansible to complete in background
	t.Log("Waiting for Ansible to complete...")
	time.Sleep(60 * time.Second)

	// Stop daemon
	t.Log("Stopping daemon...")
	cancel()

	select {
	case err := <-daemonDone:
		if err != nil && err != context.Canceled {
			t.Errorf("Daemon error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Error("Daemon didn't shut down cleanly")
	}

	// Clean up calendar events
	t.Log("Cleaning up calendar events...")
	calendarServer.ClearAllEvents()

	t.Log("=== E2E Test Complete ===")
}

// setupTestEnvironment creates a temporary test directory
func setupTestEnvironment(t *testing.T) (string, func()) {
	testDir, err := os.MkdirTemp("", "wipe-cli-e2e-*")
	if err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	t.Logf("Test directory: %s", testDir)

	cleanup := func() {
		t.Logf("Cleaning up test directory: %s", testDir)
		os.RemoveAll(testDir)
	}

	return testDir, cleanup
}

// ensureRustInstalled checks if Rust is installed, installs if needed
func ensureRustInstalled(t *testing.T) {
	rustPath := "/opt/rust/main"
	if _, err := os.Stat(filepath.Join(rustPath, "RustDedicated")); err == nil {
		t.Logf("Rust already installed at %s", rustPath)
		return
	}

	t.Log("Rust not found, installing... (this may take 5-10 minutes)")

	if err := steamcmd.EnsureRustBranchInstalled("main", ""); err != nil {
		t.Fatalf("Failed to install Rust: %v", err)
	}

	t.Log("✓ Rust installation complete")
}

// ensureCarbonInstalled checks if Carbon is installed, installs if needed
func ensureCarbonInstalled(t *testing.T) {
	carbonPath := "/opt/carbon/main"
	if _, err := os.Stat(carbonPath); err == nil {
		t.Logf("Carbon already installed at %s", carbonPath)
		return
	}

	t.Log("Carbon not found, installing...")

	if err := carbon.EnsureCarbonInstalled("main", ""); err != nil {
		t.Fatalf("Failed to install Carbon: %v", err)
	}

	t.Log("✓ Carbon installation complete")
}

// createTestServers creates test server directories
func createTestServers(t *testing.T, serverBasePath string, calendarServer *CalendarServer) []config.Server {
	servers := []config.Server{
		{
			Name:           "us-weekly",
			Path:           filepath.Join(serverBasePath, "us-weekly"),
			CalendarURL:    calendarServer.GetServerURL("us-weekly"),
			Branch:         "main",
			WipeBlueprints: false,
			GenerateMap:    true, // Won't trigger since restarting, not wiping
		},
		{
			Name:           "us-long",
			Path:           filepath.Join(serverBasePath, "us-long"),
			CalendarURL:    calendarServer.GetServerURL("us-long"),
			Branch:         "main",
			WipeBlueprints: false,
			GenerateMap:    true, // Won't trigger since restarting, not wiping
		},
		{
			Name:           "us-build",
			Path:           filepath.Join(serverBasePath, "us-build"),
			CalendarURL:    calendarServer.GetServerURL("us-build"),
			Branch:         "main",
			WipeBlueprints: false,
			GenerateMap:    false,
		},
		{
			Name:           "train",
			Path:           filepath.Join(serverBasePath, "train"),
			CalendarURL:    calendarServer.GetServerURL("train"),
			Branch:         "main",
			WipeBlueprints: false,
			GenerateMap:    false,
		},
	}

	for _, server := range servers {
		// Create server directory structure (hook scripts will provision the rest)
		if err := os.MkdirAll(server.Path, 0755); err != nil {
			t.Fatalf("Failed to create server dir: %v", err)
		}

		t.Logf("Created test server: %s at %s", server.Name, server.Path)
	}

	return servers
}

// createTestConfig creates a test configuration file
func createTestConfig(t *testing.T, testDir string, servers []config.Server, discordWebhook string) string {
	configPath := filepath.Join(testDir, "config.yaml")

	// Create config using viper
	v := viper.New()
	v.Set("lookahead_hours", 1) // Short lookahead for testing
	v.Set("check_interval", 10) // Check every 10 seconds
	v.Set("event_delay", 5)
	v.Set("discord_webhook", discordWebhook)
	v.Set("map_generation_hours", 1)
	v.Set("servers", servers)

	if err := v.WriteConfigAs(configPath); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	return configPath
}

// scheduleTestEvents adds test events to the calendar
func scheduleTestEvents(t *testing.T, cs *CalendarServer, servers []config.Server) {
	// Schedule CURRENT events 90 seconds from now (all in same batch)
	// Extra buffer ensures gocron doesn't skip as "past event" during race condition test
	currentEventTime := time.Now().Add(90 * time.Second)

	// Restart events for us-weekly and us-long (just "restart" as summary)
	cs.AddEventForServer("us-weekly", "restart-1", "restart", currentEventTime)
	cs.AddEventForServer("us-long", "restart-1", "restart", currentEventTime)

	// Wipe events for us-build and train (just "wipe" as summary)
	cs.AddEventForServer("us-build", "wipe-1", "wipe", currentEventTime)
	cs.AddEventForServer("train", "wipe-1", "wipe", currentEventTime)

	// Schedule FUTURE events (61 minutes from now - just outside lookahead initially)
	// At T+0s: lookahead is 0-60m, so events at 61m are NOT visible
	// At T+60s: lookahead is 1m-61m, so events at 61m ARE visible (edge case test)
	// This simulates events appearing in lookahead RIGHT when current events execute
	// Note: us-weekly and us-long get RESTART events to avoid triggering map generation
	futureEventTime := time.Now().Add(61 * time.Minute)

	cs.AddEventForServer("us-weekly", "restart-future", "restart", futureEventTime)
	cs.AddEventForServer("us-long", "restart-future", "restart", futureEventTime)
	cs.AddEventForServer("us-build", "wipe-future", "wipe", futureEventTime)
	cs.AddEventForServer("train", "wipe-future", "wipe", futureEventTime)

	t.Logf("Current events scheduled: 2 restart(s), 2 wipe(s) at T+90s")
	t.Logf("Future events scheduled: 2 restart(s), 2 wipe(s) at T+61m (outside initial lookahead, appear when current events execute)")
}

// verifyServersUpdated checks that servers were updated (Rust/Carbon synced)
func verifyServersUpdated(t *testing.T, servers []config.Server) {
	for _, server := range servers {
		// Check if RustDedicated exists (proves rsync worked)
		rustBinary := filepath.Join(server.Path, "RustDedicated")
		if _, err := os.Stat(rustBinary); os.IsNotExist(err) {
			t.Errorf("Server %s not updated - RustDedicated missing", server.Name)
		} else {
			t.Logf("✓ Server %s updated successfully", server.Name)
		}
	}
}

// verifyServersWiped checks that wipe files were deleted for wipe events
func verifyServersWiped(t *testing.T, servers []config.Server) {
	// Check us-build and train (the wipe servers)
	wipeServers := []string{"us-build", "train"}

	// Check for wipe files (excluding .map since hook scripts may regenerate them)
	wipeFilePatterns := []string{
		"*.sav",
		"*.sav.bak",
		"player.states.*.db*",
		"sv.files.*.db*",
	}

	for _, server := range servers {
		// Skip servers that weren't wiped
		isWipeServer := false
		for _, wipeName := range wipeServers {
			if server.Name == wipeName {
				isWipeServer = true
				break
			}
		}

		if !isWipeServer {
			continue
		}

		serverDataDir := filepath.Join(server.Path, "server", server.Name)

		// Check if any wipe files still exist
		foundWipeFiles := []string{}
		for _, pattern := range wipeFilePatterns {
			matches, err := filepath.Glob(filepath.Join(serverDataDir, pattern))
			if err != nil {
				t.Logf("Warning: Failed to glob pattern %s: %v", pattern, err)
				continue
			}
			foundWipeFiles = append(foundWipeFiles, matches...)
		}

		if len(foundWipeFiles) > 0 {
			t.Errorf("Wipe files still exist for %s: %v", server.Name, foundWipeFiles)
		} else {
			t.Logf("✓ All wipe files deleted for %s", server.Name)
		}
	}
}

// verifyFutureEvents checks that future events are still in the calendar after current events executed
func verifyFutureEvents(t *testing.T, cs *CalendarServer) {
	// This simulates the Nov 16 scenario where future events should remain
	// after current events are removed from the calendar

	// Check each server has a future event still scheduled
	// us-weekly and us-long should have future restarts
	// us-build and train should have future wipes
	serverExpectedEvents := map[string]string{
		"us-weekly": "restart-future",
		"us-long":   "restart-future",
		"us-build":  "wipe-future",
		"train":     "wipe-future",
	}

	for serverName, expectedEventID := range serverExpectedEvents {
		// Make HTTP request to check events for this server
		resp, err := http.Get(fmt.Sprintf("%s/list-events?server=%s", cs.BaseURL(), serverName))
		if err != nil {
			t.Errorf("Failed to list events for %s: %v", serverName, err)
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Errorf("Failed to read response for %s: %v", serverName, err)
			continue
		}

		// Parse the JSON response
		var result struct {
			Server string `json:"server"`
			Count  int    `json:"count"`
			Events []struct {
				ID        string `json:"id"`
				Summary   string `json:"summary"`
				StartTime string `json:"start_time"`
			} `json:"events"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			t.Errorf("Failed to parse events for %s: %v", serverName, err)
			continue
		}

		// Should have at least 1 event (might still have past event if calendar hasn't refreshed)
		if result.Count < 1 {
			t.Errorf("Server %s should have at least 1 future event, has %d", serverName, result.Count)
			continue
		}

		// Verify the expected future event exists (the key test for Nov 16 scenario)
		hasExpectedEvent := false
		for _, event := range result.Events {
			if event.ID == expectedEventID {
				hasExpectedEvent = true
				break
			}
		}

		if !hasExpectedEvent {
			t.Errorf("Server %s missing future event (should have %s)", serverName, expectedEventID)
			continue
		}

		t.Logf("✓ Server %s has future event still scheduled", serverName)
	}

	t.Log("✓ All future events preserved (Nov 16 scenario prevented)")
}
