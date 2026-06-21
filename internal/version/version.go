package version

import (
	"encoding/json"
)

// Version is the current versions of kwatch.
// Overridden at build time with -ldflags -X for releases.
var version = "dev"

// GitCommitID git commit id of the release
var gitCommitID = "none"

// BuildDate date for the release
var buildDate = "unknown"

type Info struct {
	Version   string
	GitCommit string
	BuildDate string
}

func Short() string {
	return version
}

func Version() string {
	ver, _ := json.Marshal(Info{
		Version:   version,
		GitCommit: gitCommitID,
		BuildDate: buildDate,
	})

	return string(ver)
}
