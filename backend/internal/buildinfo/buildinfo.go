package buildinfo

import "runtime"

var (
	Version = "development"
	Commit  = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	GoVersion string `json:"goVersion"`
}

func Current() Info {
	return Info{Version: Version, Commit: Commit, GoVersion: runtime.Version()}
}
