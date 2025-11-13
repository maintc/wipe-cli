package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/maintc/wipe-cli/internal/config"
)

// Color constants for embed colors
const (
	ColorSuccess = 0x00ff00 // Green
	ColorInfo    = 0x0099ff // Blue
	ColorWarning = 0xff9900 // Orange
	ColorError   = 0xff0000 // Red
)

// EmbedField represents a field in a Discord embed
type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// Embed represents a Discord embed
type Embed struct {
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Timestamp   string       `json:"timestamp,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Image       *EmbedImage  `json:"image,omitempty"`
	Thumbnail   *EmbedImage  `json:"thumbnail,omitempty"`
	Footer      *EmbedFooter `json:"footer,omitempty"`
}

// EmbedImage represents an image in a Discord embed
type EmbedImage struct {
	URL string `json:"url"`
}

// EmbedFooter represents a footer in a Discord embed
type EmbedFooter struct {
	Text    string `json:"text"`
	IconURL string `json:"icon_url,omitempty"`
}

// WebhookPayload represents the Discord webhook payload
type WebhookPayload struct {
	Content string  `json:"content,omitempty"`
	Embeds  []Embed `json:"embeds,omitempty"`
}

// GetHostname returns the system hostname
func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// SendNotification sends a Discord notification with an embed
func SendNotification(webhookURL, title, description string, color int) error {
	if webhookURL == "" {
		// Webhook not configured, skip silently
		return nil
	}

	hostname := GetHostname()

	// Load config to get mention IDs
	cfg, err := config.GetConfig()
	if err == nil {
		// Build mention string inline if mentions are configured
		userIDs := cfg.DiscordMentionUsers
		roleIDs := cfg.DiscordMentionRoles

		if len(userIDs) > 0 || len(roleIDs) > 0 {
			mentions := []string{}
			for _, roleID := range roleIDs {
				mentions = append(mentions, fmt.Sprintf("<@&%s>", roleID))
			}
			for _, userID := range userIDs {
				mentions = append(mentions, fmt.Sprintf("<@%s>", userID))
			}

			if len(mentions) > 0 {
				mentionStr := "cc " + mentions[0]
				for i := 1; i < len(mentions); i++ {
					mentionStr += " " + mentions[i]
				}
				description = mentionStr + "\n\n" + description
			}
		}
	}

	embed := Embed{
		Title:       title,
		Description: description,
		Color:       color,
		Timestamp:   time.Now().Format(time.RFC3339),
		Fields: []EmbedField{
			{
				Name:   "Hostname",
				Value:  hostname,
				Inline: true,
			},
		},
	}

	payload := WebhookPayload{
		Embeds: []Embed{embed},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// SendSuccess sends a success notification (green)
func SendSuccess(webhookURL, title, description string) {
	if err := SendNotification(webhookURL, title, description, ColorSuccess); err != nil {
		log.Printf("Failed to send Discord success notification: %v", err)
	}
}

// SendInfo sends an info notification (blue)
func SendInfo(webhookURL, title, description string) {
	if err := SendNotification(webhookURL, title, description, ColorInfo); err != nil {
		log.Printf("Failed to send Discord info notification: %v", err)
	}
}

// SendWarning sends a warning notification (orange)
func SendWarning(webhookURL, title, description string) {
	if err := SendNotification(webhookURL, title, description, ColorWarning); err != nil {
		log.Printf("Failed to send Discord warning notification: %v", err)
	}
}

// SendError sends an error notification (red)
func SendError(webhookURL, title, description string) {
	if err := SendNotification(webhookURL, title, description, ColorError); err != nil {
		log.Printf("Failed to send Discord error notification: %v", err)
	}
}
