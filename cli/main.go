package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dustin/go-humanize"
)

// main parses flags, validates input, and dispatches file or directory processing.
func main() {
	// Define command-line flags for worker count and maximum file size.
	workers := flag.Int("workers", defaultWorkerCount(), "Number of worker goroutines for directory scans")
	maxSize := flag.String("max-size", "", "Skip files larger than this size (e.g. 500MB, 2GiB)")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Printf("Input path is required.\n")
		fmt.Printf("Usage: %s [-workers N] [-max-size SIZE] <file-or-directory>\n", os.Args[0])
		os.Exit(1)
	}

	inputPath := flag.Arg(0)

	// Validate the input path and check if it's a file or directory.
	inputInfo, err := os.Stat(inputPath)
	if err != nil {
		fmt.Printf("Input path is not accessible: %s\n", inputPath)
		flag.Usage()
		os.Exit(1)
	}

	// Validate the worker count.
	if *workers < 1 {
		fmt.Printf("workers must be >= 1\n")
		os.Exit(1)
	}

	// Parse the maximum size if provided.
	maxSizeBytes, err := parseMaxSize(*maxSize)
	if err != nil {
		fmt.Printf("Invalid max-size value: %v\n", err)
		os.Exit(1)
	}

	// If the input path is a directory, process it recursively; otherwise, process the single file.
	if inputInfo.IsDir() {
		if err := processDirectory(inputPath, *workers, maxSizeBytes); err != nil {
			fmt.Printf("Error processing directory: %s\n", err)
			os.Exit(1)
		}
		return
	}

	// If the input path is a file, check its size against the max-size limit.
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

	// Print the details of the single file in a tabular format.
	details.PrettyPrint()

	fmt.Println("Fin.")
}
