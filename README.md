# file-check

`file-check` is a Go CLI that can inspect a single file or recursively scan a directory.

For a single file, it prints:

- file path
- file size
- file extension
- SHA-256 hash
- detected MIME type
- a note showing whether the detected MIME type matches the file extension

For a directory scan, it:

- recursively processes regular files in the target folder and subfolders
- prints details only for files with an explicit MIME mismatch
- writes all other file details (including match and no-mapping cases) to a timestamped CSV report

The MIME comparison uses an extension-to-MIME map derived from the list at `https://mimetype.io/all-types`.

## Usage

Inspect one file:

```bash
go run ./cli /path/to/file
```

Scan a directory recursively:

```bash
go run ./cli /path/to/folder
```

Optional: set worker count for directory scans:

```bash
go run ./cli -workers 8 /path/to/folder
```

When scanning a directory, non-mismatch results are written to a CSV file named like:

```text
file-check-YYYYMMDD-HHMMSS.csv
```
