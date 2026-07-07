package main

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

var version = ""

const revisionShortLen = 12

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

	if len(revision) > revisionShortLen {
		revision = revision[:revisionShortLen]
	}

	if modified == "true" {
		return "dev+" + revision + "-dirty"
	}

	return "dev+" + revision
}

func runVersionCmd(args []string, stdout, _ io.Writer) int {
	for _, arg := range args {
		if arg == "-h" || arg == flagHelp {
			versionHelp(stdout)

			return 0
		}
	}

	return runVersion(stdout)
}

func versionHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: lazy-tmux version

Print the version (also: lazy-tmux --version, lazy-tmux -v)
`)
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
