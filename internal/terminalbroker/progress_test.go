package terminalbroker

import (
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestLaunchProgressKeepsOrderPerAccountAndPrunes(t *testing.T) {
	var b Broker
	now := time.Now()
	b.progress.note(1, "a", "r1", terminalproto.StageResolving, now)
	b.progress.note(1, "a", "r1", terminalproto.StageStarting, now.Add(time.Second))
	b.progress.note(2, "a", "r1", terminalproto.StageWorktree, now)
	got := b.LaunchProgress(1, "a", "r1")
	if len(got) != 2 || got[0].Stage != terminalproto.StageResolving || got[1].Stage != terminalproto.StageStarting {
		t.Fatalf("stages = %+v", got)
	}
	if other := b.LaunchProgress(2, "a", "r1"); len(other) != 1 || other[0].Stage != terminalproto.StageWorktree {
		t.Fatalf("another account's stages leaked or went missing: %+v", other)
	}
	b.progress.note(1, "a", "r2", terminalproto.StageResolving, now.Add(progressKeep+time.Minute))
	if len(b.LaunchProgress(1, "a", "r1")) != 0 {
		t.Fatal("an old create's stages were kept")
	}
}
