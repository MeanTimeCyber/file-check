package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckMimeAgainstExtension(t *testing.T) {
	t.Run("match", func(t *testing.T) {
		got := checkMimeAgainstExtension(".txt", "text/plain")
		if !strings.HasPrefix(got, "Matches expected MIME") {
			t.Fatalf("expected match comment, got %q", got)
		}
	})

	t.Run("alias normalized", func(t *testing.T) {
		got := checkMimeAgainstExtension(".jpg", "image/pjpeg")
		if !strings.HasPrefix(got, "Matches expected MIME") {
			t.Fatalf("expected normalized match comment, got %q", got)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		got := checkMimeAgainstExtension(".txt", "image/png")
		if !strings.HasPrefix(got, "Mismatch:") {
			t.Fatalf("expected mismatch comment, got %q", got)
		}
	})

	t.Run("missing mapping", func(t *testing.T) {
		got := checkMimeAgainstExtension(".notrealext", "text/plain")
		if got != "No MIME mapping found for this extension" {
			t.Fatalf("unexpected comment: %q", got)
		}
	})
}

func TestIsMimeCheckPass(t *testing.T) {
	if !isMimeCheckPass("Matches expected MIME for .txt") {
		t.Fatalf("expected pass for match comment")
	}

	if isMimeCheckPass("Mismatch: expected text/plain for .txt") {
		t.Fatalf("expected mismatch comment to fail")
	}

	if !isMimeCheckPass("No MIME mapping found for this extension") {
		t.Fatalf("expected missing mapping comment to be CSV-eligible")
	}
}

func TestGetFileDetails_NoExtensionGetsComment(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample")

	if err := os.WriteFile(filePath, []byte("hello from file-check tests\n"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	details, err := getFileDetails(filePath)
	if err != nil {
		t.Fatalf("getFileDetails returned error: %v", err)
	}

	if details.Extension != "" {
		t.Fatalf("expected empty extension, got %q", details.Extension)
	}

	if details.Comment != "No MIME mapping found for this extension" {
		t.Fatalf("unexpected comment for extensionless file: %q", details.Comment)
	}
}
