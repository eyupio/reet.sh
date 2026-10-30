package tui

import "github.com/charmbracelet/lipgloss"

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

func newStyles(plain bool) styles {
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
	orange := lipgloss.AdaptiveColor{Light: "#C2410C", Dark: "#FB923C"}
	green := lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	amber := lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}
	red := lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	muted := lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
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
