package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	args := os.Args

	if len(args) != 2 {
		fmt.Printf("Input file is required.")
		os.Exit(1)
	}

	inputFile := args[1]

	// check input file exists
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		fmt.Printf("Input file does not exist: %s\n", inputFile)
		flag.Usage()
		os.Exit(1)
	}

	// check it's not a directory
	if fileInfo, err := os.Stat(inputFile); err == nil && fileInfo.IsDir() {
		fmt.Printf("Input file is a directory: %s\n", inputFile)
		flag.Usage()
		os.Exit(1)
	}

	// Process the input file
	details, err := getFileDetails(inputFile)

	if err != nil {
		fmt.Printf("Error processing file: %s\n", err)
		os.Exit(1)
	}

	details.PrettyPrint()
}
