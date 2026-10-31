package tui

import (
	"os"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

type styles struct {
	header        lipgloss.Style
	footer        lipgloss.Style
	title         lipgloss.Style
	item          lipgloss.Style
	muted         lipgloss.Style
	selected      lipgloss.Style
	selectedTitle lipgloss.Style
	pass          lipgloss.Style
	warn          lipgloss.Style
	fail          lipgloss.Style
	spinner       lipgloss.Style
}

// tuiDark chooses a palette without asking the terminal. The background query
// is what made Bubble Tea v1 slow to start on terminals that do not answer
// OSC 11; the terminal interface never sends it.
func tuiDark() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("REET_TUI_BACKGROUND"))) {
	case "light":
		return false
	case "dark":
		return true
	}
	if value := os.Getenv("COLORFGBG"); value != "" {
		parts := strings.Split(value, ";")
		if n, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			return n < 8
		}
	}
	return true
}

func newStyles(plain, dark bool) styles {
	if plain {
		return styles{
			header:        lipgloss.NewStyle().Bold(true),
			footer:        lipgloss.NewStyle(),
			title:         lipgloss.NewStyle().Bold(true),
			item:          lipgloss.NewStyle(),
			muted:         lipgloss.NewStyle(),
			selected:      lipgloss.NewStyle().Bold(true),
			selectedTitle: lipgloss.NewStyle().Bold(true),
			pass:          lipgloss.NewStyle(),
			warn:          lipgloss.NewStyle(),
			fail:          lipgloss.NewStyle(),
			spinner:       lipgloss.NewStyle(),
		}
	}
	ld := lipgloss.LightDark(dark)
	orange := ld(lipgloss.Color("#C2410C"), lipgloss.Color("#FB923C"))
	green := ld(lipgloss.Color("#15803D"), lipgloss.Color("#4ADE80"))
	amber := ld(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24"))
	red := ld(lipgloss.Color("#B91C1C"), lipgloss.Color("#F87171"))
	muted := ld(lipgloss.Color("#6B7280"), lipgloss.Color("#9CA3AF"))
	return styles{
		header:        lipgloss.NewStyle().Bold(true).Foreground(orange),
		footer:        lipgloss.NewStyle().Foreground(muted),
		title:         lipgloss.NewStyle().Bold(true),
		item:          lipgloss.NewStyle(),
		muted:         lipgloss.NewStyle().Foreground(muted),
		selected:      lipgloss.NewStyle().Bold(true).Foreground(orange),
		selectedTitle: lipgloss.NewStyle().Bold(true),
		pass:          lipgloss.NewStyle().Foreground(green),
		warn:          lipgloss.NewStyle().Foreground(amber),
		fail:          lipgloss.NewStyle().Foreground(red),
		spinner:       lipgloss.NewStyle().Foreground(orange),
	}
}

// formTheme pins the form palette to the background we detected. The form's
// own detection is ignored because the interface never queries the terminal.
func formTheme(dark bool) huh.Theme {
	return huh.ThemeFunc(func(bool) *huh.Styles { return huh.ThemeCharm(dark) })
}
