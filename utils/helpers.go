package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// EnsureDirectory creates a directory if it doesn't exist
func EnsureDirectory(path string) error {
	return os.MkdirAll(path, 0755)
}

// GenerateLogFileName generates a unique log file name based on the current time
func GenerateLogFileName(baseName string) string {
	timestamp := time.Now().Format("20060102-150405")
	return fmt.Sprintf("%s-%s.log", baseName, timestamp)
}

// CleanOldLogs removes log files older than the specified duration
func CleanOldLogs(logDir string, maxAge time.Duration) error {
	return filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && time.Since(info.ModTime()) > maxAge {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
		return nil
	})
}