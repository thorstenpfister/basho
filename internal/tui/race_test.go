//go:build race

package tui_test

// raceDetector is set in race builds: the leak tests skip there, being
// single-threaded and ten times slower; `make check` runs them without it.
const raceDetector = true
