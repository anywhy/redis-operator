package e2e

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo" // revive:disable:dot-imports
)

func execCmd(cmdStr string) (string, error) {
	logf(fmt.Sprintf("$ %s\n", cmdStr))
	result, err := exec.Command("/bin/sh", "-c", cmdStr).CombinedOutput()
	resultStr := string(result)
	logf(resultStr)
	if err != nil {
		logf(err.Error())
		return resultStr, err
	}

	return resultStr, nil
}

func getInstanceName(ns, clusterName string) string {
	return fmt.Sprintf("%s-%s", ns, clusterName)
}

// logf log a message in INFO format
func logf(format string, args ...interface{}) {
	log("INFO", format, args...)
}

func log(level string, format string, args ...interface{}) {
	fmt.Fprintf(GinkgoWriter, nowStamp()+": "+level+": "+format+"\n", args...)
}

func nowStamp() string {
	return time.Now().Format(time.StampMilli)
}
