package main

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

// version is injected at release time via -ldflags "-X main.version=<tag>"
// (goreleaser passes the git tag). It is intentionally empty for plain
// `go build` / `go install`, where resolveVersion derives the version from the
// embedded build info instead — so the version is never hand-maintained in
// source and the git tag is the single source of truth.
var version = ""

// resolveVersion returns the build's version, preferring the ldflags-injected
// release version, then the module version recorded by `go install ...@vX.Y.Z`,
// then the VCS revision for local builds, and finally "dev".
func resolveVersion() string {
	if version != "" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}

	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}

	var revision, modified string
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}

	if revision == "" {
		return "dev"
	}

	if len(revision) > 12 {
		revision = revision[:12]
	}

	if modified == "true" {
		return "dev+" + revision + "-dirty"
	}

	return "dev+" + revision
}

func runVersion(stdout io.Writer) int {
	_, _ = fmt.Fprintf(
		stdout,
		"lazy-tmux %s (%s/%s, %s)\n",
		resolveVersion(),
		runtime.GOOS,
		runtime.GOARCH,
		runtime.Version(),
	)

	return 0
}
