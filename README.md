# 🧹 Wipe CLI

**Automated Rust game server management powered by Google Calendar schedules.**

## 📖 What It Does

Rust game servers often have scheduled restart and wipe events tracked in Google Calendar. This tool:

- 📅 **Monitors multiple Google Calendar iCal feeds** (one per server)
- 🔍 **Detects upcoming events** within a configurable time window (default: 24 hours)
- 🔄 **Auto-installs and updates** Rust server files (`/opt/rust/{branch}`) and Carbon mod (`/opt/carbon/{branch}`)
- ⚡ **Executes restart/wipe operations** at scheduled times via customizable shell scripts
- 📊 **Aggregates events** across multiple servers (e.g., restart 3 servers simultaneously)
- 📣 **Discord webhook notifications** for events, updates, and errors
- 🗺️ **Custom map generation** workflows via `generate-maps.sh`

**⚠️ Event Priority**: If a server has both a restart and wipe event at the same time, it's treated as a wipe.

## 🏗️ Architecture

This project consists of two main components:

1. **`wipe`** 🖥️ - CLI tool for managing server configurations
2. **`wiped`** 🤖 - Long-running daemon that monitors calendars and schedules tasks

The two components communicate via a shared configuration file stored at `~/.config/wipe/config.yaml`.

### ⚙️ Event Execution Flow

When a restart or wipe event occurs:

1. 🛑 **Stop servers** → Calls `/opt/wipe-cli/stop-servers.sh` with server paths
2. 📦 **Update Rust & Carbon** → Installs/updates Rust and Carbon
3. 🧹 **Wipe data** (wipes only) → Deletes map, save, and blueprint files (configurable)
4. 🔧 **Run hook** → Calls `/opt/wipe-cli/pre-start-hook.sh` once with all server paths
5. ▶️ **Start servers** → Calls `/opt/wipe-cli/start-servers.sh` with server paths

All scripts receive server paths as arguments, allowing you to integrate with your existing infrastructure.

## Project Structure

```
wipe-cli/
├── cmd/
│   ├── wipe/          # CLI tool entry point
│   └── wiped/         # Daemon entry point
├── internal/
│   ├── calendar/      # iCal parsing and event detection
│   ├── carbon/        # Carbon mod installation and updates
│   ├── config/        # Shared configuration management
│   ├── daemon/        # Daemon logic
│   ├── discord/       # Discord webhook notifications
│   ├── executor/      # Event execution and script management
│   ├── scheduler/     # Event scheduling and grouping
│   └── steamcmd/      # Rust server installation via SteamCMD
├── systemd/
│   └── wipe.service   # systemd service file
├── go.mod
├── Makefile
└── README.md
```

## 🔨 Building

```bash
make build
```

This creates binaries in the `build/` directory.

## 🚀 Installation

```bash
make install
```

This will:
- ✅ Install `wipe` and `wiped` binaries to `/usr/local/bin/`
- ✅ Install the systemd service file
- ✅ Reload systemd

After installation, enable and start the service:

```bash
sudo systemctl enable wipe@$USER.service
sudo systemctl start wipe@$USER.service
```

### 📜 Management Scripts

The daemon automatically creates default management scripts in `/opt/wipe-cli/` on first run:

- 🛑 `stop-servers.sh` - Called to stop servers before restart/wipe
- ▶️ `start-servers.sh` - Called to start servers after restart/wipe
- 🔧 `pre-start-hook.sh` - Called once after updating Rust & Carbon but before server start
- 🗺️ `generate-maps.sh` - Called 22 hours before wipes (if `generate_map: true`)

**⚠️ These are template scripts - you must edit them to match your infrastructure!**

Example customization:

```bash
# /opt/wipe-cli/stop-servers.sh
#!/bin/bash
SERVER_PATHS="$@"
for SERVER_PATH in $SERVER_PATHS; do
    IDENTITY=$(basename "$SERVER_PATH")
    systemctl stop "rs-${IDENTITY}"
done
```

You can regenerate all scripts to defaults with:

```bash
wipe reset-scripts
```

This will delete and regenerate all 4 management scripts.

## 💻 Usage

### 🎯 Initial Setup

Add servers to monitor with their calendar URLs:

```bash
# Add a server (minimum required flags)
wipe add \
  --path /var/www/servers/us-weekly \
  --calendar https://calendar.google.com/calendar/ical/xxx/basic.ics

# Add a server with all options
wipe add \
  --path /var/www/servers/us-weekly \
  --calendar https://calendar.google.com/calendar/ical/xxx/basic.ics \
  --branch main \
  --wipe-blueprints \
  --generate-map
```

**🚩 Flags:**
- 📁 `--path` - Full path to Rust server directory (required). Server name is derived from the basename.
- 📅 `--calendar` - Google Calendar .ics URL (required)
- 🌿 `--branch` - Rust branch: main, staging, etc. (default: main)
- 🧹 `--wipe-blueprints` - Delete blueprints on wipe events (default: false)
- 🗺️ `--generate-map` - Call generate-maps.sh before wipes (default: false)

**💡 Note:** The server name is automatically set to the basename of the path. For example, `/var/www/servers/us-weekly` becomes `us-weekly`.

### 🔧 Managing Servers

```bash
# List all configured servers
wipe list

# Update server settings (use server name or full path)
wipe update us-weekly \
  --calendar https://new-url.com/cal.ics \
  --branch staging \
  --generate-map

# Remove a server (use server name or full path)
wipe remove us-weekly
```

### ⚙️ Configuration

```bash
# View current configuration
wipe config

# Set global options
wipe config set --check-interval 30           # How often to check calendars (seconds)
wipe config set --lookahead-hours 24          # How far ahead to schedule events (hours)
wipe config set --event-delay 5               # Delay after event time (seconds)
wipe config set --map-generation-hours 22     # When to generate maps before wipe (hours)
wipe config set --discord-webhook "https://..." # General notifications webhook
```

### 🛠️ Manual Operations

```bash
# Update Rust and Carbon on servers (without stopping/starting)
wipe sync us-weekly eu-monthly
wipe sync us-weekly --force  # Skip confirmation prompt

# Manually call a management script for specific servers
wipe call-script us-weekly us-long --script stop-servers
wipe call-script us-weekly --script generate-maps
```

### 📊 Service Management

```bash
# Check service status
systemctl status wipe@$USER.service

# View logs
journalctl -u wipe@$USER.service -f

# Restart service
sudo systemctl restart wipe@$USER.service
```

## 📜 Management Scripts

### 🔧 Pre-Start Hook

The `pre-start-hook.sh` runs once after all servers are synced but before they start. Use it for:
- 🧹 Clearing caches
- 🔌 Updating plugins
- 💾 Running database migrations
- 📢 Sending custom notifications

Example:

```bash
#!/bin/bash
SERVER_PATHS="$@"

for SERVER_PATH in $SERVER_PATHS; do
    IDENTITY=$(basename "$SERVER_PATH")
    
    # Update custom configs
    /usr/local/bin/update-configs "$IDENTITY"
done

# Send notification
IDENTITIES=$(echo "$SERVER_PATHS" | xargs -n1 basename | paste -sd,)
curl -X POST "https://api.example.com/notify" -d "servers=$IDENTITIES"
```

### 🛑▶️ Stop/Start Servers

Customize `stop-servers.sh` and `start-servers.sh` to match your infrastructure:

```bash
#!/bin/bash
# stop-servers.sh example with systemd
SERVER_PATHS="$@"
for SERVER_PATH in $SERVER_PATHS; do
    IDENTITY=$(basename "$SERVER_PATH")
    systemctl stop "rs-${IDENTITY}"
done
```

### 🗺️ Map Generation

The `generate-maps.sh` script is called 22 hours before wipes (configurable) for servers with `generate_map: true`. Customize it to:
- 🎲 Pick random seeds/sizes
- 🎨 Generate custom maps using [rustmaps-cli](https://github.com/maintc/rustmaps-cli)
- ⚙️ Update server.cfg files
- 🔄 Handle map pool logic

The script receives server paths and should exit 0 on success.

### 🔄 Manual Sync

The `wipe sync` command allows you to manually update Rust and Carbon on specified servers from `/opt/rust/{branch}` and `/opt/carbon/{branch}`:

```bash
# Update one or more servers
wipe sync us-weekly
wipe sync us-weekly eu-monthly

# Skip confirmation prompt (for automation)
wipe sync us-weekly --force
```

**⚠️ Important notes:**
- ❌ This command does NOT stop or start servers
- ❌ This command does NOT delete any files (no wipe)
- ❌ This command does NOT run the pre-start hook
- ⚠️ You should stop servers before updating to avoid issues
- ✅ This is useful for manual updates outside of scheduled events

## 📝 Configuration File

Configuration is stored at `~/.config/wipe/config.yaml`:

```yaml
# How far ahead to look for events (in hours)
lookahead_hours: 24

# How often to check calendars (in seconds)
check_interval: 30

# How long to wait after event time before executing (in seconds)
event_delay: 5

# How many hours before a wipe to call generate-maps.sh
map_generation_hours: 22

# Discord webhook URL for notifications
discord_webhook: "https://discord.com/api/webhooks/..."

# Servers to monitor
servers:
  - name: "us-weekly"
    path: "/var/www/servers/us-weekly"
    calendar_url: "https://calendar.google.com/calendar/ical/xxx/basic.ics"
    branch: "main"
    wipe_blueprints: false
    generate_map: true
    
  - name: "eu-staging"
    path: "/var/www/servers/eu-staging"
    calendar_url: "https://calendar.google.com/calendar/ical/yyy/basic.ics"
    branch: "staging"
    wipe_blueprints: true
    generate_map: false
```

## 🎯 Event Detection & Scheduling

### 📅 Calendar Events

The daemon looks for events with these summaries (case-insensitive, trimmed):
- 🔄 `"restart"` - Server restart event
- 🧹 `"wipe"` - Server wipe event

If a server has both a restart and wipe at the same time, only the wipe is executed.

### 📊 Event Grouping

Events occurring at the same time are automatically grouped:
- ⚡ Multiple servers restarting at 11:00 → **One batch operation**
- 🔄 Restarts always execute before wipes when grouped (restarts are faster)

This minimizes downtime and ensures efficient execution.

### 🔄 Update Checking

The daemon checks for Rust and Carbon updates every 2 minutes:
- 🎮 **Rust**: Monitors each configured branch via SteamCMD
- 🔌 **Carbon**: Checks GitHub releases for production/staging builds
- 📦 Updates are automatically installed to `/opt/rust/{branch}` and `/opt/carbon/{branch}`
- 🛡️ Cascade protection prevents multiple simultaneous updates

## 🛠️ Development

Run the CLI locally:
```bash
make run-cli
```

Run the daemon locally:
```bash
make run-daemon
```

Run code quality checks:
```bash
make check  # Runs fmt, vet, staticcheck, and deadcode
```

## 🗑️ Uninstallation

```bash
make uninstall
```

This will:
- 🛑 Stop the service
- ❌ Remove binaries from `/usr/local/bin/`
- ❌ Remove systemd service file
- ✅ Config files in `~/.config/wipe/` and scripts in `/opt/wipe-cli/` are preserved

## 📦 Dependencies

- 🐍 [cobra](https://github.com/spf13/cobra) - CLI framework
- ⚙️ [viper](https://github.com/spf13/viper) - Configuration management
- 📅 [golang-ical](https://github.com/arran4/golang-ical) - iCalendar parsing
- 🔄 [rrule-go](https://github.com/teambition/rrule-go) - Recurring event support

## 📄 License

See [LICENSE](./LICENSE) file.

---

**Made with ❤️ by [mainloot](https://mainloot.com)**
