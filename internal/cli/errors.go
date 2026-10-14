package cli

import (
	"errors"

	"github.com/thorstenpfister/basho/internal/service"
)

// Exit codes (SPEC §11).
const (
	exitOK          = 0
	exitInternal    = 1
	exitUsage       = 2
	exitNotFound    = 3 // not found, or not available yet
	exitUnavailable = 4 // every source failed and nothing is cached
)

// exitError carries the exit code of a failure.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

// usage marks err as a usage error: exit code 2.
func usage(err error) error { return &exitError{code: exitUsage, err: err} }

// exitCode maps an error to the process exit code.
func exitCode(err error) int {
	var e *exitError
	switch {
	case err == nil:
		return exitOK
	case errors.As(err, &e):
		return e.code
	case service.NotFound(err):
		return exitNotFound
	case service.Unavailable(err):
		return exitUnavailable
	default:
		return exitInternal
	}
}
