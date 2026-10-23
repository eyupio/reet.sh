// Package agenthealth collects host observations on the installed agent and
// tracks controller-side connection and health state.
package agenthealth

import (
	"context"
	"errors"
	"math"
	"os"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Sample is one bounded, read-only host observation. Missing readings are null
// with a fixed reason; a failed collector must not invent zero or reuse a value.
type Sample struct {
	SampledAt         time.Time
	Scope             string
	CPUPercent        terminalproto.Reading
	Memory            terminalproto.CapacityReading
	Disk              terminalproto.CapacityReading
	HostUptimeSeconds terminalproto.Reading
}

// Collector reads current host resources. Implementations must honour ctx and
// must not parse human-readable command output.
type Collector interface {
	Collect(ctx context.Context, home string) Sample
}

// CPUSampler normalises busy time over the interval since its previous sample.
// The first observation has no interval, so it reports initial_sample.
type CPUSampler struct {
	have       bool
	total      float64
	idle       float64
	lastSample time.Time
}

func (s *CPUSampler) Observe(total, idle float64, at time.Time) terminalproto.Reading {
	if math.IsNaN(total) || math.IsInf(total, 0) || math.IsNaN(idle) || math.IsInf(idle, 0) || total < 0 || idle < 0 {
		return terminalproto.Reading{Reason: terminalproto.CollectionFailed}
	}
	if !s.have {
		s.have, s.total, s.idle, s.lastSample = true, total, idle, at
		return terminalproto.Reading{Reason: terminalproto.InitialSample}
	}
	deltaTotal := total - s.total
	deltaIdle := idle - s.idle
	s.total, s.idle, s.lastSample = total, idle, at
	if deltaTotal <= 0 || deltaIdle < 0 {
		return terminalproto.Reading{Reason: terminalproto.CollectionFailed}
	}
	busy := (deltaTotal - deltaIdle) / deltaTotal * 100
	if busy < 0 {
		busy = 0
	}
	if busy > 100 {
		busy = 100
	}
	return terminalproto.Reading{Value: &busy}
}

// DefaultCollector uses the tested gopsutil adapters. It is read-only and does
// not require root; unavailable readings stay null.
type DefaultCollector struct {
	mu  sync.Mutex
	cpu CPUSampler
}

func NewCollector() *DefaultCollector { return &DefaultCollector{} }

func (c *DefaultCollector) Collect(ctx context.Context, home string) Sample {
	s := Sample{SampledAt: time.Now(), Scope: scope()}
	total, idle := cpuTimes(ctx)
	c.mu.Lock()
	s.CPUPercent = c.cpu.Observe(total, idle, s.SampledAt)
	c.mu.Unlock()
	s.Memory = memoryReading(ctx)
	s.Disk = diskReading(ctx, home)
	s.HostUptimeSeconds = uptimeReading(ctx)
	return s
}

func cpuTimes(ctx context.Context) (float64, float64) {
	stats, err := cpu.TimesWithContext(ctx, false)
	if err != nil || len(stats) == 0 {
		return math.NaN(), math.NaN()
	}
	var total, idle float64
	for _, stat := range stats {
		total += stat.Total()
		idle += stat.Idle
	}
	return total, idle
}

func memoryReading(ctx context.Context) terminalproto.CapacityReading {
	stat, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil || stat == nil || stat.Total == 0 {
		reason := reasonFor(err)
		return terminalproto.CapacityReading{
			Total:     terminalproto.Reading{Reason: reason},
			Available: terminalproto.Reading{Reason: reason},
		}
	}
	return capacity(stat.Total, stat.Available)
}

func diskReading(ctx context.Context, home string) terminalproto.CapacityReading {
	if home == "" {
		home = "/"
	}
	stat, err := disk.UsageWithContext(ctx, home)
	if err != nil || stat == nil || stat.Total == 0 {
		reason := reasonFor(err)
		return terminalproto.CapacityReading{
			Total:     terminalproto.Reading{Reason: reason},
			Available: terminalproto.Reading{Reason: reason},
		}
	}
	return capacity(stat.Total, stat.Free)
}

func uptimeReading(ctx context.Context) terminalproto.Reading {
	seconds, err := host.UptimeWithContext(ctx)
	if err != nil || seconds == 0 {
		return terminalproto.Reading{Reason: reasonFor(err)}
	}
	value := float64(seconds)
	if value > 100*366*24*60*60 {
		return terminalproto.Reading{Reason: terminalproto.CollectionFailed}
	}
	return terminalproto.Reading{Value: &value}
}

func capacity(total, available uint64) terminalproto.CapacityReading {
	if available > total {
		available = total
	}
	t, a := float64(total), float64(available)
	return terminalproto.CapacityReading{
		Total:     terminalproto.Reading{Value: &t},
		Available: terminalproto.Reading{Value: &a},
	}
}

func reasonFor(err error) terminalproto.MissingReason {
	if os.IsPermission(err) {
		return terminalproto.PermissionDenied
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return terminalproto.CollectionFailed
	}
	return terminalproto.CollectionFailed
}

// scope marks container observations so the UI never claims full host health.
func scope() string {
	if os.Getenv("container") != "" || os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return "container"
	}
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return "container"
		}
	}
	return "host"
}
