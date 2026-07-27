package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"time"
)

// createTempReport creates the temporary output file and returns a cleanup function.
func createTempReport(reportPath string) (*os.File, string, func(), func(), error) {
	reportFile, err := os.CreateTemp(".", reportPath+".tmp-")
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("create temporary CSV report: %w", err)
	}

	reportTempPath := reportFile.Name()
	reportFinalized := false
	markFinalized := func() {
		reportFinalized = true
	}
	cleanup := func() {
		if reportFile != nil {
			_ = reportFile.Close()
		}
		if !reportFinalized {
			_ = os.Remove(reportTempPath)
		}
	}

	return reportFile, reportTempPath, markFinalized, cleanup, nil
}

// writeCSVHeader initializes the report writer and writes its header row.
func writeCSVHeader(reportFile *os.File) (*csv.Writer, error) {
	csvWriter := csv.NewWriter(reportFile)
	if err := csvWriter.Write([]string{"file_path", "file_size", "extension", "sha256", "mime_type", "comment"}); err != nil {
		return nil, fmt.Errorf("write CSV header: %w", err)
	}

	return csvWriter, nil
}

// finalizeTempReport fsyncs, closes, and atomically renames the temporary report.
func finalizeTempReport(reportFile *os.File, reportTempPath, reportPath string) error {
	if err := reportFile.Sync(); err != nil {
		return fmt.Errorf("sync CSV report: %w", err)
	}

	if err := reportFile.Close(); err != nil {
		return fmt.Errorf("close CSV report: %w", err)
	}

	if err := os.Rename(reportTempPath, reportPath); err != nil {
		return fmt.Errorf("finalize CSV report: %w", err)
	}

	return nil
}

// isGeneratedReportName returns true for timestamped report filenames produced by this tool.
func isGeneratedReportName(fileName string) bool {
	if !strings.HasPrefix(fileName, "file-check-") || !strings.HasSuffix(fileName, ".csv") {
		return false
	}

	stamp := strings.TrimSuffix(strings.TrimPrefix(fileName, "file-check-"), ".csv")
	if len(stamp) != len("20060102-150405") {
		return false
	}

	_, err := time.Parse("20060102-150405", stamp)
	return err == nil
}
