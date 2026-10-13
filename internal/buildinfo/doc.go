// Package buildinfo carries version information. Release builds set it via
// -ldflags; other builds fall back to the VCS data the Go toolchain stamps
// into the binary.
package buildinfo
