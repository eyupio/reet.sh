package lifecycle

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/cliui"
)

func (a *App) context() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}
func (a *App) progressFlags(f *flag.FlagSet) {
	f.BoolVar(&a.noAnimation, "no-animation", false, "disable terminal animations")
	f.BoolVar(&a.nonInteractive, "non-interactive", os.Getenv("REET_NON_INTERACTIVE") != "", "never prompt; changes still require --yes")
}
func (a *App) presentation(disabled bool) cliui.TaskOptions {
	return cliui.TaskOptions{Disabled: disabled || a.recovering || a.structured || a.nonInteractive || !cliui.Terminal(a.Out), NoAnimation: a.noAnimation}
}
func (a *App) task(ctx context.Context, label string, disabled bool, work cliui.Work) error {
	// Recovery must neither depend on the renderer nor install another signal
	// context that a repeated interrupt could cancel.
	if a.recovering {
		return work(ctx, func(string) {})
	}
	if a.taskRunner != nil {
		return a.taskRunner(ctx, label, disabled, work)
	}
	return cliui.Task(ctx, a.In, a.Progress, label, a.presentation(disabled), work)
}
func (a *App) step(ctx context.Context, label string, work func(context.Context) error) error {
	return cliui.Step(ctx, a.In, a.Progress, label, a.presentation(false), work)
}
func (a *App) command(ctx context.Context, name string, args ...string) (string, error) {
	label := "Running " + filepath.Base(name)
	if strings.Contains(filepath.Base(name), ".reet-download-") {
		label = "Checking the verified binary's version"
	}
	var output string
	err := a.step(ctx, label, func(ctx context.Context) error { var e error; output, e = a.run(ctx, name, args...); return e })
	return output, err
}
func recoveryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
}
func (a *App) recoverServer(ctx context.Context, d *deployment, state string) error {
	previous := a.recovering
	a.recovering = true
	defer func() { a.recovering = previous }()
	ctx, cancel := recoveryContext(ctx)
	defer cancel()
	if err := a.start(ctx, d, state); err != nil {
		return err
	}
	return a.waitHealthy(ctx, d)
}
