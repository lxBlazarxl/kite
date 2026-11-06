// Package version holds the build version, set at release time with
//
//	-ldflags "-X github.com/retransmit/kite/internal/version.Version=v1.2.3"
package version

// Version is "dev" for builds that were not made by the release script.
var Version = "dev"
