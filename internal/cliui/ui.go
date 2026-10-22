// Package cliui renders optional terminal presentation without touching JSON output.
package cliui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
)

func Terminal(out io.Writer) bool {
	f, ok := out.(*os.File)
	return ok && isatty.IsTerminal(f.Fd())
}
func colour(out io.Writer) bool {
	_, noColour := os.LookupEnv("NO_COLOR")
	return Terminal(out) && os.Getenv("TERM") != "dumb" && !noColour
}
func Banner(out io.Writer) {
	if !Terminal(out) {
		return
	}
	if colour(out) {
		fmt.Fprint(out, "\n  \x1b[38;2;251;146;60m✿\x1b[0m  \x1b[1mreet\x1b[0m\x1b[38;2;251;146;60m.sh\x1b[0m\n     Get your toolkit reet.\n\n")
	} else {
		fmt.Fprint(out, "\n  reet.sh\n  Get your toolkit reet.\n\n")
	}
}

// Start animates real work only. Stop joins the renderer before other output.
func Start(out io.Writer, label string) func() {
	if !colour(out) || os.Getenv("REET_NO_ANIMATION") != "" {
		return func() {}
	}
	done := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(finished)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		for i := 0; ; i++ {
			fmt.Fprintf(out, "\r\x1b[2K  \x1b[38;2;251;146;60m%s\x1b[0m %s", frames[i%len(frames)], label)
			select {
			case <-done:
				fmt.Fprint(out, "\r\x1b[2K")
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { once.Do(func() { close(done); <-finished }) }
}
