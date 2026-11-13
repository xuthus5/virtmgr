package domain

type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

type SystemInfo struct {
	Build BuildInfo `json:"build"`
}
