package gobackupcleaner

import "errors"

var (
	// ErrNoCapacitySpecified is returned when no capacity limit is specified
	ErrNoCapacitySpecified = errors.New("no capacity limit specified")

	// ErrInvalidConfig is returned when the configuration is invalid
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrDirectoryNotFound is returned when the target directory is not found
	ErrDirectoryNotFound = errors.New("directory not found")

	// ErrInsufficientSpace is returned when enough space cannot be freed
	ErrInsufficientSpace = errors.New("cannot free enough space")
)