# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go backup cleaner package designed to help manage backup file storage by automatically cleaning old files based on specified capacity limits. The detailed design is documented in DESIGN.md.

## Development Commands

### Go Module Initialization
```bash
go mod init github.com/[username]/go-backup-cleaner
```

### Testing
```bash
# Run all tests
go test -v ./...

# Run tests with coverage
go test -v -cover ./...

# Run a specific test
go test -v -run TestFunctionName ./...
```

### Building
```bash
# Build the package
go build ./...

# Check for compilation errors
go vet ./...
```

### Linting and Formatting
```bash
# Format code
go fmt ./...

# Run static analysis
go vet ./...

# Install and run golangci-lint (recommended)
golangci-lint run
```

## Architecture Overview

The package follows a modular design with clear separation of concerns:

1. **Core Components**:
   - `cleaner.go` - Main cleaning logic and orchestration
   - `config.go` - Configuration structures (CleaningConfig)
   - `report.go` - Reporting structures (CleaningReport)
   - `callback.go` - Callback system for progress monitoring
   - `disk.go` - Disk information interface (DiskInfoProvider)
   - `scanner.go` - File scanning with parallel processing
   - `deleter.go` - File deletion with parallel processing

2. **Key Design Decisions**:
   - **Parallel Processing**: File scanning and deletion use worker pools
   - **Memory Efficiency**: Time-windowed aggregation to handle large file sets
   - **Dependency Injection**: DiskInfoProvider interface for testing
   - **Callback System**: Non-blocking progress notifications
   - **Cross-platform**: OS-specific implementations for disk operations

3. **Processing Flow**:
   - Calculate deletion target based on capacity constraints
   - Scan files in parallel, aggregating by time windows
   - Determine time threshold for deletion
   - Delete files older than threshold in parallel
   - Clean up empty directories (optional)

4. **Concurrency Control**:
   - Use `Concurrency` field (defaults to CPU count)
   - Limited by `MaxConcurrency` (defaults to 4)
   - Actual workers = min(Concurrency, MaxConcurrency)
   - MaxConcurrency=4 based on benchmarks showing diminishing returns

## Important Design Constraints

1. **Error Handling**: Individual file deletion errors don't stop the process
2. **Atomic Operations**: Either all capacity checks pass or operation fails
3. **Block Size Awareness**: Actual disk space freed considers file system block size
4. **Thread Safety**: Concurrent operations use appropriate synchronization

## Testing Strategy

The design includes comprehensive test scenarios:
- Unit tests with mocked DiskInfoProvider
- Multi-platform CI/CD via GitHub Actions
- Edge cases: permissions, symbolic links, empty directories
- Performance tests with large file sets

## Dependencies

Currently planned dependency:
- `github.com/shirou/gopsutil/v3/disk` - For disk usage information

Alternative: Pure standard library implementation if gopsutil is not desired.