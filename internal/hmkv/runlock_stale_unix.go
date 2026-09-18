//go:build !windows

package hmkv

import (
	"os"
	"syscall"
)

func processStillRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
