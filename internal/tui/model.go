// Package tui renders reet's terminal interface with Bubble Tea.
package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/eyupio/reet.sh/internal/backup"
	"github.com/eyupio/reet.sh/internal/health"
	"github.com/eyupio/reet.sh/internal/lifecycle"
	"github.com/eyupio/reet.sh/internal/scan"
	"github.com/eyupio/reet.sh/internal/setup"
	"github.com/eyupio/reet.sh/internal/terminalcontrol"
)

// ScreenID names one terminal screen.
type ScreenID string

const (
	ScreenHome     ScreenID = "home"
	ScreenDoctor   ScreenID = "doctor"
	ScreenHealth   ScreenID = "healthcheck"
	ScreenScan     ScreenID = "scan"
	ScreenSync     ScreenID = "sync"
	ScreenBackup   ScreenID = "backup"
	ScreenSetup    ScreenID = "setup"
	ScreenUpgrade  ScreenID = "upgrade"
	ScreenVersion  ScreenID = "version"
	ScreenTerminal ScreenID = "terminal"
	ScreenAgent    ScreenID = "agent"
)

// Options is what the command passes into the terminal interface. The doctor,
// health, terminal and agent fields mirror the flags of their text commands.
type Options struct {
	Version string
	Commit  string
	Start   ScreenID

	DoctorStateDir  string
	DoctorServer    string
	DoctorTokenFile string

	HealthURL   string
	HealthLocal bool

	TerminalDataDir string
	AgentConfig     string

	ScanServer    string
	ScanToken     string
	ScanTokenFile string
	ScanEnv       string
	ScanDirs      string
	ScanPrune     bool
	ScanReconcile bool
	ScanDryRun    bool

	SyncServer    string
	SyncToken     string
	SyncTokenFile string
	SyncTarget    string
	SyncEnv       string
	SyncConfig    string
	SyncDir       string
	SyncApply     bool

	BackupDataDir string

	SetupDir       string
	SetupServer    string
	SetupToken     string
	SetupTokenFile string

	UpgradeVersion  string
	UpgradeStateDir string

	In      io.Reader
	Out     io.Writer
	NoColor bool
}

// Run starts the terminal interface and blocks until the user quits.
func Run(ctx context.Context, opts Options) error {
	if opts.Start == "" {
		opts.Start = ScreenHome
	}
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	programOpts := []tea.ProgramOption{tea.WithContext(ctx), tea.WithAltScreen(), tea.WithOutput(opts.Out)}
	if opts.In != nil {
		programOpts = append(programOpts, tea.WithInput(opts.In))
	}
	_, err := tea.NewProgram(newModel(opts), programOpts...).Run()
	return err
}

type homeItem struct {
	id          ScreenID
	title       string
	description string
}

var homeItems = []homeItem{
	{ScreenDoctor, "Doctor", "Check this installation"},
	{ScreenHealth, "Healthcheck", "Check a server's /healthz"},
	{ScreenScan, "Scan", "Report installed skills to a controller"},
	{ScreenSync, "Sync", "Preview and apply a saved target"},
	{ScreenBackup, "Backups", "List, take and restore instance copies"},
	{ScreenSetup, "Setups", "Create, push and pull setup profiles"},
	{ScreenUpgrade, "Upgrade", "Check, verify and install a release"},
	{ScreenVersion, "Version", "Build and runtime details"},
	{ScreenTerminal, "Terminal status", "Controller terminal availability"},
	{ScreenAgent, "Agent status", "This machine's enrollment and daemon"},
}

type resultMsg struct {
	page  ScreenID
	value any
	err   error
}

type model struct {
	opts   Options
	styles styles

	width, height int
	page          ScreenID
	cursor        int
	loading       bool
	err           error

	viewport viewport.Model
	spinner  spinner.Model

	doctor   lifecycle.Report
	health   health.Result
	term     terminalcontrol.Status
	agent    scan.AgentStatus
	config   string
	enrolled bool

	scanForm   *huh.Form
	scanValues *scanFormValues
	scanReport scan.Report
	scanSet    bool

	syncForm    *huh.Form
	syncValues  *syncFormValues
	syncReport  scan.SyncReport
	syncApplied bool
	syncSet     bool

	backupForm          *huh.Form
	backupFormKind      string
	backupRestoreID     string
	backupNewValues     *newBackupValues
	backupRestoreValues *restoreFormValues
	backupItems         []backup.Item
	backupCursor        int
	backupMessage       string
	backupSet           bool

	setupForm       *huh.Form
	setupFormKind   string
	setupNewValues  *setupNewValues
	setupPushValues *setupPushValues
	setupPullValues *setupPullValues
	setupProfiles   setup.SetupProfileList
	setupSnapshots  setup.SetupSnapshotList
	setupCursor     int
	setupMessage    string
	setupSet        bool

	upgradeForm   *huh.Form
	upgradeValues *upgradeFormValues
	upgradeOutput string
	upgradeDone   bool
}

func newModel(opts Options) model {
	if opts.Start == "" {
		opts.Start = ScreenHome
	}
	sp := spinner.New()
	sp.Spinner = spinner.Line
	sp.Style = newStyles(opts.NoColor).spinner
	m := model{
		opts:     opts,
		styles:   newStyles(opts.NoColor),
		page:     opts.Start,
		loading:  opts.Start != ScreenHome && opts.Start != ScreenVersion && opts.Start != ScreenScan,
		viewport: viewport.New(80, 20),
		spinner:  sp,
	}
	if opts.Start == ScreenScan {
		m.scanValues = defaultScanValues(opts)
		m.scanForm = buildScanForm(m.scanValues)
	}
	if opts.Start == ScreenSync {
		m.syncValues = defaultSyncValues(opts)
		m.syncForm = buildSyncForm(m.syncValues)
	}
	if opts.Start == ScreenUpgrade {
		m.upgradeValues = &upgradeFormValues{Release: defaultUpgradeRelease(opts)}
		m.upgradeForm = buildUpgradeForm(m.upgradeValues, opts.Version+" ("+shortCommit(opts.Commit)+")")
	}
	return m
}

func (m model) Init() tea.Cmd {
	if form := m.activeForm(); form != nil {
		return form.Init()
	}
	if m.loading {
		return tea.Batch(m.spinner.Tick, m.load(m.page))
	}
	return nil
}

func (m model) load(page ScreenID) tea.Cmd {
	opts := m.opts
	return func() tea.Msg {
		switch page {
		case ScreenDoctor:
			report, err := loadDoctor(opts)
			return resultMsg{page: page, value: report, err: err}
		case ScreenHealth:
			result, err := loadHealth(opts)
			return resultMsg{page: page, value: result, err: err}
		case ScreenTerminal:
			status, err := loadTerminal(opts)
			return resultMsg{page: page, value: status, err: err}
		case ScreenAgent:
			value, err := loadAgent(opts)
			return resultMsg{page: page, value: value, err: err}
		case ScreenBackup:
			value, err := loadBackups(opts)
			return resultMsg{page: page, value: value, err: err}
		case ScreenSetup:
			value, err := loadSetup(opts)
			return resultMsg{page: page, value: value, err: err}
		}
		return resultMsg{page: page, err: fmt.Errorf("unknown screen %s", page)}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		if form := m.activeForm(); form != nil {
			updated, cmd := form.Update(msg)
			m.setForm(updated.(*huh.Form))
			m.setContent()
			return m, cmd
		}
		return m, nil
	}
	if form := m.activeForm(); form != nil {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				if m.page == ScreenBackup {
					m.backupForm = nil
					m.setContent()
					return m, nil
				}
				if m.page == ScreenSetup {
					m.setupForm = nil
					m.setContent()
					return m, nil
				}
				return m.open(ScreenHome)
			}
		}
		updated, cmd := form.Update(msg)
		form = updated.(*huh.Form)
		m.setForm(form)
		switch form.State {
		case huh.StateCompleted:
			switch m.page {
			case ScreenScan:
				m.scanForm = nil
				return m.startScan()
			case ScreenSync:
				m.syncForm = nil
				return m.startSync(false)
			case ScreenBackup:
				return m.finishBackupForm()
			case ScreenSetup:
				return m.finishSetupForm()
			case ScreenUpgrade:
				return m.finishUpgradeForm()
			}
		case huh.StateAborted:
			return m.open(ScreenHome)
		}
		m.setContent()
		return m, cmd
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.loading {
			m.setContent()
			return m, cmd
		}
		return m, nil
	case resultMsg:
		if msg.page != m.page {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		m.apply(msg)
		m.setContent()
		if msg.page == ScreenSync && m.opts.SyncApply && !m.syncApplied && m.err == nil && len(m.syncReport.Plan.Actions) > 0 {
			m.opts.SyncApply = false
			return m.startSync(true)
		}
		return m, nil
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.page == ScreenHome {
		switch msg.String() {
		case "q", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(homeItems)-1 {
				m.cursor++
			}
		case "enter":
			return m.open(homeItems[m.cursor].id)
		}
		return m, nil
	}
	if m.page == ScreenScan {
		switch msg.String() {
		case "r":
			return m.startScan()
		case "e":
			return m.open(ScreenScan)
		}
	}
	if m.page == ScreenSync {
		switch msg.String() {
		case "a":
			if m.syncSet && !m.syncApplied && m.err == nil && len(m.syncReport.Plan.Actions) > 0 {
				return m.startSync(true)
			}
		case "r":
			return m.startSync(false)
		case "e":
			return m.open(ScreenSync)
		}
	}
	if m.page == ScreenUpgrade {
		switch msg.String() {
		case "r":
			return m.open(ScreenUpgrade)
		}
	}
	if m.page == ScreenSetup {
		switch msg.String() {
		case "n":
			newest := ""
			if len(m.setupSnapshots.Snapshots) > 0 {
				newest = m.setupSnapshots.Snapshots[0].ID
			}
			m.setupNewValues = &setupNewValues{}
			m.setupFormKind = "new"
			m.setupForm = buildNewSetupForm(m.setupNewValues, newest)
			m.resize()
			m.setContent()
			return m, m.setupForm.Init()
		case "p":
			if len(m.setupProfiles.Profiles) > 0 {
				name := m.setupProfiles.Profiles[m.setupCursor].Name
				m.setupPushValues = defaultSetupPushValues(m.opts, name)
				m.setupFormKind = "push"
				m.setupForm = buildPushSetupForm(m.setupPushValues)
				m.resize()
				m.setContent()
				return m, m.setupForm.Init()
			}
		case "u":
			m.setupPullValues = defaultSetupPullValues(m.opts)
			m.setupFormKind = "pull"
			m.setupForm = buildPullSetupForm(m.setupPullValues)
			m.resize()
			m.setContent()
			return m, m.setupForm.Init()
		case "r":
			m.err = nil
			m.loading = true
			m.setContent()
			return m, tea.Batch(m.spinner.Tick, m.load(ScreenSetup))
		case "up", "k":
			if m.setupCursor > 0 {
				m.setupCursor--
			}
		case "down", "j":
			if m.setupCursor < len(m.setupProfiles.Profiles)-1 {
				m.setupCursor++
			}
		}
	}
	if m.page == ScreenBackup {
		switch msg.String() {
		case "n":
			m.backupNewValues = &newBackupValues{}
			m.backupFormKind = "new"
			m.backupForm = buildNewBackupForm(m.backupNewValues)
			m.resize()
			m.setContent()
			return m, m.backupForm.Init()
		case "x":
			if len(m.backupItems) > 0 {
				m.backupRestoreID = m.backupItems[m.backupCursor].ID
				m.backupRestoreValues = &restoreFormValues{}
				m.backupFormKind = "restore"
				m.backupForm = buildRestoreForm(m.backupRestoreID, m.backupRestoreValues)
				m.resize()
				m.setContent()
				return m, m.backupForm.Init()
			}
		case "r":
			m.err = nil
			m.loading = true
			m.setContent()
			return m, tea.Batch(m.spinner.Tick, m.load(ScreenBackup))
		case "up", "k":
			if m.backupCursor > 0 {
				m.backupCursor--
			}
		case "down", "j":
			if m.backupCursor < len(m.backupItems)-1 {
				m.backupCursor++
			}
		}
	}
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		return m.open(ScreenHome)
	case "r":
		m.err = nil
		m.loading = m.page != ScreenVersion
		m.setContent()
		if !m.loading {
			return m, nil
		}
		return m, tea.Batch(m.spinner.Tick, m.load(m.page))
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m model) open(page ScreenID) (tea.Model, tea.Cmd) {
	m.page = page
	m.cursor = 0
	m.err = nil
	m.scanForm = nil
	m.syncForm = nil
	m.backupForm = nil
	m.setupForm = nil
	m.upgradeForm = nil
	if page == ScreenScan {
		if m.scanValues == nil {
			m.scanValues = defaultScanValues(m.opts)
		}
		m.scanForm = buildScanForm(m.scanValues)
		m.scanSet = false
		m.loading = false
		m.resize()
		m.setContent()
		return m, m.scanForm.Init()
	}
	if page == ScreenSync {
		if m.syncValues == nil {
			m.syncValues = defaultSyncValues(m.opts)
		}
		m.syncForm = buildSyncForm(m.syncValues)
		m.syncSet = false
		m.syncApplied = false
		m.loading = false
		m.resize()
		m.setContent()
		return m, m.syncForm.Init()
	}
	if page == ScreenUpgrade {
		m.upgradeValues = &upgradeFormValues{Release: defaultUpgradeRelease(m.opts)}
		m.upgradeForm = buildUpgradeForm(m.upgradeValues, m.opts.Version+" ("+shortCommit(m.opts.Commit)+")")
		m.upgradeOutput = ""
		m.upgradeDone = false
		m.loading = false
		m.resize()
		m.setContent()
		return m, m.upgradeForm.Init()
	}
	m.loading = page != ScreenHome && page != ScreenVersion
	m.setContent()
	if !m.loading {
		return m, nil
	}
	return m, tea.Batch(m.spinner.Tick, m.load(page))
}

func (m *model) apply(msg resultMsg) {
	m.err = msg.err
	switch msg.page {
	case ScreenDoctor:
		if report, ok := msg.value.(lifecycle.Report); ok {
			m.doctor = report
		}
	case ScreenHealth:
		if result, ok := msg.value.(health.Result); ok {
			m.health = result
		}
	case ScreenTerminal:
		if status, ok := msg.value.(terminalcontrol.Status); ok {
			m.term = status
		}
	case ScreenAgent:
		if value, ok := msg.value.(agentValue); ok {
			m.agent = value.status
			m.config = value.config
			m.enrolled = value.enrolled
		}
	case ScreenScan:
		if report, ok := msg.value.(scan.Report); ok {
			m.scanReport = report
			m.scanSet = true
		}
	case ScreenSync:
		if value, ok := msg.value.(syncOutcome); ok {
			m.syncReport = value.report
			m.syncApplied = value.applied
			m.syncSet = value.report.PlanSet
		}
	case ScreenBackup:
		if value, ok := msg.value.(backupOutcome); ok {
			m.backupItems = value.items
			m.backupMessage = value.message
			if value.listSet {
				m.backupSet = true
			}
			if m.backupCursor >= len(m.backupItems) {
				m.backupCursor = len(m.backupItems) - 1
				if m.backupCursor < 0 {
					m.backupCursor = 0
				}
			}
		}
	case ScreenSetup:
		if value, ok := msg.value.(setupOutcome); ok {
			m.setupProfiles = value.profiles
			m.setupSnapshots = value.snapshots
			m.setupMessage = value.message
			if value.listSet {
				m.setupSet = true
			}
			if m.setupCursor >= len(m.setupProfiles.Profiles) {
				m.setupCursor = len(m.setupProfiles.Profiles) - 1
				if m.setupCursor < 0 {
					m.setupCursor = 0
				}
			}
		}
	case ScreenUpgrade:
		if value, ok := msg.value.(upgradeOutcome); ok {
			m.upgradeOutput = value.output
			m.upgradeDone = msg.err == nil
		}
	}
}

func (m model) activeForm() *huh.Form {
	if m.loading {
		return nil
	}
	switch m.page {
	case ScreenScan:
		return m.scanForm
	case ScreenSync:
		return m.syncForm
	case ScreenBackup:
		return m.backupForm
	case ScreenSetup:
		return m.setupForm
	case ScreenUpgrade:
		return m.upgradeForm
	}
	return nil
}

func (m model) formActive() bool { return m.activeForm() != nil }

func (m *model) setForm(form *huh.Form) {
	switch m.page {
	case ScreenScan:
		m.scanForm = form
	case ScreenSync:
		m.syncForm = form
	case ScreenBackup:
		m.backupForm = form
	case ScreenSetup:
		m.setupForm = form
	case ScreenUpgrade:
		m.upgradeForm = form
	}
}

func (m model) startScan() (tea.Model, tea.Cmd) {
	values := m.scanValues
	if values == nil {
		values = defaultScanValues(m.opts)
		m.scanValues = values
	}
	opts := m.opts
	m.scanForm = nil
	m.scanSet = false
	m.err = nil
	m.loading = true
	m.setContent()
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		report, err := scan.RunReport(scan.ReportOptions{
			Server:      strings.TrimSpace(values.Server),
			Token:       values.Token,
			TokenFile:   opts.ScanTokenFile,
			Host:        strings.TrimSpace(values.Host),
			Dirs:        scanDirs(values.Dirs),
			Prune:       values.Prune,
			Reconcile:   values.Reconcile,
			DryRun:      values.DryRun,
			Version:     opts.Version,
			Interactive: false,
			In:          strings.NewReader(""),
			Out:         io.Discard,
		})
		return resultMsg{page: ScreenScan, value: report, err: err}
	})
}

func (m *model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	body := m.height - 3
	if body < 3 {
		body = 3
	}
	m.viewport.Width = m.width - 2
	if m.viewport.Width < 10 {
		m.viewport.Width = 10
	}
	m.viewport.Height = body
	width := m.width - 4
	if width < 20 {
		width = 20
	}
	height := m.height - 5
	if height < 5 {
		height = 5
	}
	for _, form := range []*huh.Form{m.scanForm, m.syncForm, m.backupForm, m.setupForm, m.upgradeForm} {
		if form != nil {
			form.WithWidth(width)
			form.WithHeight(height)
		}
	}
	m.setContent()
}

func (m *model) setContent() {
	if m.page == ScreenHome {
		return
	}
	m.viewport.SetContent(m.renderPage())
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(), m.renderBody(), m.renderFooter())
}

func (m model) renderBody() string {
	if m.page == ScreenHome {
		return m.renderHome()
	}
	return m.viewport.View()
}

func (m model) renderHeader() string {
	title := "reet"
	if m.page != ScreenHome {
		title = "reet · " + pageTitle(m.page)
	}
	return m.styles.header.Width(m.width).Render("  ✿  " + title)
}

func (m model) renderFooter() string {
	keys := "↑/↓ move · enter open · q quit"
	if m.page != ScreenHome {
		keys = "↑/↓ scroll · r refresh · esc back · q quit"
	}
	if m.page == ScreenScan {
		if m.formActive() {
			keys = "enter next/submit · esc cancel · ctrl+c quit"
		} else if m.loading {
			keys = "scanning… · ctrl+c quit"
		} else {
			keys = "r run again · e edit · esc back · q quit"
		}
	}
	if m.page == ScreenSync {
		switch {
		case m.formActive():
			keys = "enter next/submit · esc cancel · ctrl+c quit"
		case m.loading:
			keys = "syncing… · ctrl+c quit"
		case m.syncApplied:
			keys = "r preview again · e edit · esc back · q quit"
		case len(m.syncReport.Plan.Actions) > 0:
			keys = "a apply · r preview again · e edit · esc back"
		default:
			keys = "r preview again · e edit · esc back · q quit"
		}
	}
	if m.page == ScreenBackup {
		switch {
		case m.formActive():
			keys = "enter next/submit · esc cancel · ctrl+c quit"
		case m.loading:
			keys = "working… · ctrl+c quit"
		default:
			keys = "n new · x restore · ↑/↓ select · r refresh · esc back · q quit"
		}
	}
	if m.page == ScreenSetup {
		switch {
		case m.formActive():
			keys = "enter next/submit · esc cancel · ctrl+c quit"
		case m.loading:
			keys = "working… · ctrl+c quit"
		default:
			keys = "n new · p push · u pull · ↑/↓ select · r refresh · esc back · q quit"
		}
	}
	if m.page == ScreenUpgrade {
		switch {
		case m.formActive():
			keys = "enter next/submit · esc cancel · ctrl+c quit"
		case m.loading:
			keys = "upgrading… · ctrl+c quit"
		default:
			keys = "r run again · esc back · q quit"
		}
	}
	if m.loading && m.page != ScreenScan && m.page != ScreenSync && m.page != ScreenBackup && m.page != ScreenSetup && m.page != ScreenUpgrade {
		keys = "loading… · q quit"
	}
	return m.styles.footer.Width(m.width).Render("  " + keys)
}

func (m model) renderHome() string {
	var b strings.Builder
	b.WriteString("\n  " + m.styles.muted.Render("Get your toolkit reet.") + "\n\n")
	b.WriteString("  " + m.styles.muted.Render(fmt.Sprintf("reet %s (%s)", m.opts.Version, shortCommit(m.opts.Commit))) + "\n\n")
	for i, item := range homeItems {
		title := m.styles.item.Render(fmt.Sprintf("%-16s", item.title))
		marker := "  "
		if i == m.cursor {
			marker = m.styles.selected.Render("› ")
			title = m.styles.selectedTitle.Render(fmt.Sprintf("%-16s", item.title))
		}
		b.WriteString("  " + marker + title + m.styles.muted.Render(item.description) + "\n")
	}
	return b.String()
}

func (m model) renderPage() string {
	if form := m.activeForm(); form != nil {
		return form.View()
	}
	if m.loading {
		return "\n  " + m.spinner.View() + " " + m.styles.muted.Render(pageLoading(m.page)) + "\n"
	}
	if m.err != nil && m.page != ScreenHealth && !(m.page == ScreenScan && m.scanSet) && !(m.page == ScreenSync && m.syncSet) && !(m.page == ScreenBackup && m.backupSet) && !(m.page == ScreenSetup && m.setupSet) && !(m.page == ScreenUpgrade && (m.upgradeOutput != "" || m.upgradeDone)) {
		return "\n  " + m.styles.fail.Render("Error: "+m.err.Error()) + "\n\n  " + m.styles.muted.Render("Press r to try again, or esc for the menu.") + "\n"
	}
	switch m.page {
	case ScreenScan:
		return m.renderScan()
	case ScreenSync:
		return m.renderSync()
	case ScreenBackup:
		return m.renderBackup()
	case ScreenSetup:
		return m.renderSetup()
	case ScreenUpgrade:
		return m.renderUpgrade()
	case ScreenDoctor:
		return m.renderDoctor()
	case ScreenHealth:
		return m.renderHealth()
	case ScreenVersion:
		return m.renderVersion()
	case ScreenTerminal:
		return m.renderTerminal()
	case ScreenAgent:
		return m.renderAgent()
	}
	return ""
}

func pageTitle(page ScreenID) string {
	switch page {
	case ScreenDoctor:
		return "Doctor"
	case ScreenHealth:
		return "Healthcheck"
	case ScreenScan:
		return "Scan"
	case ScreenSync:
		return "Sync"
	case ScreenBackup:
		return "Backups"
	case ScreenSetup:
		return "Setups"
	case ScreenUpgrade:
		return "Upgrade"
	case ScreenVersion:
		return "Version"
	case ScreenTerminal:
		return "Terminal status"
	case ScreenAgent:
		return "Agent status"
	}
	return "Home"
}

func pageLoading(page ScreenID) string {
	switch page {
	case ScreenDoctor:
		return "Running doctor checks…"
	case ScreenHealth:
		return "Checking server health…"
	case ScreenScan:
		return "Scanning installed skills…"
	case ScreenSync:
		return "Reading the sync target…"
	case ScreenBackup:
		return "Reading local backups…"
	case ScreenSetup:
		return "Reading setup profiles…"
	case ScreenUpgrade:
		return "Upgrading…"
	case ScreenTerminal:
		return "Reading terminal status…"
	case ScreenAgent:
		return "Reading agent status…"
	}
	return "Loading…"
}

func shortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
