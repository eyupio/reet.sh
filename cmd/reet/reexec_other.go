//go:build !unix

package main

import "fmt"

func reexecBinary(_ string, _, _ []string) error {
	return fmt.Errorf("restart reet to load the new binary on this platform")
}
