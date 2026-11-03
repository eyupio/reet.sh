//go:build linux || darwin

package terminalhost

import (
	"context"
	"fmt"
	"syscall"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// BenchmarkReconcile measures one tick of the Manager.Run loop (which runs
// once a second on every connected machine) against real tmux sessions. It
// also reports the CPU time spent in child processes, which is where the
// tmux invocations are charged.
func BenchmarkReconcile(b *testing.B) {
	for _, n := range []int{0, 1, 4, 8} {
		b.Run(fmt.Sprintf("sessions=%d", n), func(b *testing.B) {
			m, _ := managerFixture(b)
			for i := 1; i <= n; i++ {
				if _, err := m.Create(context.Background(), testRequestID(i), testSessionID(i), terminalproto.Create{Title: "Build", Cols: 80, Rows: 24}); err != nil {
					b.Fatal(err)
				}
			}
			var before syscall.Rusage
			_ = syscall.Getrusage(syscall.RUSAGE_CHILDREN, &before)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := m.Reconcile(context.Background()); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			var after syscall.Rusage
			_ = syscall.Getrusage(syscall.RUSAGE_CHILDREN, &after)
			cpu := time.Duration(after.Utime.Nano()-before.Utime.Nano()) + time.Duration(after.Stime.Nano()-before.Stime.Nano())
			b.ReportMetric(float64(cpu.Nanoseconds())/float64(b.N), "child-cpu-ns/op")
		})
	}
}
