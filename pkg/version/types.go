package version

// Info version info
type Info struct {
	GitVersion   string `json:"gitVersion"`
	GitCommit    string `json:"gitCommit"`
	BuildDate    string `json:"buildDate"`
	GoVersion    string `json:"goVersion"`
	Compiler     string `json:"compiler"`
	Platform     string `json:"platform"`
	RedisVersion string `json:"redisVersion"`
}

// String returns info as a human-friendly version string.
func (info Info) String() string {
	return info.GitVersion
}
