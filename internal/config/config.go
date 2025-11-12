package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	ConfigDir  = ".config/wipe"
	ConfigFile = "config.yaml"
)

// Server represents a Rust server to monitor
type Server struct {
	Name           string `mapstructure:"name" yaml:"name"`
	Path           string `mapstructure:"path" yaml:"path"`
	CalendarURL    string `mapstructure:"calendar_url" yaml:"calendar_url"`
	Branch         string `mapstructure:"branch" yaml:"branch"`                   // Rust server branch (default: main)
	WipeBlueprints bool   `mapstructure:"wipe_blueprints" yaml:"wipe_blueprints"` // Whether to delete blueprints on wipe (default: false)
	GenerateMap    bool   `mapstructure:"generate_map" yaml:"generate_map"`       // Whether to generate maps via generate-maps.sh (default: false)
}

// Config holds the application configuration
type Config struct {
	// How far ahead to look for events (in hours)
	LookaheadHours int `mapstructure:"lookahead_hours"`
	// How often to check calendars (in seconds)
	CheckInterval int `mapstructure:"check_interval"`
	// How long to wait after event time before executing (in seconds)
	EventDelay int `mapstructure:"event_delay"`
	// Discord webhook URL for notifications
	DiscordWebhook string `mapstructure:"discord_webhook"`
	// How many hours before a wipe to generate the map (default: 24)
	MapGenerationHours int `mapstructure:"map_generation_hours"`
	// Servers to monitor
	Servers []Server `mapstructure:"servers"`
}

// InitConfig initializes the configuration system
func InitConfig() {
	// Set config file location
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		return
	}

	configPath := filepath.Join(home, ConfigDir)
	viper.AddConfigPath(configPath)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// Set defaults
	viper.SetDefault("lookahead_hours", 24)
	viper.SetDefault("check_interval", 30)
	viper.SetDefault("event_delay", 5)
	viper.SetDefault("discord_webhook", "")
	viper.SetDefault("map_generation_hours", 22)
	viper.SetDefault("servers", []Server{})

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating config directory: %v\n", err)
	}

	// Read config file if it exists
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; create it with defaults
			if err := viper.SafeWriteConfig(); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating config file: %v\n", err)
			}
		}
	}
}

// GetConfig returns the current configuration
func GetConfig() (*Config, error) {
	// Reload config from disk to pick up external changes
	if err := viper.ReadInConfig(); err != nil {
		// If file doesn't exist, that's okay - we'll use defaults
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return &cfg, nil
}

// SaveConfig persists the configuration to disk
func SaveConfig() error {
	return viper.WriteConfig()
}

// AddServer adds a new server to the configuration
func AddServer(name, path, calendarURL, branch string, wipeBlueprints, generateMap bool) error {
	cfg, err := GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Check if server path already exists
	for _, s := range cfg.Servers {
		if s.Path == path {
			return fmt.Errorf("server with path %s already exists", path)
		}
	}

	// Default to main branch if not specified
	if branch == "" {
		branch = "main"
	}

	// Add new server
	cfg.Servers = append(cfg.Servers, Server{
		Name:           name,
		Path:           path,
		CalendarURL:    calendarURL,
		Branch:         branch,
		WipeBlueprints: wipeBlueprints,
		GenerateMap:    generateMap,
	})

	// Update viper
	viper.Set("servers", cfg.Servers)
	return SaveConfig()
}

// RemoveServer removes a server from the configuration by path
func RemoveServer(identifier string) error {
	cfg, err := GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Find and remove server (match by name or path)
	found := false
	newServers := make([]Server, 0)
	for _, s := range cfg.Servers {
		if s.Name != identifier && s.Path != identifier {
			newServers = append(newServers, s)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("server '%s' not found (try name or path)", identifier)
	}

	// Update viper
	viper.Set("servers", newServers)
	return SaveConfig()
}

// UpdateServer updates an existing server's configuration
func UpdateServer(identifier string, updates map[string]interface{}) error {
	cfg, err := GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Find the server (match by name or path)
	found := false
	for i, s := range cfg.Servers {
		if s.Name == identifier || s.Path == identifier {
			found = true

			// Apply updates
			if name, ok := updates["name"].(string); ok && name != "" {
				cfg.Servers[i].Name = name
			}
			if calendarURL, ok := updates["calendar_url"].(string); ok && calendarURL != "" {
				cfg.Servers[i].CalendarURL = calendarURL
			}
			if branch, ok := updates["branch"].(string); ok && branch != "" {
				cfg.Servers[i].Branch = branch
			}
			if wipeBlueprints, ok := updates["wipe_blueprints"].(bool); ok {
				cfg.Servers[i].WipeBlueprints = wipeBlueprints
			}
			if generateMap, ok := updates["generate_map"].(bool); ok {
				cfg.Servers[i].GenerateMap = generateMap
			}

			break
		}
	}

	if !found {
		return fmt.Errorf("server '%s' not found (try name or path)", identifier)
	}

	// Update viper
	viper.Set("servers", cfg.Servers)
	return SaveConfig()
}

// ListServers returns all configured servers
func ListServers() ([]Server, error) {
	cfg, err := GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}
	return cfg.Servers, nil
}

// SetCheckInterval sets the calendar check interval
func SetCheckInterval(seconds int) error {
	if seconds < 10 {
		return fmt.Errorf("check interval must be at least 10 seconds")
	}
	viper.Set("check_interval", seconds)
	return SaveConfig()
}

// SetLookaheadHours sets the event lookahead window
func SetLookaheadHours(hours int) error {
	if hours < 1 {
		return fmt.Errorf("lookahead hours must be at least 1 hour")
	}
	viper.Set("lookahead_hours", hours)
	return SaveConfig()
}

// SetDiscordWebhook sets the Discord webhook URL
func SetDiscordWebhook(url string) error {
	viper.Set("discord_webhook", url)
	return SaveConfig()
}

// SetEventDelay sets the event delay
func SetEventDelay(seconds int) error {
	if seconds < 0 {
		return fmt.Errorf("event delay must be at least 0 seconds")
	}
	viper.Set("event_delay", seconds)
	return SaveConfig()
}

// SetMapGenerationHours sets how many hours before a wipe to generate maps
func SetMapGenerationHours(hours int) error {
	if hours < 1 {
		return fmt.Errorf("map generation hours must be at least 1 hour")
	}
	viper.Set("map_generation_hours", hours)
	return SaveConfig()
}
