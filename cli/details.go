package main

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/gabriel-vasile/mimetype"
	"github.com/markkurossi/tabulate"
)

type FileDetails struct {
	FilePath  string
	FileSize  int64
	Extension string
	SHA256    string
	MimeType  string
	Comment   string
}

// getFileDetails processes the file at the given path and returns its details, including size, extension, SHA256 hash, MIME type, and any relevant comments about mismatches or issues.
func getFileDetails(filePath string) (*FileDetails, error) {

	details := FileDetails{
		FilePath: filePath,
	}

	// get the file size
	fileInfo, err := os.Stat(filePath)

	if err != nil {
		log.Printf("Error getting file info for %q: %q. Skipping it...", details.FilePath, err)
		return nil, err
	}

	details.FileSize = fileInfo.Size()

	// create a hasher for this worker to use, so we don't have to worry about locking it
	hasher := sha256.New()

	// get the file extension
	details.Extension = strings.ToLower(filepath.Ext(details.FilePath))

	// open the file once: hash it and detect MIME type in a single pass
	hash, mimeStr, err := hashAndDetectMime(hasher, details.FilePath)

	if err != nil {
		log.Printf("Error processing file %q: %q. Skipping it...", details.FilePath, err)
		return nil, err
	}

	details.SHA256 = hash
	details.MimeType = mimeStr

	if details.Extension != "" {
		details.Comment = checkMimeAgainstExtension(details.Extension, details.MimeType)
	}

	return &details, nil
}

// checkMimeAgainstExtension compares the detected MIME type against the expected MIME type for the file extension, accounting for common aliases and known variations.
func checkMimeAgainstExtension(extension, detectedMime string) string {
	expectedMime, ok := extensionToMIME[strings.ToLower(extension)]
	
	if !ok {
		return "No MIME mapping found for this extension"
	}

	normalizedDetected := normalizeMimeType(detectedMime)
	normalizedExpected := normalizeMimeType(expectedMime)

	if normalizedDetected == normalizedExpected || isKnownExtensionVariant(extension, normalizedDetected) {
		return fmt.Sprintf("Matches expected MIME for %s", extension)
	}

	return fmt.Sprintf("Mismatch: expected %s for %s", normalizedExpected, extension)
}

// normalizeMimeType standardizes MIME types for comparison, handling common aliases and variations.
func normalizeMimeType(mimeType string) string {
	normalized := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))

	if alias, ok := mimeAliases[normalized]; ok {
		return alias
	}

	switch normalized {
	case "application/x-debian-package":
		return "application/vnd.debian.binary-package"
	case "application/x-gzip":
		return "application/gzip"
	case "application/x-rar-compressed":
		return "application/vnd.rar"
	case "application/x-sh":
		return "application/x-shellscript"
	case "image/pjpeg":
		return "image/jpeg"
	case "text/xml":
		return "application/xml"
	default:
		return normalized
	}
}

// isKnownExtensionVariant checks for known cases where a file extension may be associated with multiple valid MIME types, or where common aliases exist.
func isKnownExtensionVariant(extension, detectedMime string) bool {
	switch strings.ToLower(extension) {
	case ".3g2":
		return detectedMime == "audio/3gpp2" || detectedMime == "video/3gpp2" || detectedMime == "audio/mp4"
	case ".3gp":
		return detectedMime == "video/3gpp" || detectedMime == "audio/mp4"
	case ".deb":
		return detectedMime == "application/vnd.debian.binary-package"
	case ".gz":
		return detectedMime == "application/gzip"
	case ".jpe", ".jpeg", ".jpg":
		return detectedMime == "image/jpeg"
	case ".m4a":
		return detectedMime == "audio/aac" || detectedMime == "audio/mp4"
	case ".mp4":
		return detectedMime == "audio/mp4" || detectedMime == "video/mp4"
	case ".pl":
		return detectedMime == "application/x-perl" || detectedMime == "text/plain"
	case ".rar":
		return detectedMime == "application/vnd.rar"
	case ".sh":
		return detectedMime == "application/x-shellscript"
	case ".wav":
		return detectedMime == "audio/vnd.wav"
	case ".xml":
		return detectedMime == "application/xml" || detectedMime == "application/rss+xml"
	case ".zip":
		return detectedMime == "application/zip"
	default:
		return false
	}
}

// hashAndDetectMime opens the file once, hashing it while detecting the MIME type.
func hashAndDetectMime(h hash.Hash, filePathString string) (string, string, error) {
	h.Reset()

	f, err := os.Open(filePathString)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	// TeeReader feeds bytes to the hasher as mimetype reads the header
	tr := io.TeeReader(f, h)
	mime, err := mimetype.DetectReader(tr)
	if err != nil {
		return "", "", err
	}

	// hash the rest of the file (mimetype only reads the header)
	if _, err := io.Copy(h, f); err != nil {
		return "", "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), mime.String(), nil
}

// PrettyPrint outputs the file details in a nicely formatted table.
func (f *FileDetails) PrettyPrint() {
	table := tabulate.New(tabulate.Unicode)
	table.Header("Field")
	table.Header("Value")

	row := table.Row()
	row.Column("File Path")
	row.Column(f.FilePath)
	row = table.Row()
	row.Column("File Size")
	row.Column(humanize.Bytes(uint64(f.FileSize)))
	row = table.Row()
	row.Column("Extension")
	row.Column(f.Extension)
	row = table.Row()
	row.Column("SHA256")
	row.Column(f.SHA256)
	row = table.Row()
	row.Column("MIME Type")
	row.Column(f.MimeType)
	
	if f.Comment != "" {
		row = table.Row()
		row.Column("Comment")
		row.Column(f.Comment)
	}

	fmt.Println(table.String())
}
