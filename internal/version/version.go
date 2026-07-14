// Package version carries the build identity stamped in via -ldflags at
// release time (see .goreleaser.yaml). A source build reports "dev".
package version

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// String is the status-bar form: "dev" locally, "1.0.0" on a release build.
func String() string {
	return Version
}

// IsRelease reports whether this binary was stamped by a release build
// (goreleaser ldflags set Version). Source builds stay "dev" — release-only
// behavior (self-update hints, telemetry) gates on this.
func IsRelease() bool {
	return Version != "dev"
}
