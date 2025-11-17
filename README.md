# 🧹 Wipe CLI

<a href="https://discord.gg/mainloot"><img src="https://mainloot.s3.us-west-2.amazonaws.com/Mainloot_Logo_OnBlack.png" alt="Mainloot Logo" style="width: 50%; height: auto;"></a>

[![CI Status](https://github.com/maintc/wipe-cli/actions/workflows/build.yml/badge.svg)](https://github.com/maintc/wipe-cli/actions/workflows/build.yml)
[![Go Coverage](https://github.com/maintc/wipe-cli/wiki/coverage.svg)](https://raw.githack.com/wiki/maintc/wipe-cli/coverage.html)
[![GitHub Release](https://img.shields.io/github/v/release/maintc/wipe-cli)](https://github.com/maintc/wipe-cli/releases/latest)
[![Platform](https://img.shields.io/badge/platform-linux-blue)](https://github.com/maintc/wipe-cli)

**Automated Rust game server management powered by Google Calendar schedules.**

## 📖 What It Does

> **⚠️ Note**: This tool is designed for Linux servers only. Windows is not supported.

> **💡 Tip**: Pairs well with [WipeCal](https://github.com/maintc/WipeCal) - a Carbon plugin that uses the same calendar URL to notify players of approaching restarts/wipes and display upcoming events in-game.

Server owners can schedule restart and wipe events in Google Calendar. This tool:

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

The two components communicate via a shared configuration file stored at `~/.config/wiped/config.yaml`.

### ⚙️ Event Execution Flow

When a restart or wipe event occurs:

1. 🛑 **Stop servers** → Calls `/opt/wiped/stop-servers.sh` with server paths
2. 📦 **Update Rust & Carbon** → Syncs from `/opt/rust/{branch}` and `/opt/carbon/{branch}` (parallel)
3. 🧹 **Wipe data** (wipes only) → Deletes map, save, and blueprint files (see below)
4. 🔧 **Run hook** → Calls `/opt/wiped/pre-start-hook.sh` with all server paths
5. ▶️ **Start servers** → Calls `/opt/wiped/start-servers.sh` with server paths

All scripts receive server paths as arguments, allowing you to integrate with your existing infrastructure.

**Files deleted during wipes** (from `server/{identity}/` directory):
- `*.map` - Map files
- `*.sav*` - Save files
- `player.states.*.db*` - Player state databases
- `sv.files.*.db*` - Server file databases
- `player.blueprints.*` - Blueprints (only if `wipe_blueprints: true`)

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
│   └── wiped.service  # systemd service file
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
sudo systemctl enable wiped@$USER.service
sudo systemctl start wiped@$USER.service
```

**Note:** The service uses `wiped@{username}.service` format - replace `$USER` with your actual username if needed. The daemon runs as your user and accesses your `~/.config/wiped/config.yaml`.

### 📜 Management Scripts

The daemon automatically creates default management scripts in `/opt/wiped/` on first run:

- 🛑 `stop-servers.sh` - Called to stop servers before restart/wipe
- ▶️ `start-servers.sh` - Called to start servers after restart/wipe
- 🔧 `pre-start-hook.sh` - Called after updating Rust & Carbon but before server start
- 🗺️ `generate-maps.sh` - Called by default 22 hours before wipes (if `generate_map: true`)

**⚠️ These are template scripts - you must edit them to match your infrastructure!**

Example customization:

```bash
# /opt/wiped/stop-servers.sh
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

# Update server settings (accepts server name or full path)
wipe update us-weekly \
  --calendar https://new-url.com/cal.ics \
  --branch staging \
  --generate-map

# You can also use full path
wipe update /var/www/servers/us-weekly --branch main

# Remove a server (accepts server name or full path)
wipe remove us-weekly
# Or: wipe remove /var/www/servers/us-weekly
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

### 📢 Discord Mentions

Configure user and role IDs to mention in Discord notifications:

```bash
# Add Discord users to mention (use Discord user IDs)
wipe mention add-user 123456789012345678
wipe mention add-user 987654321098765432

# Add Discord roles to mention (use Discord role IDs)
wipe mention add-role 111222333444555666
wipe mention add-role 777888999000111222

# Remove users or roles
wipe mention remove-user 123456789012345678
wipe mention remove-role 111222333444555666

# View configured mentions
wipe config
```

**How to get Discord IDs:**
1. Enable Developer Mode in Discord (Settings → App Settings → Advanced → Developer Mode)
2. Right-click on a user or role and select "Copy ID"

Configured mentions will be included in batch event notifications (start, complete, errors) as `cc <@&ROLE_ID> <@USER_ID>`.

### 🛠️ Manual Operations

```bash
# Update Rust and Carbon on servers (without stopping/starting)
wipe sync us-weekly eu-monthly
wipe sync us-weekly --force  # Skip confirmation prompt

# Manually call a management script for specific servers
wipe call-script us-weekly us-long --script stop-servers
wipe call-script us-weekly --script start-servers
wipe call-script us-weekly --script generate-maps

# Reset all management scripts to defaults (includes pre-start-hook.sh)
wipe reset-scripts
wipe reset-scripts --force  # Skip confirmation prompt
```

### 📊 Service Management

```bash
# Check service status
systemctl status wiped@$USER.service

# View logs
journalctl -u wiped@$USER.service -f

# Restart service
sudo systemctl restart wiped@$USER.service

# Run daemon with custom config path (for testing)
wiped -config /path/to/custom/config.yaml
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

Configuration is stored at `~/.config/wiped/config.yaml`:

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

# Discord user IDs to mention in notifications (optional)
discord_mention_users:
  - "123456789012345678"
  - "987654321098765432"

# Discord role IDs to mention in notifications (optional)
discord_mention_roles:
  - "111222333444555666"
  - "777888999000111222"

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

Events occurring at the same time are automatically grouped into **one unified batch**:
- ⚡ **All servers stop at once** (prevents systemd from auto-restarting during updates)
- 🚀 **All servers update in parallel** (Rust + Carbon synced simultaneously)
- 🧹 **Wipe-specific cleanup** only runs for servers with wipe events
- 🔧 **Pre-start hook runs once** for all servers in the batch
- ✅ **All servers start together**

**Example:** 2 servers restarting + 2 servers wiping at 11:00 → **One batch operation**

This ensures:
- No race conditions with systemd auto-restart
- Minimal downtime
- Efficient parallel execution

### 🔄 Update Checking

The daemon checks for Rust and Carbon updates every 2 minutes:
- 🎮 **Rust**: Monitors each configured branch via SteamCMD
- 🔌 **Carbon**: Checks GitHub releases for production/staging builds
- 📦 Updates are automatically installed to `/opt/rust/{branch}` and `/opt/carbon/{branch}`
- 🛡️ Cascade protection prevents multiple simultaneous updates

### 📢 Discord Notifications

The daemon sends webhook notifications for key events:

**🎯 Event Operations:**
- `Batch Event Starting` - When servers begin restart/wipe operations
- `Batch Event Complete` - After successful completion
- `Batch Event Failed` - If any step fails during execution

**📅 Calendar Changes:**
- `Calendar Events Added` - New events detected in calendars
- `Calendar Events Removed` - Events deleted from calendars

**🔄 Installation & Updates:**
- `Rust Installation Complete` - Initial Rust branch installation
- `Rust Update Complete` - Rust branch updated to new build
- `Rust Update Available` - New Rust build detected (before install)
- `Rust Installation Failed` - Rust installation error
- `Carbon Installation Complete` - Initial Carbon installation
- `Carbon Update Available` - New Carbon version detected
- `Carbon Installation Failed` - Carbon installation error

**⚙️ Service Management:**
- `Wipe Service Started` - Daemon startup notification
- `Server Added` - Server added to configuration
- `Server Removed` - Server removed from configuration
- `Map Generation Failed` - generate-maps.sh script error

All notifications include the hostname for easy identification in multi-server environments.

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
- ✅ Config files in `~/.config/wiped/` and scripts in `/opt/wiped/` are preserved

## 📦 Dependencies

- 🐍 [cobra](https://github.com/spf13/cobra) - CLI framework
- ⚙️ [viper](https://github.com/spf13/viper) - Configuration management
- 📅 [golang-ical](https://github.com/arran4/golang-ical) - iCalendar parsing
- 🔄 [rrule-go](https://github.com/teambition/rrule-go) - Recurring event support
- ⏰ [gocron](https://github.com/go-co-op/gocron) - Job scheduling and execution

## 📄 License

See [LICENSE](./LICENSE) file.

---

**Made with ❤️ by [mainloot](https://mainloot.com)**
