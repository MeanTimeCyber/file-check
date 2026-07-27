package main

import (
	"encoding/csv"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
)

// defaultWorkerCount returns a laptop-safe default worker count for directory scans.
func defaultWorkerCount() int {
	n := runtime.NumCPU()
	if n < 4 {
		return 4
	}
	if n > 12 {
		return 12
	}
	return n
}

// processDirectory scans files recursively, prints mismatches, and writes other results to CSV.
func processDirectory(rootPath string, workers int, maxSizeBytes int64) error {
	reportPath := timestampedCSVName()
	reportFile, reportTempPath, markReportFinalized, cleanupReport, err := createTempReport(reportPath)
	if err != nil {
		return err
	}
	defer cleanupReport()

	csvWriter, err := writeCSVHeader(reportFile)
	if err != nil {
		return err
	}

	excludedPaths := buildExcludedPaths(reportTempPath, reportPath)
	jobs := make(chan string, workers*4)
	results := make(chan fileResult, workers*4)

	stats := scanStats{}
	var workerWG sync.WaitGroup
	startWorkerPool(workers, jobs, results, &workerWG)
	startDirectoryWalk(rootPath, maxSizeBytes, excludedPaths, jobs, results, &stats)

	go func() {
		workerWG.Wait()
		close(results)
	}()

	if err := consumeScanResults(results, csvWriter, &stats); err != nil {
		return err
	}

	if err := finalizeTempReport(reportFile, reportTempPath, reportPath); err != nil {
		return err
	}
	markReportFinalized()

	fmt.Printf("Processed %d files (passed=%d, failed=%d, skipped=%d, errors=%d)\n", stats.scanned, stats.passed, stats.failed, stats.skipped, stats.errored)
	fmt.Printf("CSV report: %s\n", reportPath)

	return nil
}

// buildExcludedPaths returns paths that should never be treated as scan inputs.
func buildExcludedPaths(reportTempPath, reportPath string) map[string]struct{} {
	return map[string]struct{}{
		absoluteCleanPath(reportTempPath): {},
		absoluteCleanPath(reportPath):     {},
	}
}

// startWorkerPool starts workers that process file jobs and emit results.
func startWorkerPool(workers int, jobs <-chan string, results chan<- fileResult, wg *sync.WaitGroup) {
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range jobs {
				details, err := getFileDetails(filePath)
				results <- fileResult{path: filePath, details: details, err: err}
			}
		}()
	}
}

// startDirectoryWalk traverses the directory and schedules eligible files for processing.
func startDirectoryWalk(rootPath string, maxSizeBytes int64, excludedPaths map[string]struct{}, jobs chan<- string, results chan<- fileResult, stats *scanStats) {
	go func() {
		walkErr := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				results <- fileResult{path: path, err: walkErr}
				return nil
			}

			if d.IsDir() || shouldSkipScanFile(path, excludedPaths) || d.Type()&os.ModeSymlink != 0 {
				return nil
			}

			info, infoErr := d.Info()
			if infoErr != nil {
				results <- fileResult{path: path, err: infoErr}
				return nil
			}

			if !info.Mode().IsRegular() {
				return nil
			}

			if maxSizeBytes > 0 && info.Size() > maxSizeBytes {
				stats.skipped++
				return nil
			}

			jobs <- path
			return nil
		})

		if walkErr != nil {
			results <- fileResult{path: rootPath, err: walkErr}
		}

		close(jobs)
	}()
}

// consumeScanResults routes each processed result to CSV, console, or error counters.
func consumeScanResults(results <-chan fileResult, csvWriter *csv.Writer, stats *scanStats) error {
	for result := range results {
		if result.err != nil {
			stats.errored++
			log.Printf("Error processing %q: %v", result.path, result.err)
			continue
		}

		if result.details == nil {
			stats.errored++
			log.Printf("No details returned for %q", result.path)
			continue
		}

		stats.scanned++

		if isMimeCheckPass(result.details.Comment) {
			if err := csvWriter.Write([]string{
				result.details.FilePath,
				strconv.FormatInt(result.details.FileSize, 10),
				result.details.Extension,
				result.details.SHA256,
				result.details.MimeType,
				result.details.Comment,
			}); err != nil {
				return fmt.Errorf("write CSV record: %w", err)
			}

			stats.passed++
			continue
		}

		stats.failed++
		result.details.PrettyPrint()
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return fmt.Errorf("flush CSV report: %w", err)
	}

	return nil
}
