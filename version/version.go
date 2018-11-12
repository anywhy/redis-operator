package version

import (
	"fmt"
	"runtime"

	"github.com/golang/glog"
)

// PrintVersionInfo print version info to standard output
func PrintVersionInfo() {
	fmt.Printf("Redis-Operator Version: %#v", Get())
}

// LogVersionInfo print version info to log
func LogVersionInfo() {
	glog.Infof("Welcome to Redis Operator.")
	glog.Infof("Redis-Operator Version: %#v", Get())
}

// Get returns the overall codebase version. It's for detecting
// what code a binary was built from.
func Get() Info {
	// These variables typically come from -ldflags settings and in
	// their absence fallback to the settings in pkg/version/base.go
	return Info{
		GitVersion:   gitVersion,
		GitCommit:    gitCommit,
		BuildDate:    buildDate,
		RedisVersion: redisVersion,
		GoVersion:    runtime.Version(),
		Compiler:     runtime.Compiler,
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}
