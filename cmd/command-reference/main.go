// Command command-reference regenerates the catalog documentation.
package main

import (
	"github.com/eyupio/reet.sh/internal/cliui"
	"os"
)

func main() { cliui.Reference(os.Stdout) }
