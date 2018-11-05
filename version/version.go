package version

import (
	"fmt"

	"github.com/golang/glog"
)

var (
	// GitCommit will be set during make
	GitCommit = "None"
	// BuildDate will be set during make
	BuildDate = "None"
)

// PrintVersionInfo print version info to standard output
func PrintVersionInfo() {
	fmt.Println("Git Commit Hash:", GitCommit)
	fmt.Println("Build Date:", BuildDate)
}

// LogVersionInfo print version info to log
func LogVersionInfo() {
	glog.Infof("Welcome to Redis Operator.")
	glog.Infof("Git Commit Hash: %s", GitCommit)
	glog.Infof("Build Date: %s", BuildDate)
}
