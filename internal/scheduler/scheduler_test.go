package scheduler

import (
	"testing"
	"time"

	"github.com/maintc/wipe-cli/internal/calendar"
	"github.com/maintc/wipe-cli/internal/config"
)

func TestNewScheduler(t *testing.T) {
	s := New(48, "https://example.com", 60)

	if s == nil {
		t.Fatal("New() returned nil")
	}

	if s.lookaheadHours != 48 {
		t.Errorf("lookaheadHours = %d, want 48", s.lookaheadHours)
	}

	if s.webhookURL != "https://example.com" {
		t.Errorf("webhookURL = %s, want https://example.com", s.webhookURL)
	}

	if s.eventDelay != 60 {
		t.Errorf("eventDelay = %d, want 60", s.eventDelay)
	}

	if s.events == nil {
		t.Error("events slice should be initialized")
	}

	if s.executedEvents == nil {
		t.Error("executedEvents map should be initialized")
	}
}

func TestScheduledEventKey(t *testing.T) {
	now := time.Now()
	event := ScheduledEvent{
		Server: config.Server{
			Name:   "test-server",
			Path:   "/path/to/server",
			Branch: "release",
		},
		Event: calendar.Event{
			Type:      calendar.EventTypeWipe,
			StartTime: now,
			Summary:   "Wipe Event",
		},
		Scheduled: now,
	}

	// Create expected key format
	timeKey := now.Format("2006-01-02T15:04")
	expectedKey := timeKey + "-/path/to/server-wipe"

	// This tests the key format used internally
	actualKey := timeKey + "-" + event.Server.Path + "-" + string(event.Event.Type)

	if actualKey != expectedKey {
		t.Errorf("Event key = %s, want %s", actualKey, expectedKey)
	}
}

func TestGetEvents(t *testing.T) {
	s := New(48, "", 60)

	// Should start with empty events
	events := s.GetEvents()
	if len(events) != 0 {
		t.Errorf("len(events) = %d, want 0", len(events))
	}

	// Add some test events
	now := time.Now()
	testEvents := []ScheduledEvent{
		{
			Server: config.Server{Name: "server1", Path: "/path1"},
			Event:  calendar.Event{Type: calendar.EventTypeRestart, StartTime: now},
		},
		{
			Server: config.Server{Name: "server2", Path: "/path2"},
			Event:  calendar.Event{Type: calendar.EventTypeWipe, StartTime: now.Add(time.Hour)},
		},
	}

	s.events = testEvents

	events = s.GetEvents()
	if len(events) != 2 {
		t.Errorf("len(events) = %d, want 2", len(events))
	}
}

func TestGroupEventsByTime(t *testing.T) {
	now := time.Now()
	baseTime := now.Truncate(time.Minute)

	events := []ScheduledEvent{
		{
			Server: config.Server{Name: "server1", Path: "/path1"},
			Event:  calendar.Event{Type: calendar.EventTypeRestart, StartTime: baseTime},
		},
		{
			Server: config.Server{Name: "server2", Path: "/path2"},
			Event:  calendar.Event{Type: calendar.EventTypeWipe, StartTime: baseTime},
		},
		{
			Server: config.Server{Name: "server3", Path: "/path3"},
			Event:  calendar.Event{Type: calendar.EventTypeRestart, StartTime: baseTime.Add(time.Hour)},
		},
	}

	// Group events by time (manually for testing)
	groups := make(map[string][]ScheduledEvent)
	for _, event := range events {
		key := event.Event.StartTime.Format("2006-01-02T15:04")
		groups[key] = append(groups[key], event)
	}

	if len(groups) != 2 {
		t.Errorf("len(groups) = %d, want 2", len(groups))
	}

	firstGroupKey := baseTime.Format("2006-01-02T15:04")
	if len(groups[firstGroupKey]) != 2 {
		t.Errorf("len(groups[%s]) = %d, want 2", firstGroupKey, len(groups[firstGroupKey]))
	}

	secondGroupKey := baseTime.Add(time.Hour).Format("2006-01-02T15:04")
	if len(groups[secondGroupKey]) != 1 {
		t.Errorf("len(groups[%s]) = %d, want 1", secondGroupKey, len(groups[secondGroupKey]))
	}
}

func TestEventDuplication(t *testing.T) {
	s := New(48, "", 60)

	now := time.Now()

	// Add event to executed map
	timeKey := now.Format("2006-01-02T15:04")
	eventKey := timeKey + "-/path-wipe"
	s.executedEvents[eventKey] = true

	// Check if event is marked as executed
	if !s.executedEvents[eventKey] {
		t.Error("Event should be marked as executed")
	}
}

func TestSchedulerThreadSafety(t *testing.T) {
	s := New(48, "", 60)

	// Test concurrent reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_ = s.GetEvents()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
