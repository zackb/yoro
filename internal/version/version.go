// Package version exposes build metadata, injected at link time via -ldflags.
package version

var (
	// Version is the semantic version, set with
	// -ldflags "-X github.com/zackb/yoro/internal/version.Version=...".
	Version = "dev"
	// Commit is the short git SHA of the build.
	Commit = "none"
	// Date is the build date.
	Date = "unknown"
)

// String returns a human-readable version string.
func String() string {
	return Version + " (" + Commit + ", " + Date + ")"
}
