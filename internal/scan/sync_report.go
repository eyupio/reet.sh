package scan

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// SyncReportOptions is one preview or apply of a saved sync target. Apply uses
// ApprovedHash so the terminal interface can show a plan and then apply exactly
// that plan without the text command's typed confirmation.
type SyncReportOptions struct {
	Server       string
	Token        string
	TokenFile    string
	Target       string
	Host         string
	Config       string
	Dir          string
	Apply        bool
	ApprovedHash string
	Version      string
	In           io.Reader
	Out          io.Writer
	OnProgress   func(stage, detail string)
}

// SyncReport is what a preview or apply saw and did. The terminal interface
// renders it; the text command keeps its own live output.
type SyncReport struct {
	Target    store.SyncTarget
	TargetSet bool
	Plan      SyncPlan
	PlanSet   bool
	Hash      string
	Backups   []string
	Progress  []string
}

// RunSyncReport previews or applies a target through the same code as the text
// command. Prompts are never shown: the caller supplies the target and, for an
// apply, the hash of the plan it showed the user.
func RunSyncReport(opts SyncReportOptions) (SyncReport, error) {
	var report SyncReport
	args := []string{"--target", opts.Target}
	if opts.Server != "" {
		args = append(args, "--server", opts.Server)
	}
	if opts.Token != "" {
		args = append(args, "--token", opts.Token)
	}
	if opts.TokenFile != "" {
		args = append(args, "--env-file", opts.TokenFile)
	}
	if opts.Host != "" {
		args = append(args, "--env", opts.Host)
	}
	if opts.Config != "" {
		args = append(args, "--config", opts.Config)
	}
	if opts.Apply {
		args = append(args, "--apply")
	}
	in := opts.In
	if in == nil {
		in = strings.NewReader("")
	}
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	options := syncOptions{
		verifyEnrollment: true,
		approvedHash:     opts.ApprovedHash,
		cwd:              opts.Dir,
		onTarget: func(target store.SyncTarget) {
			report.Target = target
			report.TargetSet = true
		},
		onPlan: func(plan SyncPlan) {
			report.Plan = plan
			report.Hash = plan.Hash()
			report.PlanSet = true
		},
		onProgress: func(stage, detail string) {
			report.Progress = append(report.Progress, stage+": "+detail)
			if opts.OnProgress != nil {
				opts.OnProgress(stage, detail)
			}
		},
		onBackup: func(path string) {
			report.Backups = append(report.Backups, path)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	err := runSyncWithOptions(args, in, out, func(args []string, cwd string) error {
		cmd := exec.CommandContext(ctx, "npx", args...)
		cmd.Dir = cwd
		cmd.Stdout = out
		cmd.Stderr = out
		cmd.Stdin = in
		return cmd.Run()
	}, opts.Version, options)
	return report, err
}
