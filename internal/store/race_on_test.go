//go:build race

package store

// raceEnabled: the race detector slows the database work about tenfold, so
// the timing test uses fewer accounts.
const raceEnabled = true
