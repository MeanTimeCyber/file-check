# file-check

`file-check` is a small Go CLI that inspects a single file and prints:

- file path
- file size
- file extension
- SHA-256 hash
- detected MIME type
- a note showing whether the detected MIME type matches the file extension

The MIME comparison uses an extension-to-MIME map derived from the list at `https://mimetype.io/all-types`.

## Usage

Run the CLI with a single file path:

```bash
go run ./cli /path/to/file
```

The command prints the results as a table.
