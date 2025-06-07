# go-backup-cleaner

English | [日本語](README.ja.md)

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

- `TimeWindow`: Time interval for file aggregation (default: 5 minutes)
- `RemoveEmptyDirs`: Whether to remove empty directories (default: true)
- `Concurrency`: Level of concurrency (default: runtime.NumCPU())
- `MaxConcurrency`: Maximum level of concurrency (default: 4)

#### Concurrency Settings

The package uses parallel processing for scanning and deleting files. You can control the level of parallelism:

- `Concurrency`: Specifies the desired level of concurrency. If set to 0, it defaults to the number of CPU cores.
- `MaxConcurrency`: Limits the maximum level of concurrency. Defaults to 4.
- The actual concurrency can be obtained via `config.ActualWorkerCount()`, which returns `min(Concurrency, MaxConcurrency)`.

The reason for limiting `MaxConcurrency` to 4:
- Benchmarks show diminishing returns beyond 4 parallel workers
- Disk I/O becomes the bottleneck, making excessive parallelization ineffective
- This value provides optimal resource utilization for most systems

#### Block Size

The cleaner considers "block size" when calculating disk space. Block size refers to the minimum allocation unit used by the file system. When a file is stored on disk, it occupies space in multiples of the block size, even if the actual file size is smaller. For example:
- A 1KB file on a file system with 4KB blocks will actually use 4KB of disk space
- A 5KB file on the same system will use 8KB (2 blocks)

This package accurately tracks both the file size and the actual disk space that will be freed when files are deleted, ensuring precise capacity management.

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

### Special Case: MaxSize with No Disk Usage

When disk usage information is not available (e.g., due to permissions or OS limitations), the cleaner can still operate if `MaxSize` is specified. In this mode, it will delete old files until the total size is under the specified limit. This is useful for:

- Environments with restricted disk access
- Network storage where disk usage APIs are not available
- Simplified quota-based cleanup

Note: `MaxUsagePercent` and `MinFreeSpace` require disk usage information and cannot be used when disk usage is unavailable.

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