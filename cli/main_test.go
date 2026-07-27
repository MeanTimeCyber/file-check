package main

import (
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTimestampedCSVNameFormat(t *testing.T) {
	name := timestampedCSVName()
	matched, err := regexp.MatchString(`^file-check-\d{8}-\d{6}\.csv$`, name)
	if err != nil {
		t.Fatalf("regex error: %v", err)
	}
	if !matched {
		t.Fatalf("unexpected timestamped CSV name: %q", name)
	}
}

func TestDefaultWorkerCountBounds(t *testing.T) {
	count := defaultWorkerCount()
	if count < 4 || count > 12 {
		t.Fatalf("default worker count out of expected bounds: %d", count)
	}
}

func TestProcessDirectory_RecursiveWritesPassesAndPrintsFailures(t *testing.T) {
	tmpDir := t.TempDir()
	scanRoot := filepath.Join(tmpDir, "scan")
	if err := os.MkdirAll(filepath.Join(scanRoot, "sub"), 0o755); err != nil {
		t.Fatalf("create scan directories: %v", err)
	}

	passFile := filepath.Join(scanRoot, "pass.txt")
	if err := os.WriteFile(passFile, []byte("normal text content\n"), 0o644); err != nil {
		t.Fatalf("write pass file: %v", err)
	}

	passNested := filepath.Join(scanRoot, "sub", "pass2.txt")
	if err := os.WriteFile(passNested, []byte("more text content\n"), 0o644); err != nil {
		t.Fatalf("write nested pass file: %v", err)
	}

	unmappedFile := filepath.Join(scanRoot, "sub", "script.unknownext")
	if err := os.WriteFile(unmappedFile, []byte("#!/bin/sh\necho hi\n"), 0o644); err != nil {
		t.Fatalf("write unmapped extension file: %v", err)
	}

	failFile := filepath.Join(scanRoot, "sub", "fail.txt")
	pngHeader := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00, 0x00, 0x00, 0x0d}
	if err := os.WriteFile(failFile, pngHeader, 0o644); err != nil {
		t.Fatalf("write fail file: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir to temp dir: %v", err)
	}

	output, err := captureStdout(func() error {
		return processDirectory(scanRoot, 4)
	})
	if err != nil {
		t.Fatalf("processDirectory returned error: %v", err)
	}

	if !strings.Contains(output, failFile) {
		t.Fatalf("expected failure output to include %q; output:\n%s", failFile, output)
	}

	if strings.Contains(output, passFile) {
		t.Fatalf("did not expect pass file to be printed; output:\n%s", output)
	}

	if strings.Contains(output, unmappedFile) {
		t.Fatalf("did not expect unmapped-extension file to be printed; output:\n%s", output)
	}

	if !strings.Contains(output, "Processed 4 files") {
		t.Fatalf("expected summary count in output; output:\n%s", output)
	}

	if !strings.Contains(output, "passed=3") || !strings.Contains(output, "failed=1") {
		t.Fatalf("expected pass/fail summary in output; output:\n%s", output)
	}

	reportPath, rows := readSingleGeneratedCSV(t, tmpDir)
	_ = reportPath

	if len(rows) != 4 {
		t.Fatalf("expected 4 CSV rows (header + 3 CSV-eligible files), got %d", len(rows))
	}

	if got := strings.Join(rows[0], ","); got != "file_path,file_size,extension,sha256,mime_type,comment" {
		t.Fatalf("unexpected CSV header: %q", got)
	}

	csvBody := strings.Join([]string{strings.Join(rows[1], ","), strings.Join(rows[2], ","), strings.Join(rows[3], ",")}, "\n")
	if !strings.Contains(csvBody, passFile) || !strings.Contains(csvBody, passNested) || !strings.Contains(csvBody, unmappedFile) {
		t.Fatalf("expected CSV-eligible files in CSV body; rows: %v", rows)
	}

	if strings.Contains(csvBody, failFile) {
		t.Fatalf("did not expect fail file in CSV body; rows: %v", rows)
	}
}

func captureStdout(fn func() error) (string, error) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}

	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = oldStdout

	out, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		return "", readErr
	}

	return string(out), runErr
}

func readSingleGeneratedCSV(t *testing.T, dir string) (string, [][]string) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(dir, "file-check-*.csv"))
	if err != nil {
		t.Fatalf("glob CSV reports: %v", err)
	}

	if len(matches) != 1 {
		t.Fatalf("expected exactly one generated CSV report, got %d (%v)", len(matches), matches)
	}

	f, err := os.Open(matches[0])
	if err != nil {
		t.Fatalf("open CSV report: %v", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read CSV report: %v", err)
	}

	return matches[0], rows
}
