package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/maintc/wipe-cli/internal/calendar"
	"github.com/maintc/wipe-cli/internal/carbon"
	"github.com/maintc/wipe-cli/internal/config"
	"github.com/maintc/wipe-cli/internal/discord"
	"github.com/maintc/wipe-cli/internal/executor"
	"github.com/maintc/wipe-cli/internal/scheduler"
	"github.com/maintc/wipe-cli/internal/steamcmd"
)

// Daemon represents the long-running service
type Daemon struct {
	config           *config.Config
	scheduler        *scheduler.Scheduler
	lastUpdate       time.Time
	lastUpdateCheck  time.Time
	mapGenMutex      sync.Mutex
	mapGenInProgress bool
}

// New creates a new Daemon instance
func New() *Daemon {
	return &Daemon{
		lastUpdate:      time.Time{},
		lastUpdateCheck: time.Time{},
	}
}

// Run starts the daemon's main loop
func (d *Daemon) Run(ctx context.Context) error {
	log.Println("Daemon running...")

	// Load initial config
	cfg, err := config.GetConfig()
	if err != nil {
		log.Printf("Error loading initial config: %v", err)
		return err
	}
	d.config = cfg

	// Create scheduler
	sched, err := scheduler.New(cfg.LookaheadHours, cfg.DiscordWebhook, cfg.EventDelay)
	if err != nil {
		log.Printf("Error creating scheduler: %v", err)
		return err
	}
	d.scheduler = sched

	// Ensure scheduler is shut down on exit
	defer func() {
		if d.scheduler != nil {
			log.Println("Shutting down scheduler...")
			if err := d.scheduler.Shutdown(); err != nil {
				log.Printf("Error shutting down scheduler: %v", err)
			}
		}
	}()

	// Create pre-start hook script
	if err := executor.EnsureHookScript(); err != nil {
		log.Printf("Warning: Failed to create hook script: %v", err)
	}

	// Create wipe management scripts (stop-servers.sh, start-servers.sh, generate-maps.sh)
	if err := executor.EnsureWipeScripts(); err != nil {
		log.Printf("Warning: Failed to create wipe scripts: %v", err)
	}

	// Send startup notification
	discord.SendInfo(cfg.DiscordWebhook, "Wipe Service Started",
		fmt.Sprintf("Wipe daemon has started and is monitoring **%d** server(s)", len(cfg.Servers)))

	// Ensure all servers are installed
	if len(cfg.Servers) > 0 {
		log.Printf("Checking server installations...")
		d.ensureServersInstalled()

		log.Printf("Performing initial calendar update...")
		d.updateCalendars()
	} else {
		log.Printf("No servers configured")
	}

	// Ticker for reloading config (every 10 seconds)
	configTicker := time.NewTicker(10 * time.Second)
	defer configTicker.Stop()

	// Ticker for checking updates (every 2 minutes)
	updateCheckTicker := time.NewTicker(2 * time.Minute)
	defer updateCheckTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-updateCheckTicker.C:
			// Check for Rust updates
			d.checkForUpdates()

		case <-configTicker.C:
			// Reload config
			cfg, err := config.GetConfig()
			if err != nil {
				log.Printf("Error loading config: %v", err)
				continue
			}

			// Detect server changes (additions/removals)
			serversChanged := d.detectServerChanges(cfg)
			d.config = cfg

			// If servers changed, immediately update calendars
			if serversChanged {
				log.Printf("Server configuration changed, updating schedules...")
				d.updateCalendars()
			} else if d.shouldUpdateCalendars() {
				// Otherwise, check if it's time for periodic update
				d.updateCalendars()
			}
		}
	}
}

// detectServerChanges checks if servers were added or removed
func (d *Daemon) detectServerChanges(newConfig *config.Config) bool {
	if d.config == nil {
		return false
	}

	// Build maps of server paths for comparison
	oldServers := make(map[string]string)
	newServers := make(map[string]string)

	for _, s := range d.config.Servers {
		oldServers[s.Path] = s.Name
	}

	for _, s := range newConfig.Servers {
		newServers[s.Path] = s.Name
	}

	changed := false

	// Check for removed servers
	for path, name := range oldServers {
		if _, exists := newServers[path]; !exists {
			log.Printf("Server removed: %s (%s)", name, path)
			discord.SendWarning(newConfig.DiscordWebhook, "Server Removed",
				fmt.Sprintf("Server **%s** has been removed from monitoring\n\nPath: `%s`", name, path))
			changed = true
		}
	}

	// Check for added servers
	for path, name := range newServers {
		if _, exists := oldServers[path]; !exists {
			log.Printf("Server added: %s (%s)", name, path)
			discord.SendSuccess(newConfig.DiscordWebhook, "Server Added",
				fmt.Sprintf("Server **%s** has been added to monitoring\n\nPath: `%s`", name, path))
			changed = true
		}
	}

	return changed
}

// shouldUpdateCalendars checks if enough time has passed to update calendars
func (d *Daemon) shouldUpdateCalendars() bool {
	if d.config == nil {
		return false
	}

	if len(d.config.Servers) == 0 {
		return false
	}

	// Update if we've never updated, or if check_interval has passed
	interval := time.Duration(d.config.CheckInterval) * time.Second
	return d.lastUpdate.IsZero() || time.Since(d.lastUpdate) >= interval
}

// updateCalendars fetches and updates calendar events
func (d *Daemon) updateCalendars() {
	log.Printf("Updating calendars for %d server(s)...", len(d.config.Servers))

	if d.scheduler == nil {
		sched, err := scheduler.New(d.config.LookaheadHours, d.config.DiscordWebhook, d.config.EventDelay)
		if err != nil {
			log.Printf("Error creating scheduler: %v", err)
			return
		}
		d.scheduler = sched
	}

	// Update scheduler even if no servers (clears all events)
	if err := d.scheduler.UpdateEvents(d.config.Servers); err != nil {
		log.Printf("Error updating events: %v", err)
		return
	}

	d.lastUpdate = time.Now()

	if len(d.config.Servers) > 0 {
		log.Printf("Next calendar update in %d seconds", d.config.CheckInterval)
	} else {
		log.Printf("No servers configured - monitoring stopped")
	}

	// Check if any maps need to be generated for upcoming wipes
	go d.prepareWipeMaps()
}

// ensureServersInstalled ensures all configured Rust branches and Carbon are installed
func (d *Daemon) ensureServersInstalled() {
	// Collect unique branches
	branches := make(map[string]bool)
	for _, server := range d.config.Servers {
		if server.Branch != "" {
			branches[server.Branch] = true
		}
	}

	// Install each unique Rust branch
	for branch := range branches {
		if err := steamcmd.EnsureRustBranchInstalled(branch, d.config.DiscordWebhook); err != nil {
			log.Printf("Error installing Rust branch '%s': %v", branch, err)
		}
	}

	// Install Carbon for each branch
	for branch := range branches {
		if err := carbon.EnsureCarbonInstalled(branch, d.config.DiscordWebhook); err != nil {
			log.Printf("Error installing Carbon for branch '%s': %v", branch, err)
		}
	}
}

// checkForUpdates checks all configured branches for available updates
func (d *Daemon) checkForUpdates() {
	if d.config == nil {
		return
	}

	// Collect unique branches
	branches := make(map[string]bool)
	for _, server := range d.config.Servers {
		if server.Branch != "" {
			branches[server.Branch] = true
		}
	}

	if len(branches) == 0 {
		return
	}

	log.Printf("Checking for Rust updates for %d branch(es)...", len(branches))

	// Check each branch for Rust updates
	for branch := range branches {
		hasUpdate, buildID, err := steamcmd.CheckForUpdates(branch, d.config.DiscordWebhook)
		if err != nil {
			log.Printf("Error checking Rust updates for branch '%s': %v", branch, err)
			continue
		}

		if hasUpdate {
			log.Printf("Rust update detected for branch '%s', new build ID: %s", branch, buildID)
			// Install the update
			log.Printf("Installing Rust update for branch '%s'...", branch)
			if err := steamcmd.InstallRustBranch(branch, d.config.DiscordWebhook); err != nil {
				log.Printf("Error installing Rust update for branch '%s': %v", branch, err)
			} else {
				log.Printf("Successfully updated Rust branch '%s' to build %s", branch, buildID)
			}
		} else {
			log.Printf("Rust branch '%s' is up to date (build: %s)", branch, buildID)
		}
	}

	// Check each branch for Carbon updates
	log.Printf("Checking for Carbon updates for %d branch(es)...", len(branches))
	for branch := range branches {
		hasUpdate, version, err := carbon.CheckForCarbonUpdates(branch, d.config.DiscordWebhook)
		if err != nil {
			log.Printf("Error checking Carbon updates for branch '%s': %v", branch, err)
			continue
		}

		if hasUpdate {
			log.Printf("Carbon update detected for branch '%s', new version: %s", branch, version)
			// Install the update
			log.Printf("Installing Carbon update for branch '%s'...", branch)
			if err := carbon.InstallCarbon(branch, d.config.DiscordWebhook); err != nil {
				log.Printf("Error installing Carbon update for branch '%s': %v", branch, err)
			} else {
				log.Printf("Successfully updated Carbon for branch '%s' to version %s", branch, version)
			}
		} else if version != "" {
			log.Printf("Carbon for branch '%s' is up to date (version: %s)", branch, version)
		}
	}

	d.lastUpdateCheck = time.Now()
}

// prepareWipeMaps checks for upcoming wipe events and calls generate-maps.sh if needed
func (d *Daemon) prepareWipeMaps() {
	if d.config.MapGenerationHours == 0 || len(d.config.Servers) == 0 {
		return
	}

	// Check if map generation is already in progress
	d.mapGenMutex.Lock()
	if d.mapGenInProgress {
		d.mapGenMutex.Unlock()
		log.Printf("Map generation already in progress, skipping")
		return
	}
	d.mapGenInProgress = true
	d.mapGenMutex.Unlock()

	// Ensure we mark as complete when done
	defer func() {
		d.mapGenMutex.Lock()
		d.mapGenInProgress = false
		d.mapGenMutex.Unlock()
	}()

	// Get all scheduled events from the scheduler
	events := d.scheduler.GetEvents()

	// Build a map of servers with upcoming wipe events within the generation window
	wipeWindow := time.Duration(d.config.MapGenerationHours) * time.Hour
	serversNeedingMaps := make(map[string]bool)

	for _, event := range events {
		// Only process WIPE events
		if event.Event.Type != calendar.EventTypeWipe {
			continue
		}

		// Check if event is within the map generation window
		timeUntilWipe := time.Until(event.Scheduled)
		if timeUntilWipe > 0 && timeUntilWipe <= wipeWindow {
			serversNeedingMaps[event.Server.Name] = true
		}
	}

	// No wipes in the window? Nothing to do
	if len(serversNeedingMaps) == 0 {
		return
	}

	// Collect server paths that need maps and have generate_map enabled
	var serverPathsToGenerate []string
	for _, server := range d.config.Servers {
		if !serversNeedingMaps[server.Name] {
			continue // No wipe scheduled for this server
		}

		if !server.GenerateMap {
			continue // Server doesn't want map generation
		}

		serverPathsToGenerate = append(serverPathsToGenerate, server.Path)
	}

	// Call generate-maps.sh script if there are servers needing map generation
	if len(serverPathsToGenerate) > 0 {
		log.Printf("Calling generate-maps.sh for %d server(s)...", len(serverPathsToGenerate))
		if err := d.callGenerateMapsScript(serverPathsToGenerate); err != nil {
			log.Printf("Error calling generate-maps.sh: %v", err)
			discord.SendError(d.config.DiscordWebhook, "Map Generation Failed",
				fmt.Sprintf("Failed to generate maps: %v", err))
		}
	}
}

// callGenerateMapsScript calls generate-maps.sh with server paths
func (d *Daemon) callGenerateMapsScript(serverPaths []string) error {
	// Check if script exists
	if _, err := os.Stat(executor.GenerateMapsScriptPath); err != nil {
		return fmt.Errorf("generate-maps.sh not found at %s", executor.GenerateMapsScriptPath)
	}

	cmd := exec.Command(executor.GenerateMapsScriptPath, serverPaths...)
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("script failed: %w", err)
	}

	return nil
}
