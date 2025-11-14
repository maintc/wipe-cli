package discord

import (
	"testing"
)

func TestGetHostname(t *testing.T) {
	hostname := GetHostname()
	if hostname == "" {
		t.Error("GetHostname() returned empty string")
	}
	if hostname == "unknown" {
		t.Log("GetHostname() returned 'unknown' - this is acceptable in test environments")
	}
}

func TestColorConstants(t *testing.T) {
	tests := []struct {
		name  string
		color int
		want  int
	}{
		{"ColorSuccess", ColorSuccess, 0x00ff00},
		{"ColorInfo", ColorInfo, 0x0099ff},
		{"ColorWarning", ColorWarning, 0xff9900},
		{"ColorError", ColorError, 0xff0000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.color != tt.want {
				t.Errorf("%s = %x, want %x", tt.name, tt.color, tt.want)
			}
		})
	}
}

func TestSendNotificationWithEmptyWebhook(t *testing.T) {
	// Test that sending with empty webhook doesn't error
	err := SendNotification("", "Test", "Test message", ColorInfo)
	if err != nil {
		t.Errorf("SendNotification() with empty webhook should not error, got: %v", err)
	}
}

func TestEmbedStructure(t *testing.T) {
	footer := &EmbedFooter{
		Text: "Test Footer",
	}

	embed := Embed{
		Title:       "Test Title",
		Description: "Test Description",
		Color:       ColorSuccess,
		Footer:      footer,
	}

	if embed.Title != "Test Title" {
		t.Errorf("Embed.Title = %s, want Test Title", embed.Title)
	}
	if embed.Description != "Test Description" {
		t.Errorf("Embed.Description = %s, want Test Description", embed.Description)
	}
	if embed.Color != ColorSuccess {
		t.Errorf("Embed.Color = %x, want %x", embed.Color, ColorSuccess)
	}
	if embed.Footer == nil || embed.Footer.Text != "Test Footer" {
		t.Errorf("Embed.Footer.Text = %v, want Test Footer", embed.Footer)
	}
}

func TestWebhookPayloadStructure(t *testing.T) {
	embed := Embed{
		Title:       "Test",
		Description: "Test",
		Color:       ColorInfo,
	}

	payload := WebhookPayload{
		Content: "Test content",
		Embeds:  []Embed{embed},
	}

	if len(payload.Embeds) != 1 {
		t.Errorf("len(WebhookPayload.Embeds) = %d, want 1", len(payload.Embeds))
	}
	if payload.Embeds[0].Title != "Test" {
		t.Errorf("WebhookPayload.Embeds[0].Title = %s, want Test", payload.Embeds[0].Title)
	}
	if payload.Content != "Test content" {
		t.Errorf("WebhookPayload.Content = %s, want Test content", payload.Content)
	}
}

func TestEmbedFieldStructure(t *testing.T) {
	field := EmbedField{
		Name:   "Field Name",
		Value:  "Field Value",
		Inline: true,
	}

	if field.Name != "Field Name" {
		t.Errorf("EmbedField.Name = %s, want Field Name", field.Name)
	}
	if field.Value != "Field Value" {
		t.Errorf("EmbedField.Value = %s, want Field Value", field.Value)
	}
	if !field.Inline {
		t.Error("EmbedField.Inline should be true")
	}
}

func TestEmbedImageStructure(t *testing.T) {
	image := &EmbedImage{
		URL: "https://example.com/image.png",
	}

	if image.URL != "https://example.com/image.png" {
		t.Errorf("EmbedImage.URL = %s, want https://example.com/image.png", image.URL)
	}
}
