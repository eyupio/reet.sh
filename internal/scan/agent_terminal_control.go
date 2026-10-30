//go:build linux || darwin

package scan

import (
	"errors"
	"os/exec"
	"path/filepath"

	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/terminalagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
)

func terminalControl(config string, c agentConfig) agenthealth.TerminalFactory {
	layout, err := terminalhost.NewEnrollmentLayout(config, c.ID)
	if err != nil {
		return nil
	}
	return terminalagent.Factory(terminalhost.ManagerOptions{EnrollmentID: c.ID, Layout: layout, Home: c.Home, YoloConsent: func() error { return yoloConsent(config, c) }, Consent: func() error {
		current, err := loadAgentConfig(config)
		if err != nil {
			return err
		}
		if current.ID != c.ID || current.Secret != c.Secret || current.Server != c.Server || !current.TerminalEnabled {
			return errors.New("terminal access disabled locally")
		}
		return nil
	}, BrowseConsent: func() error { return browseConsent(config, c) }, ClipboardConsent: func() error { return clipboardConsent(config, c) }, UploadConsent: func() error { return uploadConsent(config, c) }, ModelsConsent: func() error { return modelsConsent(config, c) }})
}

// launchTools reports the agent CLIs found on this connector's PATH from its
// built-in launch presets.
func launchTools() []string { return terminalhost.InstalledTools(exec.LookPath, true) }

// gitAvailable reports whether git is on this connector's PATH (as an
// absolute path, the way the worktree runner finds it), for run worktrees.
func gitAvailable() bool {
	path, err := exec.LookPath("git")
	return err == nil && filepath.IsAbs(path)
}

// browseAllowed re-reads the enrollment: folder browsing is allowed while it
// is still this enrollment, terminal access is on and browse is not off
// locally (reet terminal browse off).
func browseAllowed(config string, c agentConfig) func() bool {
	return func() bool { return browseConsent(config, c) == nil }
}

// clipboardAllowed re-reads the enrollment: clipboard forwarding is allowed
// while it is still this enrollment, terminal access is on and clipboard is
// not off locally (reet terminal clipboard off).
func clipboardAllowed(config string, c agentConfig) func() bool {
	return func() bool { return clipboardConsent(config, c) == nil }
}

// uploadsAllowed re-reads the enrollment: image uploads are allowed while it
// is still this enrollment, terminal access is on and uploads are not off
// locally (reet terminal uploads off).
func uploadsAllowed(config string, c agentConfig) func() bool {
	return func() bool { return uploadConsent(config, c) == nil }
}

// yoloConsent re-reads the enrollment and the machine's YOLO switch.
func yoloConsent(config string, c agentConfig) error {
	current, err := currentTerminalConfig(config, c)
	if err != nil {
		return err
	}
	if current.YoloOff {
		return errors.New("YOLO disabled locally")
	}
	return nil
}

// yoloAllowed re-reads the enrollment: YOLO is allowed while it is still this
// enrollment, terminal access is on and YOLO is not off locally
// (reet terminal yolo off).
func yoloAllowed(config string, c agentConfig) func() bool {
	return func() bool { return yoloConsent(config, c) == nil }
}

// modelsAllowed re-reads the enrollment: model listing is allowed while it is
// still this enrollment, terminal access is on and models are not off locally
// (reet terminal models off).
func modelsAllowed(config string, c agentConfig) func() bool {
	return func() bool { return modelsConsent(config, c) == nil }
}

// terminalAllowed re-reads the enrollment alone: it is still this one and
// terminal access is on. Unlike the consent poll it probes no readiness
// (tmux), so it is cheap enough to check before every browse or branches
// request.
func terminalAllowed(config string, c agentConfig) func() bool {
	return func() bool {
		_, err := currentTerminalConfig(config, c)
		return err == nil
	}
}

// currentTerminalConfig loads the enrollment and checks it is still this
// one with terminal access on.
func currentTerminalConfig(config string, c agentConfig) (agentConfig, error) {
	current, err := loadAgentConfig(config)
	if err != nil {
		return agentConfig{}, err
	}
	if current.ID != c.ID || current.Secret != c.Secret || current.Server != c.Server || !current.TerminalEnabled {
		return agentConfig{}, errors.New("terminal access disabled locally")
	}
	return current, nil
}

func browseConsent(config string, c agentConfig) error {
	current, err := currentTerminalConfig(config, c)
	if err != nil {
		return err
	}
	if current.BrowseOff {
		return errors.New("folder browse disabled locally")
	}
	return nil
}

func clipboardConsent(config string, c agentConfig) error {
	current, err := currentTerminalConfig(config, c)
	if err != nil {
		return err
	}
	if current.ClipboardOff {
		return errors.New("clipboard disabled locally")
	}
	return nil
}

func uploadConsent(config string, c agentConfig) error {
	current, err := currentTerminalConfig(config, c)
	if err != nil {
		return err
	}
	if current.UploadsOff {
		return errors.New("image uploads disabled locally")
	}
	return nil
}

func modelsConsent(config string, c agentConfig) error {
	current, err := currentTerminalConfig(config, c)
	if err != nil {
		return err
	}
	if current.ModelsOff {
		return errors.New("model listing disabled locally")
	}
	return nil
}
