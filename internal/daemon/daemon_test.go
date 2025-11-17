package daemon

import (
	"testing"
	"time"

	"github.com/maintc/wipe-cli/internal/config"
)

func TestNew(t *testing.T) {
	d := New()

	if d == nil {
		t.Fatal("New() returned nil")
	}

	if d.lastUpdate.IsZero() == false {
		t.Error("lastUpdate should be zero time on creation")
	}

	if d.lastUpdateCheck.IsZero() == false {
		t.Error("lastUpdateCheck should be zero time on creation")
	}

	if d.mapGenInProgress {
		t.Error("mapGenInProgress should be false on creation")
	}
}

func TestDetectServerChanges_NoChanges(t *testing.T) {
	d := New()

	cfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
			{Name: "server2", Path: "/path2", Branch: "main"},
		},
	}

	d.config = cfg

	// Same config
	newCfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
			{Name: "server2", Path: "/path2", Branch: "main"},
		},
	}

	changed := d.detectServerChanges(newCfg)

	if changed {
		t.Error("Expected no changes when servers are identical")
	}
}

func TestDetectServerChanges_ServerAdded(t *testing.T) {
	d := New()

	cfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
		},
	}

	d.config = cfg

	// Add server
	newCfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
			{Name: "server2", Path: "/path2", Branch: "main"},
		},
	}

	changed := d.detectServerChanges(newCfg)

	if !changed {
		t.Error("Expected changes when server is added")
	}
}

func TestDetectServerChanges_ServerRemoved(t *testing.T) {
	d := New()

	cfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
			{Name: "server2", Path: "/path2", Branch: "main"},
		},
	}

	d.config = cfg

	// Remove server
	newCfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
		},
	}

	changed := d.detectServerChanges(newCfg)

	if !changed {
		t.Error("Expected changes when server is removed")
	}
}

func TestDetectServerChanges_NilConfig(t *testing.T) {
	d := New()

	newCfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
		},
	}

	// d.config is nil
	changed := d.detectServerChanges(newCfg)

	if changed {
		t.Error("Expected no changes when old config is nil")
	}
}

func TestDetectServerChanges_EmptyToPopulated(t *testing.T) {
	d := New()

	cfg := &config.Config{
		Servers: []config.Server{},
	}

	d.config = cfg

	// Add servers
	newCfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
		},
	}

	changed := d.detectServerChanges(newCfg)

	if !changed {
		t.Error("Expected changes when going from empty to populated")
	}
}

func TestDetectServerChanges_PopulatedToEmpty(t *testing.T) {
	d := New()

	cfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
		},
	}

	d.config = cfg

	// Remove all servers
	newCfg := &config.Config{
		Servers: []config.Server{},
	}

	changed := d.detectServerChanges(newCfg)

	if !changed {
		t.Error("Expected changes when going from populated to empty")
	}
}

func TestDetectServerChanges_MultipleChanges(t *testing.T) {
	d := New()

	cfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
			{Name: "server2", Path: "/path2", Branch: "main"},
		},
	}

	d.config = cfg

	// Remove server1, add server3
	newCfg := &config.Config{
		Servers: []config.Server{
			{Name: "server2", Path: "/path2", Branch: "main"},
			{Name: "server3", Path: "/path3", Branch: "main"},
		},
	}

	changed := d.detectServerChanges(newCfg)

	if !changed {
		t.Error("Expected changes when multiple servers change")
	}
}

func TestDetectServerChanges_NameChangeOnly(t *testing.T) {
	d := New()

	cfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
		},
	}

	d.config = cfg

	// Change only the name (path stays same)
	newCfg := &config.Config{
		Servers: []config.Server{
			{Name: "new-name", Path: "/path1", Branch: "main"},
		},
	}

	changed := d.detectServerChanges(newCfg)

	// Path is the key, so name change alone shouldn't trigger change detection
	if changed {
		t.Error("Expected no changes when only server name changes (path is key)")
	}
}

func TestShouldUpdateCalendars_NilConfig(t *testing.T) {
	d := New()

	should := d.shouldUpdateCalendars()

	if should {
		t.Error("Should not update calendars when config is nil")
	}
}

func TestShouldUpdateCalendars_NoServers(t *testing.T) {
	d := New()

	d.config = &config.Config{
		Servers: []config.Server{},
	}

	should := d.shouldUpdateCalendars()

	if should {
		t.Error("Should not update calendars when no servers configured")
	}
}

func TestShouldUpdateCalendars_NeverUpdated(t *testing.T) {
	d := New()

	d.config = &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
		},
		CheckInterval: 30,
	}

	// lastUpdate is zero time
	should := d.shouldUpdateCalendars()

	if !should {
		t.Error("Should update calendars when never updated before")
	}
}

func TestShouldUpdateCalendars_IntervalPassed(t *testing.T) {
	d := New()

	d.config = &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
		},
		CheckInterval: 1, // 1 second
	}

	// Set last update to 2 seconds ago
	d.lastUpdate = time.Now().Add(-2 * time.Second)

	should := d.shouldUpdateCalendars()

	if !should {
		t.Error("Should update calendars when interval has passed")
	}
}

func TestShouldUpdateCalendars_IntervalNotPassed(t *testing.T) {
	d := New()

	d.config = &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
		},
		CheckInterval: 60, // 60 seconds
	}

	// Set last update to just now
	d.lastUpdate = time.Now()

	should := d.shouldUpdateCalendars()

	if should {
		t.Error("Should not update calendars when interval has not passed")
	}
}

func TestDetectServerChanges_SamePathDifferentBranch(t *testing.T) {
	d := New()

	cfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
		},
	}

	d.config = cfg

	// Same path but different branch
	newCfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "staging"},
		},
	}

	changed := d.detectServerChanges(newCfg)

	// Path is the key, so branch change shouldn't trigger change detection
	if changed {
		t.Error("Expected no changes when only branch changes (path is key)")
	}
}

func TestDetectServerChanges_OrderChange(t *testing.T) {
	d := New()

	cfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
			{Name: "server2", Path: "/path2", Branch: "main"},
		},
	}

	d.config = cfg

	// Same servers, different order
	newCfg := &config.Config{
		Servers: []config.Server{
			{Name: "server2", Path: "/path2", Branch: "main"},
			{Name: "server1", Path: "/path1", Branch: "main"},
		},
	}

	changed := d.detectServerChanges(newCfg)

	if changed {
		t.Error("Expected no changes when only order changes")
	}
}

func TestDetectServerChanges_LargeScale(t *testing.T) {
	d := New()

	// Create 100 servers
	servers := make([]config.Server, 100)
	for i := 0; i < 100; i++ {
		servers[i] = config.Server{
			Name:   "server",
			Path:   "/path" + string(rune('0'+i%10)),
			Branch: "main",
		}
	}

	cfg := &config.Config{Servers: servers}
	d.config = cfg

	// Same config
	newCfg := &config.Config{Servers: servers}

	changed := d.detectServerChanges(newCfg)

	if changed {
		t.Error("Expected no changes with large identical server set")
	}
}

func TestMapGenInProgress_InitialState(t *testing.T) {
	d := New()

	if d.mapGenInProgress {
		t.Error("mapGenInProgress should be false initially")
	}
}

func TestMapGenMutex_CanLock(t *testing.T) {
	d := New()

	// Should be able to lock and unlock
	d.mapGenMutex.Lock()
	state := d.mapGenInProgress // Read state inside critical section
	d.mapGenMutex.Unlock()

	// Verify initial state
	if state {
		t.Error("mapGenInProgress should be false initially")
	}
}

func TestShouldUpdateCalendars_EdgeCase_ZeroInterval(t *testing.T) {
	d := New()

	d.config = &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
		},
		CheckInterval: 0,
	}

	d.lastUpdate = time.Now()

	should := d.shouldUpdateCalendars()

	// With 0 interval, should always update (interval has technically passed)
	if !should {
		t.Error("Should update calendars when interval is 0")
	}
}

func TestDetectServerChanges_DuplicatePaths(t *testing.T) {
	d := New()

	cfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
		},
	}

	d.config = cfg

	// Add duplicate path (different name)
	newCfg := &config.Config{
		Servers: []config.Server{
			{Name: "server1", Path: "/path1", Branch: "main"},
			{Name: "server1-copy", Path: "/path1", Branch: "main"},
		},
	}

	changed := d.detectServerChanges(newCfg)

	// Duplicate path shouldn't be detected as change since path is the key
	if changed {
		t.Error("Expected no changes when duplicate path is added")
	}
}

func TestDaemon_StateConsistency(t *testing.T) {
	d := New()

	// Verify initial state
	if d.config != nil {
		t.Error("config should be nil initially")
	}

	if d.scheduler != nil {
		t.Error("scheduler should be nil initially")
	}

	if d.lastUpdate.IsZero() == false {
		t.Error("lastUpdate should be zero initially")
	}

	if d.lastUpdateCheck.IsZero() == false {
		t.Error("lastUpdateCheck should be zero initially")
	}

	if d.mapGenInProgress {
		t.Error("mapGenInProgress should be false initially")
	}
}

func TestShouldUpdateCalendars_BoundaryConditions(t *testing.T) {
	tests := []struct {
		name            string
		checkInterval   int
		timeSinceUpdate time.Duration
		expectUpdate    bool
	}{
		{"exactly at interval", 30, 30 * time.Second, true},
		{"just before interval", 30, 29 * time.Second, false},
		{"just after interval", 30, 31 * time.Second, true},
		{"far past interval", 30, 300 * time.Second, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := New()
			d.config = &config.Config{
				Servers: []config.Server{
					{Name: "server1", Path: "/path1", Branch: "main"},
				},
				CheckInterval: tt.checkInterval,
			}

			d.lastUpdate = time.Now().Add(-tt.timeSinceUpdate)

			should := d.shouldUpdateCalendars()

			if should != tt.expectUpdate {
				t.Errorf("shouldUpdateCalendars() = %v, want %v", should, tt.expectUpdate)
			}
		})
	}
}
