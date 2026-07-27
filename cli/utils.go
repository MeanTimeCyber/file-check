package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

// shouldSkipScanFile reports whether a path should be excluded from directory scanning.
func shouldSkipScanFile(path string, excludedPaths map[string]struct{}) bool {
	if _, ok := excludedPaths[absoluteCleanPath(path)]; ok {
		return true
	}

	return isGeneratedReportName(filepath.Base(path))
}

// absoluteCleanPath returns a normalized absolute path when possible.
func absoluteCleanPath(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}

	return filepath.Clean(absPath)
}

// isMimeCheckPass classifies non-mismatch comments as CSV-eligible results.
func isMimeCheckPass(comment string) bool {
	return !strings.HasPrefix(comment, "Mismatch:")
}

// timestampedCSVName builds a report filename using the current local time.
func timestampedCSVName() string {
	stamp := time.Now().Format("20060102-150405")
	return fmt.Sprintf("file-check-%s.csv", stamp)
}

// parseMaxSize parses a human-readable byte size string and returns bytes.
func parseMaxSize(value string) (int64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}

	size, err := humanize.ParseBytes(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%q (%w)", value, err)
	}

	if size == 0 {
		return 0, fmt.Errorf("%q must be greater than zero", value)
	}

	if size > uint64(math.MaxInt64) {
		return 0, fmt.Errorf("%q is too large", value)
	}

	return int64(size), nil
}
