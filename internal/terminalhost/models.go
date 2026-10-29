//go:build linux || darwin

package terminalhost

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// These are variables so a test can shorten the deadline and the caps. The
// output limits bound everything the connector reads into memory.
var (
	modelListTimeout     = 10 * time.Second
	modelListOutputLimit = 64 << 10
	modelListStderrLimit = 4 << 10
)

// ListModels runs the connector-owned listing for tool and parses its output.
// Only an enabled preset with a ModelList runs anything; every other tool is
// unsupported without a process. The answer always fits the protocol's model
// reply size and is a valid ModelsResult.
func ListModels(ctx context.Context, tool string, lookPath func(string) (string, error), home string) terminalproto.ModelsResult {
	result := terminalproto.ModelsResult{Tool: tool}
	p, ok := launchPresets[tool]
	if !ok || p.modelList == nil || len(p.modelList.Argv) == 0 {
		result.Reason = terminalproto.ModelsUnsupported
		return result
	}
	binary, ok := resolveBinary(p, lookPath)
	if !ok {
		result.Reason = terminalproto.ModelsNotInstalled
		return result
	}
	dir, err := os.MkdirTemp("", "reet-models-")
	if err != nil {
		result.Reason = terminalproto.ModelsExited
		return result
	}
	defer os.RemoveAll(dir)
	runCtx, cancel := context.WithTimeout(ctx, modelListTimeout)
	defer cancel()
	cmd := exec.Command(binary, p.modelList.Argv...)
	cmd.Env = modelListEnvironment(home, p.modelList.Env)
	cmd.Dir = dir
	// stdin is nil, so the child reads /dev/null.
	cmd.Stdin = nil
	// Its own process group lets a timeout kill anything it started.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr cappedBuffer
	stdout.limit = modelListOutputLimit
	stderr.limit = modelListStderrLimit
	// os/exec writes stdout and stderr itself, so Wait cannot race a pipe
	// reader. The writer swallows an overflow and keeps draining; the limit
	// is remembered and checked after Wait.
	cmd.Stdout = modelOutput{&stdout}
	cmd.Stderr = modelOutput{&stderr}
	if err := cmd.Start(); err != nil {
		result.Reason = terminalproto.ModelsExited
		return result
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timedOut := false
	var waitErr error
	select {
	case waitErr = <-done:
	case <-runCtx.Done():
		timedOut = true
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		waitErr = <-done
	}
	if stdout.overflow {
		result.Reason = terminalproto.ModelsTooLarge
		return result
	}
	if timedOut || runCtx.Err() != nil {
		// A cancelled context is a timeout: no answer can be trusted.
		result.Reason = terminalproto.ModelsTimedOut
		return result
	}
	if waitErr != nil && (cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 0) {
		result.Reason = terminalproto.ModelsExited
		return result
	}
	return parseModels(tool, stdout.Bytes())
}

// modelOutput exposes only Write. cappedBuffer embeds bytes.Buffer, whose
// ReadFrom would otherwise let io.Copy bypass the limit entirely.
type modelOutput struct{ buf *cappedBuffer }

func (w modelOutput) Write(p []byte) (int, error) {
	if w.buf.overflow {
		return len(p), nil
	}
	if _, err := w.buf.Write(p); err != nil && !errors.Is(err, ErrOutputLimit) {
		return len(p), err
	}
	return len(p), nil
}

// modelListEnvironment starts from the launch allowlist, drops TERM and SHELL,
// pins HOME to the connector's home and applies the row's fixed opt-outs. A
// repeated key is replaced, never emitted twice.
func modelListEnvironment(home string, extra []string) []string {
	values := map[string]string{}
	for _, entry := range safeEnvironment() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "TERM" || key == "SHELL" {
			continue
		}
		values[key] = value
	}
	values["HOME"] = home
	for _, entry := range extra {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" || strings.ContainsAny(key, " \t\r\n\x00") || !cleanEnvValue(value) {
			continue
		}
		values[key] = value
	}
	return envList(values)
}

// parseModels rejects a whole output that carries a control byte or ESC,
// skips lines the protocol cannot carry, de-duplicates, caps the list at 500
// and trims the encoded answer to MaxModelsReplyBytes.
func parseModels(tool string, out []byte) terminalproto.ModelsResult {
	result := terminalproto.ModelsResult{Tool: tool}
	for _, b := range out {
		if b < 0x20 && b != '\n' && b != '\t' {
			result.Reason = terminalproto.ModelsUnreadable
			return result
		}
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !terminalproto.ValidModel(line) || seen[line] {
			continue
		}
		if len(result.Models) == terminalproto.MaxModels {
			result.More = true
			break
		}
		seen[line] = true
		provider := ""
		if index := strings.IndexByte(line, '/'); index >= 0 {
			provider = line[:index]
		}
		if len(provider) > 64 {
			continue
		}
		result.Models = append(result.Models, terminalproto.ModelEntry{ID: line, Provider: provider})
	}
	trimModels(&result)
	return result
}

// trimModels drops the tail of a too-large encoded answer and marks it cut.
func trimModels(result *terminalproto.ModelsResult) {
	for {
		data, err := json.Marshal(result)
		if err != nil || len(data) <= terminalproto.MaxModelsReplyBytes {
			return
		}
		if len(result.Models) == 0 {
			return
		}
		result.Models = result.Models[:len(result.Models)-1]
		result.More = true
	}
}
