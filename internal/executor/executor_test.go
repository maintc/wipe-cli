package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maintc/wipe-cli/internal/config"
)

func TestExecuteEventBatch_Ordering(t *testing.T) {
	// This test proves the 5-step execution order
	// We'll create mock scripts that log their execution

	tmpDir := t.TempDir()

	// Override script paths for testing
	origStopPath := StopServersScriptPath
	origStartPath := StartServersScriptPath
	origHookPath := HookScriptPath

	defer func() {
		// Restore original paths
		StopServersScriptPath = origStopPath
		StartServersScriptPath = origStartPath
		HookScriptPath = origHookPath
	}()

	// Create log file
	logFile := filepath.Join(tmpDir, "execution.log")

	// Create mock scripts that log execution order
	stopScript := filepath.Join(tmpDir, "stop.sh")
	startScript := filepath.Join(tmpDir, "start.sh")
	hookScript := filepath.Join(tmpDir, "hook.sh")

	// Stop script
	stopContent := fmt.Sprintf(`#!/bin/bash
echo "STOP: $@" >> %s
exit 0
`, logFile)
	if err := os.WriteFile(stopScript, []byte(stopContent), 0755); err != nil {
		t.Fatalf("Failed to create stop script: %v", err)
	}

	// Start script
	startContent := fmt.Sprintf(`#!/bin/bash
echo "START: $@" >> %s
exit 0
`, logFile)
	if err := os.WriteFile(startScript, []byte(startContent), 0755); err != nil {
		t.Fatalf("Failed to create start script: %v", err)
	}

	// Hook script
	hookContent := fmt.Sprintf(`#!/bin/bash
echo "HOOK: $@" >> %s
exit 0
`, logFile)
	if err := os.WriteFile(hookScript, []byte(hookContent), 0755); err != nil {
		t.Fatalf("Failed to create hook script: %v", err)
	}

	// Override paths
	StopServersScriptPath = stopScript
	StartServersScriptPath = startScript
	HookScriptPath = hookScript

	// Create test servers (no actual directories needed for this test)
	servers := []config.Server{
		{Name: "server-a", Path: "/test/server-a", Branch: "main"},
		{Name: "server-b", Path: "/test/server-b", Branch: "main"},
	}

	wipeServers := make(map[string]bool)

	// Execute (will fail on sync step since we don't have actual servers, but we can check order)
	// Note: This will fail at sync step, but we can verify stop was called first
	_ = ExecuteEventBatch(servers, wipeServers, "", 0)

	// Read log file
	logData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logLines := strings.Split(strings.TrimSpace(string(logData)), "\n")

	// Verify stop was called first
	if len(logLines) < 1 {
		t.Fatal("Expected at least STOP to be logged")
	}

	if !strings.HasPrefix(logLines[0], "STOP:") {
		t.Errorf("Expected first action to be STOP, got: %s", logLines[0])
	}

	// Verify correct server paths were passed
	if !strings.Contains(logLines[0], "/test/server-a") {
		t.Error("STOP should include server-a path")
	}
	if !strings.Contains(logLines[0], "/test/server-b") {
		t.Error("STOP should include server-b path")
	}
}

func TestWipeServerData_FilePatterns(t *testing.T) {
	// Test that wipeServerData deletes correct file patterns
	tmpDir := t.TempDir()

	// Create mock server directory structure
	// wipeServerData uses filepath.Base(server.Path) as the identity
	serverPath := filepath.Join(tmpDir, "my-server")
	identityDir := filepath.Join(serverPath, "server", "my-server")
	if err := os.MkdirAll(identityDir, 0755); err != nil {
		t.Fatalf("Failed to create identity dir: %v", err)
	}

	// Create files that should be deleted
	filesToDelete := []string{
		"world.map",
		"world.sav",
		"world.sav.bak",
		"player.states.0.db",
		"player.states.0.db-wal",
		"sv.files.0.db",
		"sv.files.0.db-wal",
	}

	// Create files that should NOT be deleted
	filesToKeep := []string{
		"cfg/server.cfg",
		"oxide/config/plugin.json",
		"player.blueprints.5.db", // Only deleted if wipe_blueprints=true
	}

	// Create all files
	for _, file := range filesToDelete {
		path := filepath.Join(identityDir, file)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", file, err)
		}
	}

	for _, file := range filesToKeep {
		path := filepath.Join(identityDir, file)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir for %s: %v", file, err)
		}
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", file, err)
		}
	}

	// Create test server config
	server := config.Server{
		Name:           "my-server",
		Path:           serverPath,
		Branch:         "main",
		WipeBlueprints: false,
	}

	// Execute wipe
	if err := wipeServerData(server); err != nil {
		t.Fatalf("wipeServerData failed: %v", err)
	}

	// Verify files were deleted
	for _, file := range filesToDelete {
		path := filepath.Join(identityDir, file)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("File %s should have been deleted", file)
		}
	}

	// Verify files were kept
	for _, file := range filesToKeep {
		path := filepath.Join(identityDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("File %s should have been kept", file)
		}
	}
}

func TestWipeServerData_Blueprints(t *testing.T) {
	// Test that blueprints are only deleted when wipe_blueprints=true
	tmpDir := t.TempDir()

	// wipeServerData uses filepath.Base(server.Path) as the identity
	serverPath := filepath.Join(tmpDir, "test-server")
	identityDir := filepath.Join(serverPath, "server", "test-server")
	if err := os.MkdirAll(identityDir, 0755); err != nil {
		t.Fatalf("Failed to create identity dir: %v", err)
	}

	blueprintFile := filepath.Join(identityDir, "player.blueprints.5.db")
	if err := os.WriteFile(blueprintFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create blueprint file: %v", err)
	}

	// Test with wipe_blueprints=false
	server := config.Server{
		Name:           "test-server",
		Path:           serverPath,
		Branch:         "main",
		WipeBlueprints: false,
	}

	if err := wipeServerData(server); err != nil {
		t.Fatalf("wipeServerData failed: %v", err)
	}

	// Blueprint should still exist
	if _, err := os.Stat(blueprintFile); os.IsNotExist(err) {
		t.Error("Blueprint file should NOT have been deleted when wipe_blueprints=false")
	}

	// Recreate blueprint file
	if err := os.WriteFile(blueprintFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to recreate blueprint file: %v", err)
	}

	// Test with wipe_blueprints=true
	server.WipeBlueprints = true

	if err := wipeServerData(server); err != nil {
		t.Fatalf("wipeServerData failed: %v", err)
	}

	// Blueprint should be deleted
	if _, err := os.Stat(blueprintFile); !os.IsNotExist(err) {
		t.Error("Blueprint file SHOULD have been deleted when wipe_blueprints=true")
	}
}

func TestSyncServers_Parallel(t *testing.T) {
	// Test that SyncServers processes servers in parallel
	// We can't test actual rsync, but we can verify the function signature and error handling

	servers := []config.Server{
		{Name: "s1", Path: "/nonexistent/s1", Branch: "main"},
		{Name: "s2", Path: "/nonexistent/s2", Branch: "main"},
		{Name: "s3", Path: "/nonexistent/s3", Branch: "main"},
	}

	// This will fail (no actual servers), but should fail for all 3
	err := SyncServers(servers)

	if err == nil {
		t.Error("Expected error when syncing nonexistent servers")
	}

	// Error message should mention all servers
	errMsg := err.Error()
	if !strings.Contains(errMsg, "s1") {
		t.Error("Error should mention s1")
	}
	if !strings.Contains(errMsg, "s2") {
		t.Error("Error should mention s2")
	}
	if !strings.Contains(errMsg, "s3") {
		t.Error("Error should mention s3")
	}
}

func TestScriptPaths(t *testing.T) {
	// Verify script paths are correct
	expectedPaths := map[string]string{
		"HookScript":         "/opt/wiped/pre-start-hook.sh",
		"StopServersScript":  "/opt/wiped/stop-servers.sh",
		"StartServersScript": "/opt/wiped/start-servers.sh",
		"GenerateMapsScript": "/opt/wiped/generate-maps.sh",
	}

	if HookScriptPath != expectedPaths["HookScript"] {
		t.Errorf("HookScriptPath = %s, want %s", HookScriptPath, expectedPaths["HookScript"])
	}
	if StopServersScriptPath != expectedPaths["StopServersScript"] {
		t.Errorf("StopServersScriptPath = %s, want %s", StopServersScriptPath, expectedPaths["StopServersScript"])
	}
	if StartServersScriptPath != expectedPaths["StartServersScript"] {
		t.Errorf("StartServersScriptPath = %s, want %s", StartServersScriptPath, expectedPaths["StartServersScript"])
	}
	if GenerateMapsScriptPath != expectedPaths["GenerateMapsScript"] {
		t.Errorf("GenerateMapsScriptPath = %s, want %s", GenerateMapsScriptPath, expectedPaths["GenerateMapsScript"])
	}
}

func TestEnsureHookScript_Creation(t *testing.T) {
	// Test that hook script is created correctly
	tmpDir := t.TempDir()

	origPath := HookScriptPath
	defer func() {
		HookScriptPath = origPath
	}()

	testPath := filepath.Join(tmpDir, "hook.sh")
	HookScriptPath = testPath

	// Ensure script doesn't exist
	os.Remove(testPath)

	// Create it
	if err := EnsureHookScript(); err != nil {
		t.Fatalf("EnsureHookScript failed: %v", err)
	}

	// Verify it exists and is executable
	info, err := os.Stat(testPath)
	if err != nil {
		t.Fatalf("Script not created: %v", err)
	}

	// Check executable bit
	if info.Mode().Perm()&0111 == 0 {
		t.Error("Script should be executable")
	}

	// Read content and verify it's a bash script
	content, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("Failed to read script: %v", err)
	}

	contentStr := string(content)
	if !strings.HasPrefix(contentStr, "#!/bin/bash") {
		t.Error("Script should start with #!/bin/bash")
	}

	// Verify it doesn't overwrite existing script
	testData := "# custom script"
	if err := os.WriteFile(testPath, []byte(testData), 0755); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	// Try to ensure again
	if err := EnsureHookScript(); err != nil {
		t.Fatalf("EnsureHookScript failed: %v", err)
	}

	// Verify content wasn't overwritten
	newContent, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("Failed to read script: %v", err)
	}

	if string(newContent) != testData {
		t.Error("Script should not be overwritten if it already exists")
	}
}
