package terminalagent

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A create naming a desk pane is served only on a connection that negotiated
// desk-shell-v1; an older controller never sends it, so one that arrives
// without the feature is a protocol error, not a shell started elsewhere.
func TestCreateDeskRefNeedsFeature(t *testing.T) {
	for _, desk := range []terminalproto.DeskRef{{Pane: "0123456789abcdef"}, {Pane: "0123456789abcdef", Dir: "/srv/app"}} {
		create := &terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24, Desk: desk}
		old := terminalproto.ParseFeatures(terminalproto.ShellDirFeature + "," + terminalproto.AttentionFeature)
		if bodyFeatureError(create, old) == nil {
			t.Fatalf("desk create %+v accepted without desk-shell-v1", desk)
		}
		negotiated := terminalproto.ParseFeatures(terminalproto.ShellDirFeature + "," + terminalproto.DeskShellFeature)
		if err := bodyFeatureError(create, negotiated); err != nil {
			t.Fatalf("desk create %+v refused with desk-shell-v1: %v", desk, err)
		}
	}
	plain := &terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24}
	if err := bodyFeatureError(plain, terminalproto.ParseFeatures("")); err != nil {
		t.Fatal(err)
	}
}

// A desk folder that is gone is answered as launch_failed with the folder-gone
// detail; every other launch refusal keeps its plain answer.
func TestFolderGoneAnswer(t *testing.T) {
	if _, err := terminalhost.ResolveDeskDir("/nonexistent-reet-desk-folder"); result(err) != (terminalproto.Result{Code: terminalproto.LaunchFailed, Detail: terminalproto.FolderGone}) {
		t.Fatalf("folder gone: %+v", result(err))
	}
	if got := result(terminalhost.ErrLaunchUnavailable); got != (terminalproto.Result{Code: terminalproto.LaunchFailed}) {
		t.Fatalf("plain launch refusal: %+v", got)
	}
}

// An end that removes a desk pane's history is served only on a connection
// that negotiated desk-shell-v1; a plain end needs nothing.
func TestEndForgetDeskNeedsFeature(t *testing.T) {
	forget := &terminalproto.End{ForgetDesk: true}
	if bodyFeatureError(forget, terminalproto.ParseFeatures(terminalproto.ShellDirFeature)) == nil {
		t.Fatal("forgetDesk accepted without desk-shell-v1")
	}
	if err := bodyFeatureError(forget, terminalproto.ParseFeatures(terminalproto.DeskShellFeature)); err != nil {
		t.Fatal(err)
	}
	if err := bodyFeatureError(&terminalproto.End{}, terminalproto.ParseFeatures("")); err != nil {
		t.Fatal(err)
	}
}
