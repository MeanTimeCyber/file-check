package main

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