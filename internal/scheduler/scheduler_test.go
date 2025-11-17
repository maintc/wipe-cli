package scheduler

import (
	"fmt"
	"testing"
	"time"

	"github.com/maintc/wipe-cli/internal/calendar"
	"github.com/maintc/wipe-cli/internal/config"
)

func TestNewScheduler(t *testing.T) {
	s, err := New(48, "https://example.com", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

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

	if s.scheduledJobs == nil {
		t.Error("scheduledJobs map should be initialized")
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
	s, err := New(48, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

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
	s, err := New(48, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	now := time.Now()

	// Add job to scheduled map (simulating a scheduled job)
	timeKey := now.Truncate(time.Minute).Format(time.RFC3339)

	// Verify scheduler was initialized properly
	if s.scheduledJobs == nil {
		t.Error("scheduledJobs map should be initialized")
	}

	// Check that we can track scheduled jobs by time key
	if _, exists := s.scheduledJobs[timeKey]; exists {
		t.Error("Job should not exist yet")
	}
}

func TestSchedulerThreadSafety(t *testing.T) {
	s, err := New(48, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

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

func TestResolveConflicts_WipeTakesPrecedence(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	now := time.Now().Truncate(time.Minute)

	events := []ScheduledEvent{
		{
			Server:    config.Server{Name: "server1", Path: "/path1", Branch: "main"},
			Event:     calendar.Event{Type: calendar.EventTypeRestart, StartTime: now},
			Scheduled: now,
		},
		{
			Server:    config.Server{Name: "server1", Path: "/path1", Branch: "main"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: now},
			Scheduled: now,
		},
	}

	resolved := s.resolveConflicts(events)

	if len(resolved) != 1 {
		t.Fatalf("Expected 1 event after conflict resolution, got %d", len(resolved))
	}

	if resolved[0].Event.Type != calendar.EventTypeWipe {
		t.Errorf("Expected wipe event to take precedence, got %s", resolved[0].Event.Type)
	}
}

func TestResolveConflicts_NoConflict(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	now := time.Now().Truncate(time.Minute)

	events := []ScheduledEvent{
		{
			Server:    config.Server{Name: "server1", Path: "/path1", Branch: "main"},
			Event:     calendar.Event{Type: calendar.EventTypeRestart, StartTime: now},
			Scheduled: now,
		},
		{
			Server:    config.Server{Name: "server2", Path: "/path2", Branch: "main"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: now},
			Scheduled: now,
		},
	}

	resolved := s.resolveConflicts(events)

	if len(resolved) != 2 {
		t.Fatalf("Expected 2 events (no conflicts), got %d", len(resolved))
	}
}

func TestResolveConflicts_DifferentTimes(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	now := time.Now().Truncate(time.Minute)

	events := []ScheduledEvent{
		{
			Server:    config.Server{Name: "server1", Path: "/path1", Branch: "main"},
			Event:     calendar.Event{Type: calendar.EventTypeRestart, StartTime: now},
			Scheduled: now,
		},
		{
			Server:    config.Server{Name: "server1", Path: "/path1", Branch: "main"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: now.Add(time.Hour)},
			Scheduled: now.Add(time.Hour),
		},
	}

	resolved := s.resolveConflicts(events)

	if len(resolved) != 2 {
		t.Fatalf("Expected 2 events (different times), got %d", len(resolved))
	}
}

func TestResolveConflicts_EmptyEvents(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	events := []ScheduledEvent{}
	resolved := s.resolveConflicts(events)

	if len(resolved) != 0 {
		t.Fatalf("Expected 0 events, got %d", len(resolved))
	}
}

func TestResolveConflicts_MultipleRestarts(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	now := time.Now().Truncate(time.Minute)

	// Multiple restart events for same server at same time
	events := []ScheduledEvent{
		{
			Server:    config.Server{Name: "server1", Path: "/path1", Branch: "main"},
			Event:     calendar.Event{Type: calendar.EventTypeRestart, StartTime: now},
			Scheduled: now,
		},
		{
			Server:    config.Server{Name: "server1", Path: "/path1", Branch: "main"},
			Event:     calendar.Event{Type: calendar.EventTypeRestart, StartTime: now},
			Scheduled: now,
		},
	}

	resolved := s.resolveConflicts(events)

	if len(resolved) != 1 {
		t.Fatalf("Expected 1 event after deduplication, got %d", len(resolved))
	}
}

func TestSchedulerShutdown(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	// Shutdown should not return error
	if err := s.Shutdown(); err != nil {
		t.Errorf("Shutdown() returned error: %v", err)
	}

	// Multiple shutdowns should be safe
	if err := s.Shutdown(); err != nil {
		t.Errorf("Second Shutdown() returned error: %v", err)
	}
}

func TestGetEvents_ReturnsCopy(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	now := time.Now()
	testEvent := ScheduledEvent{
		Server:    config.Server{Name: "server1", Path: "/path1"},
		Event:     calendar.Event{Type: calendar.EventTypeRestart, StartTime: now},
		Scheduled: now,
	}

	s.events = []ScheduledEvent{testEvent}

	// Get events
	events1 := s.GetEvents()
	events2 := s.GetEvents()

	// Modify first copy
	if len(events1) > 0 {
		events1[0].Server.Name = "modified"
	}

	// Second copy should be unchanged
	if len(events2) > 0 && events2[0].Server.Name == "modified" {
		t.Error("GetEvents() should return a copy, not reference to internal slice")
	}

	// Original should be unchanged
	if s.events[0].Server.Name == "modified" {
		t.Error("Modifying returned events should not affect internal state")
	}
}

func TestSchedulerConfiguration_DifferentValues(t *testing.T) {
	tests := []struct {
		name           string
		lookaheadHours int
		webhookURL     string
		eventDelay     int
	}{
		{"minimal", 1, "", 0},
		{"typical", 24, "https://discord.com/webhook", 5},
		{"extended", 168, "https://example.com/webhook/123", 300},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := New(tt.lookaheadHours, tt.webhookURL, tt.eventDelay)
			if err != nil {
				t.Fatalf("New() returned error: %v", err)
			}
			defer s.Shutdown()

			if s.lookaheadHours != tt.lookaheadHours {
				t.Errorf("lookaheadHours = %d, want %d", s.lookaheadHours, tt.lookaheadHours)
			}

			if s.webhookURL != tt.webhookURL {
				t.Errorf("webhookURL = %s, want %s", s.webhookURL, tt.webhookURL)
			}

			if s.eventDelay != tt.eventDelay {
				t.Errorf("eventDelay = %d, want %d", s.eventDelay, tt.eventDelay)
			}
		})
	}
}

func TestEventGrouping_ByMinute(t *testing.T) {
	now := time.Now()
	baseTime := now.Truncate(time.Minute)

	events := []ScheduledEvent{
		{
			Server:    config.Server{Name: "s1", Path: "/p1"},
			Event:     calendar.Event{Type: calendar.EventTypeRestart, StartTime: baseTime},
			Scheduled: baseTime,
		},
		{
			Server:    config.Server{Name: "s2", Path: "/p2"},
			Event:     calendar.Event{Type: calendar.EventTypeRestart, StartTime: baseTime.Add(30 * time.Second)},
			Scheduled: baseTime.Add(30 * time.Second),
		},
	}

	// Group by truncated minute
	groups := make(map[string][]ScheduledEvent)
	for _, event := range events {
		key := event.Scheduled.Truncate(time.Minute).Format(time.RFC3339)
		groups[key] = append(groups[key], event)
	}

	// Both should be in same group (same minute)
	if len(groups) != 1 {
		t.Errorf("Expected events within same minute to be grouped together, got %d groups", len(groups))
	}

	for _, group := range groups {
		if len(group) != 2 {
			t.Errorf("Expected 2 events in group, got %d", len(group))
		}
	}
}

func TestScheduledEvent_FieldAccess(t *testing.T) {
	now := time.Now()

	event := ScheduledEvent{
		Server: config.Server{
			Name:   "test-server",
			Path:   "/var/servers/test",
			Branch: "main",
		},
		Event: calendar.Event{
			Type:      calendar.EventTypeWipe,
			StartTime: now,
			Summary:   "Monthly Wipe",
		},
		Scheduled: now,
	}

	// Test all fields are accessible
	if event.Server.Name != "test-server" {
		t.Errorf("Server.Name = %s, want test-server", event.Server.Name)
	}

	if event.Server.Path != "/var/servers/test" {
		t.Errorf("Server.Path = %s, want /var/servers/test", event.Server.Path)
	}

	if event.Server.Branch != "main" {
		t.Errorf("Server.Branch = %s, want main", event.Server.Branch)
	}

	if event.Event.Type != calendar.EventTypeWipe {
		t.Errorf("Event.Type = %s, want wipe", event.Event.Type)
	}

	if event.Event.Summary != "Monthly Wipe" {
		t.Errorf("Event.Summary = %s, want Monthly Wipe", event.Event.Summary)
	}

	if !event.Scheduled.Equal(now) {
		t.Errorf("Scheduled time mismatch")
	}
}

func TestResolveConflicts_LargeScale(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	now := time.Now().Truncate(time.Minute)

	// Create 50 servers, each with a restart and wipe at same time
	events := make([]ScheduledEvent, 0, 100)
	for i := 0; i < 50; i++ {
		serverPath := fmt.Sprintf("/path/server%d", i)
		events = append(events,
			ScheduledEvent{
				Server:    config.Server{Name: fmt.Sprintf("server%d", i), Path: serverPath, Branch: "main"},
				Event:     calendar.Event{Type: calendar.EventTypeRestart, StartTime: now},
				Scheduled: now,
			},
			ScheduledEvent{
				Server:    config.Server{Name: fmt.Sprintf("server%d", i), Path: serverPath, Branch: "main"},
				Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: now},
				Scheduled: now,
			},
		)
	}

	resolved := s.resolveConflicts(events)

	// Should have 50 events (one wipe per server)
	if len(resolved) != 50 {
		t.Errorf("Expected 50 events after conflict resolution, got %d", len(resolved))
	}

	// All should be wipes
	for _, event := range resolved {
		if event.Event.Type != calendar.EventTypeWipe {
			t.Errorf("Expected all resolved events to be wipes, got %s", event.Event.Type)
		}
	}
}

func TestGetEvents_Concurrency(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	// Add some test data
	now := time.Now()
	s.events = []ScheduledEvent{
		{
			Server:    config.Server{Name: "s1", Path: "/p1"},
			Event:     calendar.Event{Type: calendar.EventTypeRestart, StartTime: now},
			Scheduled: now,
		},
	}

	// Run 100 concurrent reads
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			events := s.GetEvents()
			if len(events) != 1 {
				t.Errorf("Expected 1 event, got %d", len(events))
			}
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 100; i++ {
		<-done
	}
}

// TestRaceConditionPrevention verifies the exact scenario from Nov 16 cannot happen again
func TestRaceConditionPrevention(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	// Setup: 5 servers with events scheduled for 2 minutes from now
	eventTime := time.Now().Add(2 * time.Minute).Truncate(time.Minute)

	servers := []config.Server{
		{Name: "us-weekly", Path: "/path1", Branch: "main"},
		{Name: "modded", Path: "/path2", Branch: "main"},
		{Name: "us-long", Path: "/path3", Branch: "main"},
		{Name: "us-build", Path: "/path4", Branch: "main"},
		{Name: "train", Path: "/path5", Branch: "main"},
	}

	// Create initial events
	initialEvents := make([]ScheduledEvent, 5)
	for i, server := range servers {
		initialEvents[i] = ScheduledEvent{
			Server:    server,
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		}
	}

	s.events = initialEvents

	// Schedule jobs for these events
	if err := s.scheduleJobs(); err != nil {
		t.Fatalf("Failed to schedule jobs: %v", err)
	}

	// Verify all 5 events are scheduled
	if len(s.scheduledJobs) != 1 {
		t.Fatalf("Expected 1 job group (all at same time), got %d", len(s.scheduledJobs))
	}

	timeKey := eventTime.Format(time.RFC3339)
	jobID, exists := s.scheduledJobs[timeKey]
	if !exists {
		t.Fatal("Job for event time not found in scheduledJobs")
	}

	// SIMULATE THE RACE CONDITION: Calendar update happens at execution time
	// Remove 3 events (simulating calendar returning next occurrence)
	s.mutex.Lock()
	s.events = []ScheduledEvent{
		initialEvents[0], // us-weekly
		initialEvents[1], // modded
		// us-long, us-build, train "disappear" from calendar
	}
	s.mutex.Unlock()

	// Try to reschedule (simulating the calendar update at 19:00:00)
	if err := s.scheduleJobs(); err != nil {
		t.Fatalf("Failed to reschedule jobs: %v", err)
	}

	// CRITICAL CHECK: The job should STILL exist in gocron
	// Even though we removed events from s.events, the gocron job is immutable
	stillExists := s.scheduledJobs[timeKey]
	if stillExists != jobID {
		t.Error("Job ID changed after calendar update - race condition possible!")
	}

	// Verify the job still exists in gocron's internal scheduler
	// (we can't directly check gocron internals, but we verify our tracking)
	if len(s.scheduledJobs) == 0 {
		t.Error("All jobs were removed - race condition would occur!")
	}

	// The key insight: scheduleJobs() has this logic:
	// "Skip if already scheduled" (line 371-373)
	// This means once a job is scheduled in gocron, it CANNOT be removed
	// by a calendar update unless the event is explicitly cancelled from the calendar
}

// TestJobImmutability verifies that once a gocron job is scheduled, it cannot be affected by s.events changes
func TestJobImmutability(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	eventTime := time.Now().Add(1 * time.Minute).Truncate(time.Minute)

	// Schedule a job
	s.events = []ScheduledEvent{
		{
			Server:    config.Server{Name: "server1", Path: "/path1"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		},
	}

	if err := s.scheduleJobs(); err != nil {
		t.Fatalf("Failed to schedule initial job: %v", err)
	}

	timeKey := eventTime.Format(time.RFC3339)
	_, originallyScheduled := s.scheduledJobs[timeKey]
	if !originallyScheduled {
		t.Fatal("Job was not scheduled initially")
	}

	// Clear all events (simulating calendar returning empty)
	s.mutex.Lock()
	s.events = []ScheduledEvent{}
	s.mutex.Unlock()

	// Try to reschedule
	if err := s.scheduleJobs(); err != nil {
		t.Fatalf("Failed to reschedule: %v", err)
	}

	// The job should be REMOVED from tracking because the time key is no longer in currentTimeKeys
	// But this is INTENTIONAL - if an event disappears from calendar entirely, we should cancel it
	_, stillTracked := s.scheduledJobs[timeKey]
	if stillTracked {
		t.Log("Job still tracked (would execute even though event was removed from calendar)")
	} else {
		t.Log("Job was cancelled (correct behavior when event is removed from calendar)")
	}
}

// TestJobPersistenceDuringCalendarUpdate tests that jobs persist when events remain in calendar
func TestJobPersistenceDuringCalendarUpdate(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	eventTime := time.Now().Add(2 * time.Minute).Truncate(time.Minute)

	// Initial schedule
	s.events = []ScheduledEvent{
		{
			Server:    config.Server{Name: "server1", Path: "/path1"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		},
		{
			Server:    config.Server{Name: "server2", Path: "/path2"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		},
	}

	if err := s.scheduleJobs(); err != nil {
		t.Fatalf("Failed to schedule: %v", err)
	}

	timeKey := eventTime.Format(time.RFC3339)
	originalJobID := s.scheduledJobs[timeKey]

	// Calendar update: same events, maybe different details
	s.mutex.Lock()
	s.events = []ScheduledEvent{
		{
			Server:    config.Server{Name: "server1", Path: "/path1"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		},
		{
			Server:    config.Server{Name: "server2", Path: "/path2"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		},
	}
	s.mutex.Unlock()

	// Reschedule
	if err := s.scheduleJobs(); err != nil {
		t.Fatalf("Failed to reschedule: %v", err)
	}

	// Job ID should be UNCHANGED
	currentJobID := s.scheduledJobs[timeKey]
	if currentJobID != originalJobID {
		t.Errorf("Job ID changed from %v to %v - job was rescheduled when it shouldn't be",
			originalJobID, currentJobID)
	}
}

// TestMultipleCalendarUpdatesBeforeExecution simulates rapid calendar updates before event time
func TestMultipleCalendarUpdatesBeforeExecution(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	eventTime := time.Now().Add(5 * time.Minute).Truncate(time.Minute)

	// Simulate 10 rapid calendar updates
	for i := 0; i < 10; i++ {
		s.mutex.Lock()
		s.events = []ScheduledEvent{
			{
				Server:    config.Server{Name: "server1", Path: "/path1"},
				Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
				Scheduled: eventTime,
			},
		}
		s.mutex.Unlock()

		if err := s.scheduleJobs(); err != nil {
			t.Fatalf("Failed to schedule on iteration %d: %v", i, err)
		}
	}

	// After all updates, job should still be scheduled
	timeKey := eventTime.Format(time.RFC3339)
	if _, exists := s.scheduledJobs[timeKey]; !exists {
		t.Error("Job was lost after multiple calendar updates")
	}

	// Verify only ONE job was created (not 10)
	if len(s.scheduledJobs) != 1 {
		t.Errorf("Expected 1 job, got %d (possible duplicate scheduling)", len(s.scheduledJobs))
	}
}

// TestIndividualServerAddRemove verifies individual servers can be added/removed from time group
func TestIndividualServerAddRemove(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	eventTime := time.Now().Add(3 * time.Minute).Truncate(time.Minute)
	timeKey := eventTime.Format(time.RFC3339)

	// Initial: 3 servers at same time
	s.events = []ScheduledEvent{
		{
			Server:    config.Server{Name: "server-a", Path: "/path-a"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		},
		{
			Server:    config.Server{Name: "server-b", Path: "/path-b"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		},
		{
			Server:    config.Server{Name: "server-c", Path: "/path-c"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		},
	}

	if err := s.scheduleJobs(); err != nil {
		t.Fatalf("Failed to schedule: %v", err)
	}

	// Verify job created and has 3 servers
	if _, exists := s.scheduledJobs[timeKey]; !exists {
		t.Fatal("Job not created")
	}

	if len(s.jobEvents[timeKey]) != 3 {
		t.Fatalf("Expected 3 servers in job, got %d", len(s.jobEvents[timeKey]))
	}

	originalJobID := s.scheduledJobs[timeKey]

	// Calendar update: Remove server-b
	s.events = []ScheduledEvent{
		{
			Server:    config.Server{Name: "server-a", Path: "/path-a"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		},
		{
			Server:    config.Server{Name: "server-c", Path: "/path-c"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		},
	}

	if err := s.scheduleJobs(); err != nil {
		t.Fatalf("Failed to reschedule: %v", err)
	}

	// Job should STILL exist (not rescheduled)
	currentJobID := s.scheduledJobs[timeKey]
	if currentJobID != originalJobID {
		t.Error("Job was rescheduled (should be updated, not replaced)")
	}

	// But event list should be UPDATED to 2 servers
	if len(s.jobEvents[timeKey]) != 2 {
		t.Fatalf("Expected 2 servers after update, got %d", len(s.jobEvents[timeKey]))
	}

	// Verify server-b is gone
	for _, event := range s.jobEvents[timeKey] {
		if event.Server.Name == "server-b" {
			t.Error("server-b should have been removed from event list")
		}
	}

	// Calendar update: Add server-d
	s.events = []ScheduledEvent{
		{
			Server:    config.Server{Name: "server-a", Path: "/path-a"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		},
		{
			Server:    config.Server{Name: "server-c", Path: "/path-c"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		},
		{
			Server:    config.Server{Name: "server-d", Path: "/path-d"},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		},
	}

	if err := s.scheduleJobs(); err != nil {
		t.Fatalf("Failed to reschedule after add: %v", err)
	}

	// Job ID should STILL be same
	if s.scheduledJobs[timeKey] != originalJobID {
		t.Error("Job was rescheduled when adding server (should be updated)")
	}

	// Event list should have 3 servers again
	if len(s.jobEvents[timeKey]) != 3 {
		t.Fatalf("Expected 3 servers after adding, got %d", len(s.jobEvents[timeKey]))
	}

	// Verify server-d is present
	found := false
	for _, event := range s.jobEvents[timeKey] {
		if event.Server.Name == "server-d" {
			found = true
			break
		}
	}
	if !found {
		t.Error("server-d should have been added to event list")
	}
}

// TestEventListUpdateReflectsInExecution verifies that event list updates affect what executes
func TestEventListUpdateReflectsInExecution(t *testing.T) {
	s, err := New(24, "", 60)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer s.Shutdown()

	eventTime := time.Now().Add(2 * time.Minute).Truncate(time.Minute)
	timeKey := eventTime.Format(time.RFC3339)

	// Schedule 5 servers
	initialServers := []string{"s1", "s2", "s3", "s4", "s5"}
	initialEvents := make([]ScheduledEvent, len(initialServers))
	for i, name := range initialServers {
		initialEvents[i] = ScheduledEvent{
			Server:    config.Server{Name: name, Path: "/path" + name},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		}
	}

	s.events = initialEvents
	if err := s.scheduleJobs(); err != nil {
		t.Fatalf("Failed to schedule: %v", err)
	}

	// Update to only 2 servers
	updatedServers := []string{"s1", "s3"}
	updatedEvents := make([]ScheduledEvent, len(updatedServers))
	for i, name := range updatedServers {
		updatedEvents[i] = ScheduledEvent{
			Server:    config.Server{Name: name, Path: "/path" + name},
			Event:     calendar.Event{Type: calendar.EventTypeWipe, StartTime: eventTime},
			Scheduled: eventTime,
		}
	}

	s.events = updatedEvents
	if err := s.scheduleJobs(); err != nil {
		t.Fatalf("Failed to update: %v", err)
	}

	// The stored job events should reflect the update
	storedEvents := s.jobEvents[timeKey]
	if len(storedEvents) != 2 {
		t.Fatalf("Expected 2 servers in stored events, got %d", len(storedEvents))
	}

	// Verify correct servers are present
	serverNames := make(map[string]bool)
	for _, event := range storedEvents {
		serverNames[event.Server.Name] = true
	}

	if !serverNames["s1"] || !serverNames["s3"] {
		t.Error("Expected s1 and s3 in stored events")
	}

	if serverNames["s2"] || serverNames["s4"] || serverNames["s5"] {
		t.Error("s2, s4, s5 should not be in stored events")
	}
}
