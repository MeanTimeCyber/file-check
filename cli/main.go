package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
)

// fileResult carries per-file worker output back to the directory aggregator.
type fileResult struct {
	path    string
	details *FileDetails
	err     error
}

// scanStats tracks aggregate scan outcomes for final reporting.
type scanStats struct {
	scanned int
	passed  int
	failed  int
	skipped int
	errored int
}

// main parses flags, validates input, and dispatches file or directory processing.
func main() {
	workers := flag.Int("workers", defaultWorkerCount(), "Number of worker goroutines for directory scans")
	maxSize := flag.String("max-size", "", "Skip files larger than this size (e.g. 500MB, 2GiB)")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Printf("Input path is required.\n")
		fmt.Printf("Usage: %s [-workers N] [-max-size SIZE] <file-or-directory>\n", os.Args[0])
		os.Exit(1)
	}

	inputPath := flag.Arg(0)

	inputInfo, err := os.Stat(inputPath)
	if err != nil {
		fmt.Printf("Input path is not accessible: %s\n", inputPath)
		flag.Usage()
		os.Exit(1)
	}

	if *workers < 1 {
		fmt.Printf("workers must be >= 1\n")
		os.Exit(1)
	}

	maxSizeBytes, err := parseMaxSize(*maxSize)
	if err != nil {
		fmt.Printf("Invalid max-size value: %v\n", err)
		os.Exit(1)
	}

	if inputInfo.IsDir() {
		if err := processDirectory(inputPath, *workers, maxSizeBytes); err != nil {
			fmt.Printf("Error processing directory: %s\n", err)
			os.Exit(1)
		}
		return
	}

	if maxSizeBytes > 0 && inputInfo.Size() > maxSizeBytes {
		fmt.Printf("Skipping file %s: size %s exceeds max-size %s\n", inputPath, humanize.Bytes(uint64(inputInfo.Size())), humanize.Bytes(uint64(maxSizeBytes)))
		return
	}

	// Preserve single-file behavior.
	details, err := getFileDetails(inputPath)
	if err != nil {
		fmt.Printf("Error processing file: %s\n", err)
		os.Exit(1)
	}

	details.PrettyPrint()
}

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
