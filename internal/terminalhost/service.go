package terminalhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var prefixPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,20}$`)

type ServiceOptions struct {
	Prefix           string
	Binary           string
	ConfigPath       string
	Layout           Layout
	UnitDir          string
	Env              []string
	AllowMissingTmux bool
}

type ServicePlan struct {
	Prefix        string
	Binary        string
	ConfigPath    string
	Layout        Layout
	TmuxPath      string
	Env           []string
	UnitDir       string
	AgentName     string
	TerminalName  string
	AgentLabel    string
	TerminalLabel string
	AgentPath     string
	TerminalPath  string
}

type ServiceStatus struct {
	Installed        bool
	Active           bool
	ManagerAvailable bool
	AgentPath        string
	TerminalPath     string
}

// PlanServices prepares independent agent and tmux service definitions. It
// performs no writes and starts no service.
func PlanServices(ctx context.Context, options ServiceOptions) (ServicePlan, error) {
	prefix := options.Prefix
	if prefix == "" {
		prefix = "reet"
	}
	if !prefixPattern.MatchString(prefix) {
		return ServicePlan{}, errors.New("invalid service name prefix")
	}
	binary := options.Binary
	if binary == "" {
		executable, err := os.Executable()
		if err != nil {
			return ServicePlan{}, err
		}
		binary = executable
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		return ServicePlan{}, errors.New("service binary path is invalid")
	}
	if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return ServicePlan{}, errors.New("service binary must be an executable file")
	}
	configPath, err := filepath.Abs(options.ConfigPath)
	if err != nil {
		return ServicePlan{}, errors.New("enrollment configuration path is invalid")
	}
	if err := CheckEnrollmentConfig(configPath); err != nil {
		return ServicePlan{}, err
	}
	if options.Layout.Dir == "" {
		options.Layout, err = NewLayout(configPath)
		if err != nil {
			return ServicePlan{}, err
		}
	}
	tmux, err := FindTmux(ctx)
	if err != nil && !options.AllowMissingTmux {
		return ServicePlan{}, err
	}
	tmuxPath := tmux.Path
	if tmuxPath == "" {
		tmuxPath = "tmux"
	}
	unitDir := options.UnitDir
	if unitDir == "" {
		unitDir, err = defaultUnitDir()
		if err != nil {
			return ServicePlan{}, err
		}
	}
	env := options.Env
	if env == nil {
		env = safeEnvironment()
	}
	env = validateEnvironment(env)
	plan := ServicePlan{
		Prefix: prefix, Binary: binary, ConfigPath: configPath, Layout: options.Layout,
		TmuxPath: tmuxPath, Env: env, UnitDir: unitDir,
	}
	if runtime.GOOS == "darwin" {
		plan.AgentLabel = labelFor(prefix, "agent")
		plan.TerminalLabel = labelFor(prefix, "terminal")
		plan.AgentName = plan.AgentLabel + ".plist"
		plan.TerminalName = plan.TerminalLabel + ".plist"
	} else {
		plan.AgentName = prefix + "-agent.service"
		plan.TerminalName = prefix + "-terminal.service"
		plan.AgentLabel = plan.AgentName
		plan.TerminalLabel = plan.TerminalName
	}
	plan.AgentPath = filepath.Join(unitDir, plan.AgentName)
	plan.TerminalPath = filepath.Join(unitDir, plan.TerminalName)
	return plan, nil
}

func labelFor(prefix, kind string) string {
	if prefix == "reet" {
		return "sh.reet." + kind
	}
	return "sh.reet." + prefix + "." + kind
}

func defaultUnitDir() (string, error) {
	if os.Geteuid() == 0 {
		if runtime.GOOS == "darwin" {
			return "/Library/LaunchDaemons", nil
		}
		return "/etc/systemd/system", nil
	}
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "LaunchAgents"), nil
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "systemd", "user"), nil
}

func (p ServicePlan) AgentText() string {
	if runtime.GOOS == "darwin" {
		return p.plist(p.AgentLabel, p.agentArgs(), filepath.Join(p.Layout.Logs, "agent.log"))
	}
	return p.systemdUnit("reet agent (private monitoring and sync)", "After=network-online.target\nWants=network-online.target", p.agentArgs())
}

func (p ServicePlan) TerminalText() string {
	if runtime.GOOS == "darwin" {
		return p.plist(p.TerminalLabel, p.terminalArgs(), filepath.Join(p.Layout.Logs, "terminal.log"))
	}
	return p.systemdUnit("reet terminal tmux supervisor", "After=network-online.target\nWants=network-online.target", p.terminalArgs())
}

func (p ServicePlan) agentArgs() []string {
	return []string{p.Binary, "agent", "run", "--foreground", "--supervised", "--service-prefix", p.Prefix, "--config", p.ConfigPath}
}

func (p ServicePlan) terminalArgs() []string {
	return []string{p.TmuxPath, "-D", "-S", p.Layout.Socket, "-f", p.Layout.Config}
}

func (p ServicePlan) cleanEnvironment(executable string) []string {
	envPath, err := exec.LookPath("env")
	if err != nil {
		envPath = "/usr/bin/env"
	}
	envPath, _ = filepath.Abs(envPath)
	return append([]string{envPath, "-i"}, append(append([]string{}, p.Env...), executable)...)
}

func (p ServicePlan) systemdUnit(description, after string, command []string) string {
	args := append(p.cleanEnvironment(command[0]), command[1:]...)
	return "[Unit]\nDescription=" + description + "\n" + after + "\n\n[Service]\nType=simple\nExecStart=" + systemdExec(args) + "\nRestart=on-failure\nRestartSec=5\nUMask=0077\nNoNewPrivileges=true\n\n[Install]\nWantedBy=default.target\n"
}

func (p ServicePlan) plist(label string, command []string, logPath string) string {
	var args strings.Builder
	for _, arg := range p.cleanEnvironment(command[0]) {
		args.WriteString("<string>" + xmlEscape(arg) + "</string>")
	}
	for _, arg := range command[1:] {
		args.WriteString("<string>" + xmlEscape(arg) + "</string>")
	}
	return `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>` + xmlEscape(label) + `</string><key>ProgramArguments</key><array>` + args.String() + `</array><key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>Umask</key><integer>63</integer><key>WorkingDirectory</key><string>` + xmlEscape(p.Layout.Dir) + `</string><key>StandardOutPath</key><string>` + xmlEscape(logPath) + `</string><key>StandardErrorPath</key><string>` + xmlEscape(logPath) + `</string><key>ProcessType</key><string>Background</string><key>ThrottleInterval</key><integer>5</integer></dict></plist>`
}

func systemdExec(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = systemdQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func systemdQuote(value string) string {
	value = strings.ReplaceAll(value, "%", "%%")
	value = strings.ReplaceAll(value, "$", "$$")
	return strconv.Quote(value)
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return replacer.Replace(value)
}

func safeEnvironment() []string {
	allowed := map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true,
		"TMPDIR": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true, "TERM": true,
	}
	values := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && allowed[key] && cleanEnvValue(value) {
			values[key] = value
		}
	}
	if values["HOME"] == "" {
		values["HOME"], _ = os.UserHomeDir()
	}
	if values["USER"] == "" {
		if current, err := user.Current(); err == nil {
			values["USER"] = current.Username
		}
	}
	if values["LOGNAME"] == "" {
		values["LOGNAME"] = values["USER"]
	}
	if values["SHELL"] == "" || !filepath.IsAbs(values["SHELL"]) {
		values["SHELL"] = "/bin/sh"
	}
	if values["PATH"] == "" {
		values["PATH"] = "/usr/local/bin:/usr/bin:/bin"
	}
	if values["TMPDIR"] == "" {
		values["TMPDIR"] = os.TempDir()
	}
	if values["TERM"] == "" {
		values["TERM"] = "xterm-256color"
	}
	return envList(values)
}

func validateEnvironment(env []string) []string {
	allowed := map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true,
		"TMPDIR": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true, "TERM": true,
	}
	values := map[string]string{}
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !allowed[key] || strings.ContainsAny(key, " \t\r\n\x00") || !cleanEnvValue(value) {
			continue
		}
		values[key] = value
	}
	return envList(values)
}

func envList(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

var runServiceOutput = func(ctx context.Context, name string, args ...string) (string, error) {
	// Root enrollments use the native system manager, without depending on a
	// root login/user bus or a root GUI session. Never elevate another user.
	if os.Geteuid() == 0 && (name == "systemctl" || name == "journalctl") && len(args) > 0 && args[0] == "--user" {
		args = args[1:]
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(output), err
}

func macServiceDomain() string {
	if os.Geteuid() == 0 {
		return "system"
	}
	return "gui/" + strconv.Itoa(os.Getuid())
}

func runServiceCommand(ctx context.Context, name string, args ...string) error {
	if _, err := runServiceOutput(ctx, name, args...); err != nil {
		return fmt.Errorf("%s failed; inspect the local service manager", name)
	}
	return nil
}

func bootoutMac(ctx context.Context, label string) error {
	output, err := runServiceOutput(ctx, "launchctl", "bootout", macServiceDomain()+"/"+label)
	if err == nil {
		return nil
	}
	for _, missing := range []string{"Could not find service", "No such process", "service not found"} {
		if strings.Contains(output, missing) {
			return nil
		}
	}
	return fmt.Errorf("launchctl bootout failed; inspect the service manager")
}

// Install writes both unit definitions and starts them through the native
// service manager. The units remain independent: stopping the agent never
// stops tmux or its sessions.
func CheckInstall(ctx context.Context, plan ServicePlan) error {
	_, err := checkInstall(ctx, plan)
	return err
}
func checkInstall(ctx context.Context, plan ServicePlan) (bool, error) {
	// Never replace a running tmux supervisor as a side effect of refreshing
	// the agent. A live service bound to another enrollment needs explicit local
	// migration after its work is finished; do not silently kill/adopt it.
	preserve := responsive(ctx, Tmux{Path: plan.TmuxPath}, plan.Layout)
	active, err := terminalServiceActive(ctx, plan)
	if err != nil {
		return false, err
	}
	if active && !preserve {
		return false, errors.New("terminal service is already active for a different or unavailable socket; preserve its work and stop or migrate that service explicitly before installing")
	}
	if preserve && !active {
		return false, errors.New("a tmux server is already running outside the terminal service; preserve its work and migrate it explicitly before installing")
	}
	return preserve, nil
}
func Install(ctx context.Context, plan ServicePlan) error {
	preserve, err := checkInstall(ctx, plan)
	if err != nil {
		return err
	}
	if err := Ensure(plan.Layout); err != nil {
		return err
	}
	if err := os.MkdirAll(plan.UnitDir, 0700); err != nil {
		return err
	}
	if err := writePrivateFile(plan.AgentPath, []byte(plan.AgentText())); err != nil {
		return err
	}
	if err := writePrivateFile(plan.TerminalPath, []byte(plan.TerminalText())); err != nil {
		return err
	}
	if runtime.GOOS == "darwin" {
		for _, item := range []struct{ label, path string }{{plan.TerminalLabel, plan.TerminalPath}, {plan.AgentLabel, plan.AgentPath}} {
			if preserve && item.label == plan.TerminalLabel {
				continue
			}
			if err := bootoutMac(ctx, item.label); err != nil {
				return err
			}
			if err := runServiceCommand(ctx, "launchctl", "bootstrap", macServiceDomain(), item.path); err != nil {
				return err
			}
			_ = runServiceCommand(ctx, "launchctl", "enable", macServiceDomain()+"/"+item.label)
		}
		return nil
	}
	if err := runServiceCommand(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	if err := runServiceCommand(ctx, "systemctl", "--user", "reset-failed", plan.AgentName, plan.TerminalName); err != nil {
		return err
	}
	if err := runServiceCommand(ctx, "systemctl", "--user", "enable", plan.TerminalName); err != nil {
		return err
	}
	if !preserve {
		if err := runServiceCommand(ctx, "systemctl", "--user", "restart", plan.TerminalName); err != nil {
			return err
		}
	}
	if err := runServiceCommand(ctx, "systemctl", "--user", "enable", plan.AgentName); err != nil {
		return err
	}
	return runServiceCommand(ctx, "systemctl", "--user", "restart", plan.AgentName)
}

func terminalServiceActive(ctx context.Context, plan ServicePlan) (bool, error) {
	if runtime.GOOS != "darwin" {
		return serviceActive(ctx, plan.TerminalName)
	}
	out, err := runServiceOutput(ctx, "launchctl", "print", macServiceDomain()+"/"+plan.TerminalLabel)
	if err == nil {
		return true, nil
	}
	for _, missing := range []string{"Could not find service", "No such process", "service not found"} {
		if strings.Contains(out, missing) {
			return false, nil
		}
	}
	return false, errors.New("could not inspect the terminal service; inspect the local service manager")
}

// ControlAgentService changes only the agent, preserving the independent tmux
// service. Refuse a shared unit bound to a different local enrollment.
func ControlAgentService(ctx context.Context, plan ServicePlan, action string) error {
	if action != "start" && action != "stop" {
		return errors.New("invalid agent service action")
	}
	if err := CheckEnrollmentConfig(plan.AgentPath); err != nil {
		return err
	}
	data, err := os.ReadFile(plan.AgentPath)
	if err != nil {
		return err
	}
	binding := systemdExec([]string{"--config", plan.ConfigPath})
	if runtime.GOOS == "darwin" {
		binding = "<string>--config</string><string>" + xmlEscape(plan.ConfigPath) + "</string>"
	}
	if !strings.Contains(string(data), binding) {
		return errors.New("installed agent service belongs to another enrollment; inspect it locally")
	}
	if action == "start" {
		if err := writePrivateFile(plan.AgentPath, []byte(plan.AgentText())); err != nil {
			return err
		}
	}
	if runtime.GOOS == "darwin" {
		if action == "stop" {
			return bootoutMac(ctx, plan.AgentLabel)
		}
		if err := bootoutMac(ctx, plan.AgentLabel); err != nil {
			return err
		}
		domain := macServiceDomain()
		if err := runServiceCommand(ctx, "launchctl", "bootstrap", domain, plan.AgentPath); err != nil {
			return err
		}
		return runServiceCommand(ctx, "launchctl", "kickstart", domain+"/"+plan.AgentLabel)
	}
	if action == "start" {
		if err := runServiceCommand(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
			return err
		}
		if err := runServiceCommand(ctx, "systemctl", "--user", "reset-failed", plan.AgentName); err != nil {
			return err
		}
	}
	return runServiceCommand(ctx, "systemctl", "--user", action, plan.AgentName)
}

// Uninstall stops both services and removes their definitions. Stopping the
// terminal service ends local tmux sessions; callers must warn before calling.
func Uninstall(ctx context.Context, plan ServicePlan) error {
	var errs []error
	if runtime.GOOS == "darwin" {
		for _, item := range []struct{ label, path string }{{plan.AgentLabel, plan.AgentPath}, {plan.TerminalLabel, plan.TerminalPath}} {
			if err := bootoutMac(ctx, item.label); err != nil {
				errs = append(errs, err)
			}
			if err := os.Remove(item.path); err != nil && !os.IsNotExist(err) {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
	if _, err := os.Stat(plan.AgentPath); err == nil {
		if err := runServiceCommand(ctx, "systemctl", "--user", "disable", "--now", plan.AgentName); err != nil {
			errs = append(errs, err)
		}
	}
	if _, err := os.Stat(plan.TerminalPath); err == nil {
		if err := runServiceCommand(ctx, "systemctl", "--user", "disable", "--now", plan.TerminalName); err != nil {
			errs = append(errs, err)
		}
	}
	if err := os.Remove(plan.AgentPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}
	if err := os.Remove(plan.TerminalPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}
	if err := runServiceCommand(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func ReadStatus(ctx context.Context, plan ServicePlan) ServiceStatus {
	status := ServiceStatus{AgentPath: plan.AgentPath, TerminalPath: plan.TerminalPath}
	_, agentFileErr := os.Stat(plan.AgentPath)
	_, terminalFileErr := os.Stat(plan.TerminalPath)
	status.Installed = agentFileErr == nil && terminalFileErr == nil
	if runtime.GOOS == "darwin" {
		agentActive := runServiceCommand(ctx, "launchctl", "print", macServiceDomain()+"/"+plan.AgentLabel) == nil
		terminalActive := runServiceCommand(ctx, "launchctl", "print", macServiceDomain()+"/"+plan.TerminalLabel) == nil
		status.ManagerAvailable = agentActive || terminalActive
		status.Active = agentActive && terminalActive
		return status
	}
	agent, agentErr := serviceActive(ctx, plan.AgentName)
	terminal, terminalErr := serviceActive(ctx, plan.TerminalName)
	if agentErr == nil || terminalErr == nil {
		status.ManagerAvailable = true
	}
	status.Active = agent && terminal
	return status
}

func serviceActive(ctx context.Context, name string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := runServiceOutput(ctx, "systemctl", "--user", "is-active", name)
	state := strings.TrimSpace(output)
	switch state {
	case "active":
		return true, nil
	case "inactive", "failed", "deactivating", "activating", "unknown":
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

// LingerGuidance explains Linux logout behaviour without changing it.
func LingerGuidance(ctx context.Context) string {
	if os.Geteuid() == 0 {
		return "Root enrollments use independent system services, which continue after logout. Host reboot still ends running shells."
	}
	if runtime.GOOS != "linux" {
		return "macOS stops per-user LaunchAgents at logout; tmux sessions continue only while the user session remains."
	}
	current, err := user.Current()
	if err != nil {
		return "Linux user services stop at logout unless lingering is enabled (loginctl enable-linger)."
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "loginctl", "show-user", current.Username, "--property=Linger", "--value").Output()
	if err == nil && strings.TrimSpace(string(output)) == "yes" {
		return "Linux user services will continue after logout because lingering is enabled."
	}
	return "Linux user services stop at logout unless you enable lingering: sudo loginctl enable-linger " + current.Username
}
