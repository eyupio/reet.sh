package terminalbroker

import (
	"errors"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A level beyond low, medium and high reaches only a machine that negotiated
// effort-levels-v1, and only for an agent whose preset maps it.
func TestTerminalEffortLevelsNeedTheFeatureAndTheAgent(t *testing.T) {
	for _, tc := range []struct {
		levels       bool
		tool, effort string
		refused      bool
	}{
		{false, "claude", "max", true},
		{true, "claude", "max", false},
		{true, "codex", "max", true},
		{false, "claude", "high", false},
	} {
		b, _, uid, agent := chatFixtureCaps(t, nil, func(c *terminalproto.Capabilities) {
			c.AgentsV2, c.EffortLevels, c.Tools = true, tc.levels, []string{"claude", "codex"}
		})
		_, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Effort", Cols: 100, Rows: 30, Tool: tc.tool, Effort: tc.effort})
		if errors.Is(err, ErrLaunchOptionUnsupported) != tc.refused {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}
