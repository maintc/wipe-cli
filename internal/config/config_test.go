package config

import (
	"testing"
)

func TestServerStruct(t *testing.T) {
	tests := []struct {
		name   string
		server Server
	}{
		{
			name: "valid server with all fields",
			server: Server{
				Name:           "test-server",
				Path:           "/path/to/server",
				CalendarURL:    "https://example.com/calendar.ics",
				Branch:         "release",
				WipeBlueprints: true,
				GenerateMap:    true,
			},
		},
		{
			name: "server with minimal fields",
			server: Server{
				Name:   "minimal-server",
				Path:   "/path/to/minimal",
				Branch: "main",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.server.Name == "" {
				t.Error("Server name should not be empty")
			}
			if tt.server.Path == "" {
				t.Error("Server path should not be empty")
			}
			if tt.server.Branch == "" {
				t.Error("Server branch should not be empty")
			}
		})
	}
}

func TestConfigStruct(t *testing.T) {
	cfg := Config{
		LookaheadHours:      48,
		CheckInterval:       30,
		EventDelay:          60,
		DiscordWebhook:      "https://discord.com/api/webhooks/test",
		DiscordMentionUsers: []string{"123456789"},
		DiscordMentionRoles: []string{"987654321"},
		MapGenerationHours:  22,
		Servers: []Server{
			{
				Name:   "test",
				Path:   "/test",
				Branch: "main",
			},
		},
	}

	if cfg.LookaheadHours != 48 {
		t.Errorf("LookaheadHours = %d, want 48", cfg.LookaheadHours)
	}

	if cfg.CheckInterval != 30 {
		t.Errorf("CheckInterval = %d, want 30", cfg.CheckInterval)
	}

	if cfg.EventDelay != 60 {
		t.Errorf("EventDelay = %d, want 60", cfg.EventDelay)
	}

	if cfg.MapGenerationHours != 22 {
		t.Errorf("MapGenerationHours = %d, want 22", cfg.MapGenerationHours)
	}

	if len(cfg.Servers) != 1 {
		t.Errorf("len(Servers) = %d, want 1", len(cfg.Servers))
	}

	if len(cfg.DiscordMentionUsers) != 1 {
		t.Errorf("len(DiscordMentionUsers) = %d, want 1", len(cfg.DiscordMentionUsers))
	}

	if len(cfg.DiscordMentionRoles) != 1 {
		t.Errorf("len(DiscordMentionRoles) = %d, want 1", len(cfg.DiscordMentionRoles))
	}
}

func TestConfigConstants(t *testing.T) {
	if ConfigDir != ".config/wiped" {
		t.Errorf("ConfigDir = %s, want .config/wiped", ConfigDir)
	}

	if ConfigFile != "config.yaml" {
		t.Errorf("ConfigFile = %s, want config.yaml", ConfigFile)
	}
}

func TestServerWithDefaultBranch(t *testing.T) {
	server := Server{
		Name:   "test-server",
		Path:   "/path/to/server",
		Branch: "", // Empty branch should be handled by AddServer function
	}

	if server.Name == "" {
		t.Error("Server name should not be empty")
	}

	if server.Path == "" {
		t.Error("Server path should not be empty")
	}

	// Branch can be empty, AddServer will set default
}

func TestDiscordMentionArrays(t *testing.T) {
	cfg := Config{
		DiscordMentionUsers: []string{"user1", "user2", "user3"},
		DiscordMentionRoles: []string{"role1", "role2"},
	}

	if len(cfg.DiscordMentionUsers) != 3 {
		t.Errorf("len(DiscordMentionUsers) = %d, want 3", len(cfg.DiscordMentionUsers))
	}

	if len(cfg.DiscordMentionRoles) != 2 {
		t.Errorf("len(DiscordMentionRoles) = %d, want 2", len(cfg.DiscordMentionRoles))
	}

	// Test that we can iterate over them
	for _, user := range cfg.DiscordMentionUsers {
		if user == "" {
			t.Error("Discord mention user should not be empty")
		}
	}

	for _, role := range cfg.DiscordMentionRoles {
		if role == "" {
			t.Error("Discord mention role should not be empty")
		}
	}
}
