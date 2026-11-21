//go:build linux || darwin

package terminalagent

import (
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// checkFeatures is the gate every message passes first: a create asking for
// something the generation did not negotiate, or any body of a kind it did not
// negotiate, is refused here. It reports whether the message was answered, in
// which case err is what the dispatcher returns.
func (r *agentReq) checkFeatures() (answered bool, err error) {
	msg, callCtx, features := r.msg, r.callCtx, r.features
	// Launch fields are served only on a generation that negotiated
	// launch-v1; a plain shell is never substituted for them.
	launchCreate, launchOK := msg.Body.(*terminalproto.Create)
	launchNeeded := launchOK && launchCreate.Tool != ""
	if launchNeeded && !features.Has(terminalproto.LaunchFeature) {
		return true, r.respond(terminalhost.ErrLaunchUnavailable)
	}
	// A resume is accepted only on a generation that negotiated
	// resume-v1; an older controller never sends it.
	resumeCreate, resumeOK := msg.Body.(*terminalproto.Create)
	resumeNeeded := resumeOK && resumeCreate.Resume
	if resumeNeeded && !features.Has(terminalproto.ResumeFeature) {
		r.reply.Body = terminalproto.Result{Code: terminalproto.LaunchOptionUnsupported}
		return true, r.send(callCtx, r.reply)
	}
	// Likewise a worktree is made only on a generation that negotiated
	// worktree-v1; without it the agent is never started in the folder.
	worktreeCreate, worktreeOK := msg.Body.(*terminalproto.Create)
	worktreeNeeded := worktreeOK && worktreeCreate.Worktree != ""
	if worktreeNeeded && !features.Has(terminalproto.WorktreeFeature) {
		return true, r.respond(terminalhost.ErrLaunchUnavailable)
	}
	if create, ok := msg.Body.(*terminalproto.Create); ok {
		if create.Structured && create.Worktree != "" && !features.Has(terminalproto.ACPWorktreeFeature) {
			r.reply.Body = terminalproto.Result{Code: terminalproto.LaunchOptionUnsupported}
			return true, r.send(callCtx, r.reply)
		}
		if create.Tool == "antigravity" && !features.Has(terminalproto.AntigravityFeature) {
			r.reply.Body = terminalproto.Result{Code: terminalproto.LaunchOptionUnsupported}
			return true, r.send(callCtx, r.reply)
		}
		if create.Tool == "copilot" && !features.Has(terminalproto.CopilotFeature) {
			r.reply.Body = terminalproto.Result{Code: terminalproto.LaunchOptionUnsupported}
			return true, r.send(callCtx, r.reply)
		}
		if create.Tool == "antigravity" && create.Structured && !features.Has(terminalproto.AntigravityACPFeature) {
			r.reply.Body = terminalproto.Result{Code: terminalproto.LaunchOptionUnsupported}
			return true, r.send(callCtx, r.reply)
		}
		needsStartOptions := create.Structured && create.Effort != ""
		if needsStartOptions && !features.Has(terminalproto.ACPStartOptionsFeature) {
			r.reply.Body = terminalproto.Result{Code: terminalproto.LaunchOptionUnsupported}
			return true, r.send(callCtx, r.reply)
		}
		if create.Provider != "" && !features.Has(terminalproto.ACPProviderFeature) {
			r.reply.Body = terminalproto.Result{Code: terminalproto.LaunchOptionUnsupported}
			return true, r.send(callCtx, r.reply)
		}
		needsAgents := create.Base != "" || (!create.Structured && create.Effort != "") ||
			create.Yolo || (create.Tool != "" && !terminalproto.ValidTool(create.Tool))
		if needsAgents && !features.Has(terminalproto.AgentsFeature) {
			r.reply.Body = terminalproto.Result{Code: terminalproto.LaunchOptionUnsupported}
			return true, r.send(callCtx, r.reply)
		}
	}
	if err := bodyFeatureError(msg.Body, features); err != nil {
		return true, err
	}
	create, ok := msg.Body.(*terminalproto.Create)
	if ok {
		needsShellDir := create.Tool == "" && create.Dir != ""
		if needsShellDir && !features.Has(terminalproto.ShellDirFeature) {
			r.reply.Body = terminalproto.Result{Code: terminalproto.Unavailable}
			return true, r.send(callCtx, r.reply)
		}
	}
	acpCreate, acpOK := msg.Body.(*terminalproto.Create)
	needsACP := acpOK && acpCreate.Structured
	if needsACP && !features.Has(terminalproto.ACPFeature) {
		r.reply.Body = terminalproto.Result{Code: terminalproto.ACPUnavailable}
		return true, r.send(callCtx, r.reply)
	}
	return false, nil
}
