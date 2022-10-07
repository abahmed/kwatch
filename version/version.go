package version

import (
	"encoding/json"
)

// Version is the current versions of kwatch
const version = "dev"

// GitCommitID git commit id of the release
const gitCommitID = "none"

// BuildDate date for the release
const buildDate = "unknown"

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
