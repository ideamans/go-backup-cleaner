# go-backup-cleaner

[![Test](https://github.com/ideamans/go-backup-cleaner/actions/workflows/test.yml/badge.svg)](https://github.com/ideamans/go-backup-cleaner/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ideamans/go-backup-cleaner.svg)](https://pkg.go.dev/github.com/ideamans/go-backup-cleaner)

A Go package for cleaning backup files based on capacity constraints. It automatically removes old files to meet specified disk usage targets and can clean up empty directories.

## Features

- Multiple capacity constraint options (max size, max usage percentage, min free space)
- Parallel file scanning and deletion for performance
- Time-windowed aggregation for memory efficiency with large file sets
- Block-size aware deletion (actual disk space freed)
- Callback system for progress monitoring
- Empty directory cleanup
- Cross-platform support (Linux, macOS, Windows)

## Installation

```bash
go get github.com/ideamans/go-backup-cleaner
```

## Usage

```go
package main

import (
    "log"
    cleaner "github.com/ideamans/go-backup-cleaner"
)

func main() {
    // Set 80% as maximum disk usage
    maxUsage := 80.0
    config := cleaner.CleaningConfig{
        MaxUsagePercent: &maxUsage,
        RemoveEmptyDirs: true,
        Callbacks: cleaner.Callbacks{
            OnFileDeleted: func(info cleaner.FileDeletedInfo) {
                log.Printf("Deleted: %s (%d bytes)", info.Path, info.Size)
            },
        },
    }

    report, err := cleaner.CleanBackup("/path/to/backup", config)
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Deleted %d files, freed %d bytes in %v",
        report.DeletedFiles, report.DeletedSize, report.TotalDuration)
}
```

## Configuration Options

### Capacity Constraints (at least one required)

- `MaxSize`: Maximum total size in bytes
- `MaxUsagePercent`: Maximum disk usage percentage (0-100)
- `MinFreeSpace`: Minimum free space in bytes

### Optional Settings

- `TimeWindow`: Time interval for file aggregation (default: 1 minute)
- `RemoveEmptyDirs`: Whether to remove empty directories (default: true)
- `WorkerCount`: Number of parallel workers (default: runtime.NumCPU())

### Callbacks

Monitor the cleaning process with callbacks:

- `OnStart`: Called when cleaning starts
- `OnScanComplete`: Called after file scanning completes
- `OnDeleteStart`: Called before deletion begins
- `OnFileDeleted`: Called for each deleted file
- `OnDirDeleted`: Called for each deleted directory
- `OnComplete`: Called when cleaning completes
- `OnError`: Called on non-fatal errors

## How It Works

1. **Scans** the backup directory to catalog all files
2. **Calculates** how much space needs to be freed based on constraints
3. **Determines** a time threshold - files older than this will be deleted
4. **Deletes** files in parallel, starting with the oldest
5. **Cleans up** empty directories (if enabled)

## Testing

Run tests:
```bash
go test -v ./...
```

Run tests with coverage:
```bash
go test -v -cover ./...
```

## License

MIT License - see LICENSE file for details.