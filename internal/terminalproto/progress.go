package terminalproto

import "strings"

// ProgressFeature lets a machine report the stages of a launch while its
// create call is still running, so the person can see where a slow start is.
// It adds one message kind (launch_progress) and nothing else: a controller
// that does not advertise it is never sent one, and a machine that does not
// claim it never sends one.
const ProgressFeature = "launch-progress-v1"

// SupportsProgress reports whether features includes launch-progress-v1.
func SupportsProgress(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == ProgressFeature {
			return true
		}
	}
	return false
}

// LaunchStage names one stage of a create on the machine, in the order they
// happen. A stage is announced when it begins.
type LaunchStage string

const (
	// StageResolving: checking the folder and finding the agent program.
	StageResolving LaunchStage = "resolving"
	// StageWorktree: making (or adopting) the git worktree. Only worktree launches.
	StageWorktree LaunchStage = "worktree"
	// StageStarting: asking tmux to start the session and the agent in it.
	StageStarting LaunchStage = "starting"
	// StageConfirming: reading back that the session is running.
	StageConfirming LaunchStage = "confirming"
)

func (s LaunchStage) Valid() bool {
	return s == StageResolving || s == StageWorktree || s == StageStarting || s == StageConfirming
}

// LaunchProgress is sent by the machine, carrying the request id and session
// id of the create it belongs to, before that create's single answer.
type LaunchProgress struct {
	Stage LaunchStage `json:"stage"`
}

func (LaunchProgress) kind() string  { return "launch_progress" }
func (p LaunchProgress) valid() bool { return p.Stage.Valid() }
