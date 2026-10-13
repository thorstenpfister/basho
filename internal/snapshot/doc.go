// Package snapshot loads the embedded snapshot of finished tournaments and
// builds the in-memory bout index (SPEC §4.3, §5.8).
//
// Values it returns share memory with its caches: treat them as read-only.
package snapshot
