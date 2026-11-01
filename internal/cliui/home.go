package cliui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type menuState struct {
	entries []Command
	cursor  int
	title   string
}

type homeModel struct {
	menuState
	stack              []menuState
	width, height      int
	colour             bool
	version            string
	searching, editing bool
	query, problem     string
	input              textinput.Model
	choice             []string
}

func newHome(version string) homeModel {
	input := textinput.New()
	input.CharLimit = 4096
	// Static even when motion is enabled: home has nothing to animate.
	input.SetStyles(textinput.Styles{})
	return homeModel{menuState: menuState{entries: HomeCommands(), title: "Home"},
		width: 80, height: 30, colour: Colour(), version: version, input: input}
}

// Choose returns only after Bubble Tea has restored the terminal. The caller
// can now run the real CLI with its prompts, passwords and subprocesses intact.
func Choose(ctx context.Context, in io.Reader, out io.Writer, version string) ([]string, error) {
	result, err := tea.NewProgram(newHome(version), tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return nil, err
	}
	return result.(homeModel).choice, nil
}

func (m homeModel) Init() tea.Cmd { return nil }

func (m homeModel) items() []Command {
	if m.query == "" {
		return m.entries
	}
	var all, result []Command
	for _, c := range Commands() {
		all = append(all, c)
		all = append(all, c.Children...)
	}
	for _, c := range all {
		if strings.Contains(strings.ToLower(c.Name+" "+c.Title+" "+c.Summary+" "+strings.Join(c.Args, " ")), strings.ToLower(m.query)) {
			result = append(result, c)
		}
	}
	return result
}

func (m homeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(msg.Width, 1), max(msg.Height, 1)
		m.input.SetWidth(max(m.width-6, 1))
		if m.editing {
			m.input.SetWidth(max(m.width-len(m.input.Prompt)-2, 1))
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			m.choice = nil
			return m, tea.Quit
		}
		if m.width < 24 || m.height < 8 {
			if key == "esc" || key == "q" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.editing {
			switch key {
			case "esc":
				m.editing = false
				m.input.Blur()
				m.problem = ""
				return m, nil
			case "enter":
				args, err := SplitArgs(m.input.Value())
				if err != nil {
					m.problem = err.Error()
					return m, nil
				}
				items := m.items()
				if len(items) > 0 {
					m.choice = append([]string{items[m.cursor].Args[0]}, args...)
					return m, tea.Quit
				}
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		if m.searching {
			switch key {
			case "esc":
				m.searching = false
				m.query = ""
				m.cursor = 0
				m.input.Blur()
				return m, nil
			case "up", "down", "enter": // Selection keys still work while typing.
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				m.query = plain(m.input.Value())
				m.cursor = 0
				return m, cmd
			}
		}
		items := m.items()
		switch key {
		case "q":
			return m, tea.Quit
		case "esc":
			if m.query != "" {
				m.query = ""
				m.cursor = 0
			} else if len(m.stack) > 0 {
				m.menuState = m.stack[len(m.stack)-1]
				m.stack = m.stack[:len(m.stack)-1]
			} else {
				return m, tea.Quit
			}
		case "/":
			m.searching = true
			m.input.Prompt = "/ "
			m.input.SetValue(m.query)
			return m, m.input.Focus()
		case "up", "k":
			m.cursor = max(m.cursor-1, 0)
		case "down", "j":
			m.cursor = min(m.cursor+1, max(len(items)-1, 0))
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = max(len(items)-1, 0)
		case "pgup":
			m.cursor = max(m.cursor-max(m.height-8, 1), 0)
		case "pgdown":
			m.cursor = min(m.cursor+max(m.height-8, 1), max(len(items)-1, 0))
		case "?":
			if len(items) > 0 {
				c := items[m.cursor]
				m.choice = append(append([]string{}, c.Args...), "--help")
				if c.Args[0] == "backup" {
					m.choice = []string{"backup", "--help"}
				}
				return m, tea.Quit
			}
		case "e":
			if len(items) > 0 {
				m.editing = true
				m.problem = ""
				m.input.Prompt = items[m.cursor].Args[0] + " "
				m.input.SetWidth(max(m.width-len(m.input.Prompt)-2, 1))
				m.input.SetValue(QuoteArgs(items[m.cursor].Args[1:]))
				return m, m.input.Focus()
			}
		case "enter":
			if len(items) > 0 {
				c := items[m.cursor]
				if len(c.Children) > 0 {
					previous := m.menuState
					if m.query != "" {
						previous.cursor = 0
					}
					m.stack = append(m.stack, previous)
					m.menuState = menuState{entries: c.Children, title: c.Title}
					m.searching = false
					m.query = ""
					m.input.Blur()
				} else {
					m.choice = append([]string{}, c.Args...)
					return m, tea.Quit
				}
			}
		}
	}
	return m, nil
}

func (m homeModel) View() tea.View {
	var rows []string
	if m.width < 24 || m.height < 8 {
		rows = []string{"reet.sh", "Resize to at least 24 × 8.", "Esc / q quit"}
	} else {
		rows = []string{pad(Brand(m.colour), max(m.width-len(m.version)-4, 7)) + styled(plain(m.version), "muted", m.colour), styled(Tagline, "muted", m.colour), ""}
		if m.editing {
			rows = append(rows, "Arguments · "+m.items()[m.cursor].Title, m.input.View())
			if m.problem != "" {
				rows = append(rows, styled(m.problem, "fail", m.colour))
			}
			if m.height >= 10 {
				rows = append(rows, "", "Quoted words only; no shell expansion.")
			}
			rows = append(rows, "Enter run   Esc cancel")
		} else {
			if m.searching {
				rows = append(rows, m.input.View())
			} else if m.query != "" {
				rows = append(rows, "/ "+plain(m.query))
			}
			items := m.items()
			var body []string
			selected, group := 0, ""
			for i, c := range items {
				g := c.Group
				if g == "" {
					g = m.title
				}
				if g != group {
					if group != "" && m.height >= 28 {
						body = append(body, "")
					}
					body = append(body, styled("  "+strings.ToUpper(g), "muted", m.colour))
					group = g
				}
				prefix := "  "
				role := ""
				if i == m.cursor {
					prefix = "› "
					role = "accent"
					selected = len(body)
				}
				line := prefix + pad(c.Title, 18)
				if m.width >= 64 {
					line += styled(c.Summary, "muted", m.colour)
				}
				body = append(body, styled(line, role, m.colour))
			}
			if len(items) == 0 {
				body = []string{"Nowt matches just yet.", "Try another search, or Esc to clear it."}
			}
			footer := []string{"", "↑/↓ j/k move   Enter open   / search", "e edit   ? help   Esc back   q quit"}
			if m.searching {
				footer = []string{"", "↑/↓ select   Enter open   Esc clear", "Type to search · Ctrl+C quit"}
			}
			if m.width < 40 {
				footer = []string{"", "↑/↓ j/k · Enter open", "e edit  ? help  / search", "Esc back  q quit"}
				if m.searching {
					footer = []string{"", "↑/↓ select · Enter open", "Esc clear · Ctrl+C quit"}
				}
			}
			if m.width < 64 && m.height >= 12 && len(items) > 0 {
				footer = append([]string{styled(items[m.cursor].Summary, "muted", m.colour)}, footer...)
			}
			available := max(m.height-len(rows)-len(footer), 1)
			start := max(0, selected-available+2)
			start = min(start, max(len(body)-available, 0))
			end := min(start+available, len(body))
			rows = append(rows, body[start:end]...)
			rows = append(rows, footer...)
		}
	}
	for i := range rows {
		rows[i] = fit(rows[i], m.width)
	}
	view := tea.NewView(strings.Join(rows[:min(len(rows), m.height)], "\n"))
	view.AltScreen = true
	return view
}

// SplitArgs accepts shell-style quoting, but performs no expansion or execution.
func SplitArgs(s string) ([]string, error) {
	var args []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, r := range s {
		if unicode.IsControl(r) {
			return nil, fmt.Errorf("arguments cannot contain control characters")
		}
		if escaped {
			word.WriteRune(r)
			escaped = false
			started = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			started = true
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			started = true
		default:
			if unicode.IsSpace(r) {
				if started {
					args = append(args, word.String())
					word.Reset()
					started = false
				}
			} else {
				word.WriteRune(r)
				started = true
			}
		}
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("close the quote or escape before running")
	}
	if started {
		args = append(args, word.String())
	}
	return args, nil
}

func QuoteArgs(args []string) string {
	var words []string
	for _, arg := range args {
		words = append(words, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
	}
	return strings.Join(words, " ")
}

type pauseModel struct {
	code, width int
	quit        bool
}

func (m pauseModel) Init() tea.Cmd { return nil }
func (m pauseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter", "esc":
			return m, tea.Quit
		case "q", "ctrl+c":
			m.quit = true
			return m, tea.Quit
		}
	}
	return m, nil
}
func (m pauseModel) View() tea.View {
	status := "Command finished."
	if m.code != 0 {
		status = fmt.Sprintf("Command failed (exit %d).", m.code)
	}
	return tea.NewView("\n" + fit(status, m.width) + "\n" + fit("Enter / Esc home   q quit", m.width) + "\n")
}

// Pause leaves the child's original stdout/stderr in normal terminal scrollback.
func Pause(ctx context.Context, in io.Reader, out io.Writer, code int) (bool, error) {
	result, err := tea.NewProgram(pauseModel{code: code, width: 80}, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return true, err
	}
	return result.(pauseModel).quit, nil
}
