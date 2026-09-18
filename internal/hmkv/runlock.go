package hmkv

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const finalizeLockName = "finalize.lock"

// finalizeLockPathOverride is set in tests only.
var finalizeLockPathOverride string

func finalizeLockPath() (string, error) {
	if finalizeLockPathOverride != "" {
		return finalizeLockPathOverride, nil
	}
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA environment variable is not set")
		}
		return filepath.Join(appData, "handymkv", finalizeLockName), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "handymkv", finalizeLockName), nil
}

func withFinalizeLock(fn func() error) error {
	lockPath, err := finalizeLockPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(lockPath), 0740); err != nil {
		return fmt.Errorf("could not create finalize lock directory: %w", err)
	}

	deadline := time.Now().Add(2 * time.Hour)
	for time.Now().Before(deadline) {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			runErr := fn()
			_ = f.Close()
			_ = os.Remove(lockPath)
			return runErr
		}
		if !os.IsExist(err) {
			return fmt.Errorf("could not acquire finalize lock: %w", err)
		}

		if staleFinalizeLock(lockPath) {
			_ = os.Remove(lockPath)
			continue
		}

		time.Sleep(250 * time.Millisecond)
	}

	return fmt.Errorf("timed out waiting for another HandyMKV run to finish library organization")
}

func staleFinalizeLock(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return true
	}

	pidStr := strings.TrimSpace(strings.Split(string(data), "\n")[0])
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return true
	}

	if processStillRunning(pid) {
		return false
	}
	return true
}
