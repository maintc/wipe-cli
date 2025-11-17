package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/maintc/wipe-cli/internal/config"
	"github.com/maintc/wipe-cli/internal/executor"
	"github.com/maintc/wipe-cli/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "wipe",
	Short:   "Wipe CLI - Configure the wipe monitoring service",
	Long:    `A CLI tool to configure Rust server calendars for the wipe daemon to monitor.`,
	Version: version.GetVersion(),
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a Rust server to monitor",
	Long:  `Add a Rust server with its calendar URL to the monitoring configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		path, _ := cmd.Flags().GetString("path")
		calendarURL, _ := cmd.Flags().GetString("calendar")
		branch, _ := cmd.Flags().GetString("branch")
		wipeBlueprints, _ := cmd.Flags().GetBool("wipe-blueprints")
		generateMap, _ := cmd.Flags().GetBool("generate-map")

		// Validate required flags
		if path == "" {
			fmt.Fprintf(os.Stderr, "Error: --path is required\n")
			os.Exit(1)
		}
		if calendarURL == "" {
			fmt.Fprintf(os.Stderr, "Error: --calendar is required\n")
			os.Exit(1)
		}

		// Derive name from path basename
		name := filepath.Base(path)

		// Default to main branch
		if branch == "" {
			branch = "main"
		}

		if err := config.AddServer(name, path, calendarURL, branch, wipeBlueprints, generateMap); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding server: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Added server: %s\n", name)
		fmt.Printf("  Path: %s\n", path)
		fmt.Printf("  Branch: %s\n", branch)
		fmt.Printf("  Calendar: %s\n", calendarURL)
		fmt.Printf("  Wipe blueprints: %v\n", wipeBlueprints)
		fmt.Printf("  Generate map: %v\n", generateMap)
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured servers",
	Long:  `Display all Rust servers currently being monitored.`,
	Run: func(cmd *cobra.Command, args []string) {
		servers, err := config.ListServers()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing servers: %v\n", err)
			os.Exit(1)
		}

		if len(servers) == 0 {
			fmt.Println("No servers configured.")
			fmt.Println("\nAdd a server with: wipe add --path /path/to/server --calendar https://...")
			return
		}

		fmt.Printf("Configured servers (%d):\n\n", len(servers))
		for i, s := range servers {
			fmt.Printf("%d. %s\n", i+1, s.Name)
			fmt.Printf("   Path: %s\n", s.Path)
			fmt.Printf("   Branch: %s\n", s.Branch)
			fmt.Printf("   Wipe blueprints: %v\n", s.WipeBlueprints)
			fmt.Printf("   Generate map: %v\n", s.GenerateMap)
			fmt.Printf("   Calendar: %s\n", s.CalendarURL)
			if i < len(servers)-1 {
				fmt.Println()
			}
		}
	},
}

var removeCmd = &cobra.Command{
	Use:   "remove [name or path]",
	Short: "Remove a server from monitoring",
	Long:  `Remove a Rust server from the monitoring configuration by its name or path.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		identifier := args[0]

		if err := config.RemoveServer(identifier); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing server: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Removed server: %s\n", identifier)
	},
}

var updateCmd = &cobra.Command{
	Use:   "update [name or path]",
	Short: "Update a server's configuration",
	Long:  `Update configuration settings for an existing server by name or path. Only provide flags for settings you want to change.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		identifier := args[0]

		updates := make(map[string]interface{})

		// Check which flags were provided and add them to updates map
		if cmd.Flags().Changed("calendar") {
			calendarURL, _ := cmd.Flags().GetString("calendar")
			updates["calendar_url"] = calendarURL
		}
		if cmd.Flags().Changed("branch") {
			branch, _ := cmd.Flags().GetString("branch")
			updates["branch"] = branch
		}
		if cmd.Flags().Changed("wipe-blueprints") {
			wipeBlueprints, _ := cmd.Flags().GetBool("wipe-blueprints")
			updates["wipe_blueprints"] = wipeBlueprints
		}
		if cmd.Flags().Changed("generate-map") {
			generateMap, _ := cmd.Flags().GetBool("generate-map")
			updates["generate_map"] = generateMap
		}

		if len(updates) == 0 {
			fmt.Fprintf(os.Stderr, "Error: No settings to update. Provide at least one flag to change.\n")
			os.Exit(1)
		}

		if err := config.UpdateServer(identifier, updates); err != nil {
			fmt.Fprintf(os.Stderr, "Error updating server: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Updated server: %s\n", identifier)
		fmt.Println("  Changes:")
		for key := range updates {
			switch key {
			case "calendar_url":
				fmt.Println("    - calendar URL updated")
			case "branch":
				fmt.Printf("    - branch: %s\n", updates[key])
			case "wipe_blueprints":
				fmt.Printf("    - wipe blueprints: %v\n", updates[key])
			case "generate_map":
				fmt.Printf("    - generate map: %v\n", updates[key])
			}
		}
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or modify configuration settings",
	Long:  `View or modify global configuration settings like check interval and lookahead hours.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.GetConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Current configuration:")
		fmt.Printf("  Check interval: %d seconds (refresh calendars every %ds)\n", cfg.CheckInterval, cfg.CheckInterval)
		fmt.Printf("  Lookahead hours: %d hours (schedule events up to %dh ahead)\n", cfg.LookaheadHours, cfg.LookaheadHours)
		fmt.Printf("  Event delay: %d seconds (wait %ds after event time before executing)\n", cfg.EventDelay, cfg.EventDelay)
		fmt.Printf("  Map generation hours: %d hours (generate maps %dh before wipe)\n", cfg.MapGenerationHours, cfg.MapGenerationHours)
		if cfg.DiscordWebhook != "" {
			fmt.Printf("  Discord webhook: configured\n")
		} else {
			fmt.Printf("  Discord webhook: not configured\n")
		}
		fmt.Printf("  Discord mention users: %d configured\n", len(cfg.DiscordMentionUsers))
		if len(cfg.DiscordMentionUsers) > 0 {
			for _, userID := range cfg.DiscordMentionUsers {
				fmt.Printf("    - %s\n", userID)
			}
		}
		fmt.Printf("  Discord mention roles: %d configured\n", len(cfg.DiscordMentionRoles))
		if len(cfg.DiscordMentionRoles) > 0 {
			for _, roleID := range cfg.DiscordMentionRoles {
				fmt.Printf("    - %s\n", roleID)
			}
		}
		fmt.Printf("  Servers configured: %d\n", len(cfg.Servers))
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set a configuration value",
	Long:  `Set configuration values like check-interval or lookahead-hours.`,
	Run: func(cmd *cobra.Command, args []string) {
		checkInterval, _ := cmd.Flags().GetInt("check-interval")
		lookaheadHours, _ := cmd.Flags().GetInt("lookahead-hours")
		eventDelay, _ := cmd.Flags().GetInt("event-delay")
		mapGenerationHours, _ := cmd.Flags().GetInt("map-generation-hours")
		discordWebhook, _ := cmd.Flags().GetString("discord-webhook")

		changed := false

		if cmd.Flags().Changed("check-interval") {
			if err := config.SetCheckInterval(checkInterval); err != nil {
				fmt.Fprintf(os.Stderr, "Error setting check interval: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✓ Check interval set to %d seconds\n", checkInterval)
			changed = true
		}

		if cmd.Flags().Changed("lookahead-hours") {
			if err := config.SetLookaheadHours(lookaheadHours); err != nil {
				fmt.Fprintf(os.Stderr, "Error setting lookahead hours: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✓ Lookahead hours set to %d hours\n", lookaheadHours)
			changed = true
		}

		if cmd.Flags().Changed("event-delay") {
			if err := config.SetEventDelay(eventDelay); err != nil {
				fmt.Fprintf(os.Stderr, "Error setting event delay: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✓ Event delay set to %d seconds\n", eventDelay)
			changed = true
		}

		if cmd.Flags().Changed("discord-webhook") {
			if err := config.SetDiscordWebhook(discordWebhook); err != nil {
				fmt.Fprintf(os.Stderr, "Error setting discord webhook: %v\n", err)
				os.Exit(1)
			}
			if discordWebhook == "" {
				fmt.Println("✓ Discord webhook disabled")
			} else {
				fmt.Println("✓ Discord webhook configured")
			}
			changed = true
		}

		if cmd.Flags().Changed("map-generation-hours") {
			if err := config.SetMapGenerationHours(mapGenerationHours); err != nil {
				fmt.Fprintf(os.Stderr, "Error setting map generation hours: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✓ Map generation hours set to %d hours\n", mapGenerationHours)
			changed = true
		}

		if !changed {
			fmt.Println("No settings changed. Use --check-interval, --lookahead-hours, --event-delay, --discord-webhook, or --map-generation-hours")
		}
	},
}

var callScriptCmd = &cobra.Command{
	Use:   "call-script [server-names...] --script <script-name>",
	Short: "Call a management script with server paths",
	Long: `Call one of the management scripts with the paths of specified servers.

Available scripts:
  - stop-servers
  - start-servers
  - generate-maps

Example:
  wipe call-script us-weekly eu-monthly --script stop-servers`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		scriptName, _ := cmd.Flags().GetString("script")

		if scriptName == "" {
			fmt.Fprintf(os.Stderr, "Error: --script flag is required\n")
			os.Exit(1)
		}

		// Trim .sh extension if provided
		scriptName = filepath.Base(scriptName)
		if filepath.Ext(scriptName) == ".sh" {
			scriptName = scriptName[:len(scriptName)-3]
		}

		// Validate script name
		validScripts := map[string]string{
			"stop-servers":  "/opt/wiped/stop-servers.sh",
			"start-servers": "/opt/wiped/start-servers.sh",
			"generate-maps": "/opt/wiped/generate-maps.sh",
		}

		scriptPath, ok := validScripts[scriptName]
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: Invalid script name '%s'\n", scriptName)
			fmt.Fprintf(os.Stderr, "Valid scripts: stop-servers, start-servers, generate-maps\n")
			os.Exit(1)
		}

		// Check if script exists
		if _, err := os.Stat(scriptPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Script not found at %s\n", scriptPath)
			fmt.Fprintf(os.Stderr, "Restart the wiped service to generate scripts: sudo systemctl restart wiped@$USER.service\n")
			os.Exit(1)
		}

		// Get config to look up server paths
		cfg, err := config.GetConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}

		// Map server names to paths
		serverPaths := []string{}
		for _, serverName := range args {
			found := false
			for _, server := range cfg.Servers {
				if server.Name == serverName {
					serverPaths = append(serverPaths, server.Path)
					found = true
					break
				}
			}
			if !found {
				fmt.Fprintf(os.Stderr, "Error: Server '%s' not found in configuration\n", serverName)
				os.Exit(1)
			}
		}

		if len(serverPaths) == 0 {
			fmt.Fprintf(os.Stderr, "Error: No valid servers found\n")
			os.Exit(1)
		}

		// Call the script
		fmt.Printf("📞 Calling %s with %d server(s)...\n", scriptName, len(serverPaths))
		fmt.Printf("   Script: %s\n", scriptPath)
		fmt.Printf("   Servers: %v\n\n", args)

		// Use exec to run the script with streaming output
		fmt.Println("--- Script Output ---")
		cmdExec := exec.Command(scriptPath, serverPaths...)
		cmdExec.Stdout = os.Stdout
		cmdExec.Stderr = os.Stderr

		if err := cmdExec.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "\n❌ Script failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("--- End Output ---")
		fmt.Println("\n✓ Script completed successfully")
	},
}

var syncCmd = &cobra.Command{
	Use:   "sync [server-names...]",
	Short: "Update Rust and Carbon on servers",
	Long: `Updates Rust and Carbon installations from /opt/rust and /opt/carbon on the specified servers.

This command:
  - Does NOT stop or start servers
  - Does NOT delete any files (no wipe)
  - Does NOT run the pre-start hook
  - Only updates Rust and Carbon files

Example:
  wipe sync us-weekly eu-monthly
  wipe sync us-weekly --force  # Skip confirmation prompt`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		force, _ := cmd.Flags().GetBool("force")
		// Initialize logger for executor output
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags)

		// Get config
		cfg, err := config.GetConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}

		// Find servers by name
		var serversToSync []config.Server
		for _, serverName := range args {
			found := false
			for _, server := range cfg.Servers {
				if server.Name == serverName {
					serversToSync = append(serversToSync, server)
					found = true
					break
				}
			}
			if !found {
				fmt.Fprintf(os.Stderr, "Error: server '%s' not found\n", serverName)
				fmt.Fprintf(os.Stderr, "Available servers: ")
				for i, s := range cfg.Servers {
					if i > 0 {
						fmt.Fprintf(os.Stderr, ", ")
					}
					fmt.Fprintf(os.Stderr, "%s", s.Name)
				}
				fmt.Fprintf(os.Stderr, "\n")
				os.Exit(1)
			}
		}

		// Show warning and get confirmation (unless --force is used)
		if !force {
			fmt.Printf("⚠️  WARNING: You are about to update Rust and Carbon on %d server(s):\n\n", len(serversToSync))
			for _, server := range serversToSync {
				fmt.Printf("  • %s (%s, branch: %s)\n", server.Name, server.Path, server.Branch)
			}
			fmt.Println("\n⚠️  IMPORTANT: These servers should be STOPPED before updating!")
			fmt.Println("   Updating files while servers are running may cause issues.")
			fmt.Print("\nDo you want to continue? (yes/no): ")

			var response string
			fmt.Scanln(&response)

			if response != "yes" && response != "y" {
				fmt.Println("❌ Update cancelled")
				os.Exit(0)
			}
		}

		// Update servers
		fmt.Printf("\n🔄 Updating %d server(s)...\n\n", len(serversToSync))
		if err := executor.SyncServers(serversToSync); err != nil {
			fmt.Fprintf(os.Stderr, "\n❌ Update failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n✓ All servers updated successfully")
	},
}

var resetScriptsCmd = &cobra.Command{
	Use:   "reset-scripts",
	Short: "Reset management scripts to defaults",
	Long: `Deletes and regenerates the management scripts in /opt/wiped:
  - stop-servers.sh
  - start-servers.sh
  - pre-start-hook.sh
  - generate-maps.sh

WARNING: This will overwrite any customizations you've made to these scripts.`,
	Run: func(cmd *cobra.Command, args []string) {
		force, _ := cmd.Flags().GetBool("force")

		if !force {
			fmt.Println("⚠️  WARNING: This will delete and regenerate the following scripts:")
			fmt.Println("   - /opt/wiped/stop-servers.sh")
			fmt.Println("   - /opt/wiped/start-servers.sh")
			fmt.Println("   - /opt/wiped/pre-start-hook.sh")
			fmt.Println("   - /opt/wiped/generate-maps.sh")
			fmt.Println()
			fmt.Println("Any customizations you've made will be LOST!")
			fmt.Println()
			fmt.Print("Are you sure you want to continue? (yes/no): ")

			var response string
			fmt.Scanln(&response)

			if response != "yes" {
				fmt.Println("❌ Operation cancelled")
				os.Exit(0)
			}
		}

		fmt.Println("🔄 Resetting scripts...")

		// Import executor package
		executor := struct {
			HookScriptPath         string
			StopServersScriptPath  string
			StartServersScriptPath string
			GenerateMapsScriptPath string
		}{
			HookScriptPath:         "/opt/wiped/pre-start-hook.sh",
			StopServersScriptPath:  "/opt/wiped/stop-servers.sh",
			StartServersScriptPath: "/opt/wiped/start-servers.sh",
			GenerateMapsScriptPath: "/opt/wiped/generate-maps.sh",
		}

		scriptsRemoved := 0
		scriptsToRemove := []string{
			executor.HookScriptPath,
			executor.StopServersScriptPath,
			executor.StartServersScriptPath,
			executor.GenerateMapsScriptPath,
		}

		for _, script := range scriptsToRemove {
			if _, err := os.Stat(script); err == nil {
				if err := os.Remove(script); err != nil {
					fmt.Fprintf(os.Stderr, "Error removing %s: %v\n", script, err)
					os.Exit(1)
				}
				scriptsRemoved++
				fmt.Printf("  ✓ Removed %s\n", filepath.Base(script))
			}
		}

		if scriptsRemoved > 0 {
			fmt.Printf("\n✓ Removed %d script(s)\n", scriptsRemoved)
			fmt.Println("\nTo regenerate the scripts, restart the wiped service:")
			fmt.Println("  sudo systemctl restart wiped@$USER.service")
		} else {
			fmt.Println("ℹ️  No scripts found to remove")
		}
	},
}

var mentionCmd = &cobra.Command{
	Use:   "mention",
	Short: "Manage Discord mention lists",
	Long:  `Add or remove Discord user and role IDs to mention in notifications.`,
}

var mentionAddUserCmd = &cobra.Command{
	Use:   "add-user [user-id]",
	Short: "Add a Discord user ID to mention in notifications",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		userID := args[0]

		if err := config.AddDiscordMentionUser(userID); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding user: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Added Discord user ID: %s\n", userID)
		fmt.Printf("  This user will be mentioned in Discord notifications as <@%s>\n", userID)
	},
}

var mentionRemoveUserCmd = &cobra.Command{
	Use:   "remove-user [user-id]",
	Short: "Remove a Discord user ID from mentions",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		userID := args[0]

		if err := config.RemoveDiscordMentionUser(userID); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing user: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Removed Discord user ID: %s\n", userID)
	},
}

var mentionAddRoleCmd = &cobra.Command{
	Use:   "add-role [role-id]",
	Short: "Add a Discord role ID to mention in notifications",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		roleID := args[0]

		if err := config.AddDiscordMentionRole(roleID); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding role: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Added Discord role ID: %s\n", roleID)
		fmt.Printf("  This role will be mentioned in Discord notifications as <@&%s>\n", roleID)
	},
}

var mentionRemoveRoleCmd = &cobra.Command{
	Use:   "remove-role [role-id]",
	Short: "Remove a Discord role ID from mentions",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		roleID := args[0]

		if err := config.RemoveDiscordMentionRole(roleID); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing role: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ Removed Discord role ID: %s\n", roleID)
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Initialize config
	config.InitConfig()

	// Add flags for add command
	addCmd.Flags().StringP("path", "p", "", "Full path to Rust server (required)")
	addCmd.Flags().StringP("calendar", "c", "", "Google Calendar .ics URL (required)")
	addCmd.Flags().StringP("branch", "b", "main", "Rust server branch (main, staging, etc.)")
	addCmd.Flags().Bool("wipe-blueprints", false, "Delete blueprints on wipe events")
	addCmd.Flags().Bool("generate-map", false, "Generate custom maps via generate-maps.sh")

	// Add flags for config set command
	configSetCmd.Flags().Int("check-interval", 0, "How often to refresh calendars (in seconds)")
	configSetCmd.Flags().Int("lookahead-hours", 0, "How far ahead to schedule events (in hours)")
	configSetCmd.Flags().Int("event-delay", 0, "How long to wait after event time before executing (in seconds)")
	configSetCmd.Flags().Int("map-generation-hours", 0, "How many hours before a wipe to generate maps")
	configSetCmd.Flags().String("discord-webhook", "", "Discord webhook URL for notifications (empty to disable)")

	// Add flags for update command
	updateCmd.Flags().StringP("calendar", "c", "", "Google Calendar .ics URL")
	updateCmd.Flags().StringP("branch", "b", "", "Rust server branch (main, staging, etc.)")
	updateCmd.Flags().Bool("wipe-blueprints", false, "Delete blueprints on wipe events")
	updateCmd.Flags().Bool("generate-map", false, "Generate custom maps via generate-maps.sh")

	// Add flags for sync command
	syncCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	// Add flags for reset-scripts command
	resetScriptsCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	// Add flags for call-script command
	callScriptCmd.Flags().StringP("script", "s", "", "Script name to call (required): stop-servers, start-servers, generate-maps")
	callScriptCmd.MarkFlagRequired("script")

	// Add subcommands
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(removeCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(resetScriptsCmd)
	rootCmd.AddCommand(callScriptCmd)
	rootCmd.AddCommand(mentionCmd)
	configCmd.AddCommand(configSetCmd)
	mentionCmd.AddCommand(mentionAddUserCmd)
	mentionCmd.AddCommand(mentionRemoveUserCmd)
	mentionCmd.AddCommand(mentionAddRoleCmd)
	mentionCmd.AddCommand(mentionRemoveRoleCmd)
}
