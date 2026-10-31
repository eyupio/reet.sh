package terminalhost

import (
	"context"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type progressKey struct{}

// WithProgress returns a context whose Create reports each launch stage to fn
// as the stage begins. fn must not block: a report that cannot be delivered is
// simply lost, and never changes the outcome of the create.
func WithProgress(ctx context.Context, fn func(terminalproto.LaunchStage)) context.Context {
	return context.WithValue(ctx, progressKey{}, fn)
}

func reportStage(ctx context.Context, stage terminalproto.LaunchStage) {
	if fn, ok := ctx.Value(progressKey{}).(func(terminalproto.LaunchStage)); ok && fn != nil {
		fn(stage)
	}
}
