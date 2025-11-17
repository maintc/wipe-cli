package version

// These variables are set at build time via ldflags
var (
	Version   = "dev"      // Injected via -ldflags "-X internal/version.Version=v1.2.3"
	GitCommit = "unknown"  // Injected via -ldflags "-X internal/version.GitCommit=abc123"
	BuildDate = "unknown"  // Injected via -ldflags "-X internal/version.BuildDate=2024-01-01"
)

// GetVersion returns the full version string
func GetVersion() string {
	if Version == "dev" {
		return "dev (commit: " + GitCommit + ")"
	}
	return Version
}

// GetFullVersion returns the version with all metadata
func GetFullVersion() string {
	return "wipe-cli " + GetVersion() + " (built: " + BuildDate + ")"
}

