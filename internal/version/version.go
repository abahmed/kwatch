package version

import "encoding/json"

// Version is the current versions of kwatch.
// Overridden at build time with -ldflags -X for releases.
var version = "dev"

// GitCommitID git commit id of the release
var gitCommitID = "none"

// BuildDate date for the release
var buildDate = "unknown"

func Short() string {
	return version
}

// Info describes the build identity embedded in a release artifact. Keeping
// this separate from Short preserves the long-standing --version output for
// scripts while giving operators a way to verify an image's source commit.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
}

func Current() Info {
	return Info{Version: version, Commit: gitCommitID, BuildDate: buildDate}
}

func JSON() ([]byte, error) {
	return json.Marshal(Current())
}
