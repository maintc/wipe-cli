package calendar

import (
	"testing"
	"time"
)

func TestEventTypeConstants(t *testing.T) {
	if EventTypeRestart != "restart" {
		t.Errorf("EventTypeRestart = %s, want restart", EventTypeRestart)
	}

	if EventTypeWipe != "wipe" {
		t.Errorf("EventTypeWipe = %s, want wipe", EventTypeWipe)
	}
}

func TestEventStruct(t *testing.T) {
	now := time.Now()
	event := Event{
		Type:      EventTypeWipe,
		StartTime: now,
		EndTime:   now.Add(time.Hour),
		Summary:   "Force Wipe",
	}

	if event.Type != EventTypeWipe {
		t.Errorf("Event.Type = %s, want wipe", event.Type)
	}

	if event.Summary != "Force Wipe" {
		t.Errorf("Event.Summary = %s, want Force Wipe", event.Summary)
	}

	if event.StartTime.After(event.EndTime) {
		t.Error("Event.StartTime should be before Event.EndTime")
	}
}

func TestScheduledEventStruct(t *testing.T) {
	now := time.Now()
	event := ScheduledEvent{
		Type:      EventTypeRestart,
		StartTime: now,
	}

	if event.Type != EventTypeRestart {
		t.Errorf("ScheduledEvent.Type = %s, want restart", event.Type)
	}

	if event.StartTime.IsZero() {
		t.Error("ScheduledEvent.StartTime should not be zero")
	}
}

func TestEventSorting(t *testing.T) {
	now := time.Now()
	events := []Event{
		{Type: EventTypeRestart, StartTime: now.Add(3 * time.Hour)},
		{Type: EventTypeWipe, StartTime: now.Add(1 * time.Hour)},
		{Type: EventTypeRestart, StartTime: now.Add(2 * time.Hour)},
	}

	// Manual sort for testing
	for i := 0; i < len(events)-1; i++ {
		for j := 0; j < len(events)-i-1; j++ {
			if events[j].StartTime.After(events[j+1].StartTime) {
				events[j], events[j+1] = events[j+1], events[j]
			}
		}
	}

	// Check if sorted correctly
	if !events[0].StartTime.Before(events[1].StartTime) {
		t.Error("Events should be sorted by StartTime ascending")
	}

	if !events[1].StartTime.Before(events[2].StartTime) {
		t.Error("Events should be sorted by StartTime ascending")
	}
}

func TestEventTypeString(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		want      string
	}{
		{
			name:      "restart event",
			eventType: EventTypeRestart,
			want:      "restart",
		},
		{
			name:      "wipe event",
			eventType: EventTypeWipe,
			want:      "wipe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(tt.eventType)
			if got != tt.want {
				t.Errorf("EventType string = %s, want %s", got, tt.want)
			}
		})
	}
}
