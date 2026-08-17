// Package domain holds the entities of the reservation system and the rules
// that govern them, and imports nothing from the rest of the project. Time is
// always passed in by the caller rather than read from time.Now, and failures
// are reported with the sentinel errors in errors.go.
package domain
