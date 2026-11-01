// Package cliui presents existing CLI commands without owning their work.
package cliui

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-isatty"
)

const Tagline = "Get your toolkit reet."

func Terminal(out io.Writer) bool {
	f, ok := out.(*os.File)
	return ok && isatty.IsTerminal(f.Fd())
}

func Interactive(in io.Reader, out io.Writer) bool {
	f, ok := in.(*os.File)
	return ok && isatty.IsTerminal(f.Fd()) && Terminal(out) && os.Getenv("TERM") != "dumb" && os.Getenv("CI") == ""
}

func Colour() bool {
	_, disabled := os.LookupEnv("NO_COLOR")
	return !disabled && os.Getenv("TERM") != "dumb"
}

func Motion() bool {
	return os.Getenv("REET_NO_ANIMATION") == "" && os.Getenv("REET_REDUCED_MOTION") == "" &&
		os.Getenv("REDUCE_MOTION") == "" && os.Getenv("PREFERS_REDUCED_MOTION") != "reduce" &&
		os.Getenv("CI") == "" && os.Getenv("TERM") != "dumb"
}

func darkBackground() bool {
	switch strings.ToLower(os.Getenv("REET_TUI_BACKGROUND")) {
	case "light":
		return false
	case "dark":
		return true
	}
	parts := strings.Split(os.Getenv("COLORFGBG"), ";")
	if n, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
		return n < 8
	}
	return true
}

func styled(text, role string, colour bool) string {
	if !colour || role == "" {
		return text
	}
	dark := darkBackground()
	value := "#FB923C"
	if !dark {
		value = "#C2410C"
	}
	s := lipgloss.NewStyle()
	switch role {
	case "bold":
		return s.Bold(true).Render(text)
	case "muted":
		value = "#9CA3AF"
		if !dark {
			value = "#6B7280"
		}
	case "fail":
		value = "#F87171"
		if !dark {
			value = "#B91C1C"
		}
	case "warn":
		value = "#FBBF24"
		if !dark {
			value = "#B45309"
		}
	}
	return s.Foreground(lipgloss.Color(value)).Render(text)
}

func Brand(colour bool) string {
	return styled("reet", "bold", colour) + styled(".sh", "accent", colour)
}

func Banner(out io.Writer) {
	if Terminal(out) && os.Getenv("TERM") != "dumb" && os.Getenv("CI") == "" {
		fmt.Fprintf(out, "\n  %s\n  %s\n\n", Brand(Colour()), Tagline)
	}
}

func fit(s string, width int) string { return ansi.Truncate(s, max(width, 0), "") }
func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(width-ansi.StringWidth(s), 0))
}

func plain(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, ansi.Strip(s))
}
