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
	dim           lipgloss.Style
	accent        lipgloss.Style
	cursorRow     lipgloss.Style
	panel         lipgloss.Style
	panelFocus    lipgloss.Style
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
			dim:           lipgloss.NewStyle(),
			accent:        lipgloss.NewStyle(),
			cursorRow:     lipgloss.NewStyle().Bold(true).Reverse(true),
			panel:         lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1),
			panelFocus:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1),
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
		dim:           lipgloss.NewStyle().Foreground(muted),
		accent:        lipgloss.NewStyle().Foreground(orange).Bold(true),
		cursorRow:     lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#FFFFFF"), lipgloss.Color("#111827"))).Background(orange),
		panel:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(muted).Padding(0, 1),
		panelFocus:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(orange).Padding(0, 1),
	}
}

// formTheme pins the form palette to the background we detected. The form's
// own detection is ignored because the interface never queries the terminal.
// Buttons get an unmistakable focus: the highlighted choice is a solid, bold
// block with arrows either side, and the other is plain text with no fill.
func formTheme(dark bool) huh.Theme {
	return huh.ThemeFunc(func(bool) *huh.Styles {
		t := huh.ThemeCharm(dark)
		ld := lipgloss.LightDark(dark)
		fg := ld(lipgloss.Color("#FFFFFF"), lipgloss.Color("#111827"))
		accent := ld(lipgloss.Color("#C2410C"), lipgloss.Color("#FB923C"))
		dim := ld(lipgloss.Color("#4B5563"), lipgloss.Color("#9CA3AF"))
		for _, state := range []*huh.FieldStyles{&t.Focused, &t.Blurred} {
			state.FocusedButton = lipgloss.NewStyle().Padding(0, 2).Margin(0, 1).Bold(true).Underline(true).
				Foreground(fg).Background(accent)
			state.BlurredButton = lipgloss.NewStyle().Padding(0, 2).Margin(0, 1).
				Foreground(dim)
		}
		t.Focused.Next = t.Focused.FocusedButton
		return t
	})
}
