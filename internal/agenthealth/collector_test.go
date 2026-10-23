package agenthealth

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestCPUSamplerNeedsAnIntervalAndNormalises(t *testing.T) {
	var sampler CPUSampler
	at := time.Unix(0, 0)
	first := sampler.Observe(1000, 700, at)
	if first.Value != nil || first.Reason != terminalproto.InitialSample {
		t.Fatalf("first sample: %+v", first)
	}
	second := sampler.Observe(1100, 770, at.Add(30*time.Second))
	if second.Value == nil || math.Abs(*second.Value-30) > 1e-9 {
		t.Fatalf("second sample: %+v", second)
	}
	clampedLow := sampler.Observe(1200, 900, at.Add(time.Minute))
	if clampedLow.Value == nil || *clampedLow.Value != 0 {
		t.Fatalf("clamped low sample: %+v", clampedLow)
	}
	clampedHigh := sampler.Observe(1300, 900, at.Add(90*time.Second))
	if clampedHigh.Value == nil || *clampedHigh.Value != 100 {
		t.Fatalf("clamped high sample: %+v", clampedHigh)
	}
	if zero := sampler.Observe(1300, 900, at.Add(2*time.Minute)); zero.Value != nil || zero.Reason != terminalproto.CollectionFailed {
		t.Fatalf("zero interval: %+v", zero)
	}
	if bad := sampler.Observe(math.NaN(), 1, at); bad.Value != nil || bad.Reason != terminalproto.CollectionFailed {
		t.Fatalf("nonfinite sample: %+v", bad)
	}
}

func TestPercentileUsesRetainedSamples(t *testing.T) {
	samples := []float64{40, 10, 30, 20}
	if got := Percentile(samples, 0.5); got == nil || *got != 25 {
		t.Fatalf("p50: %v", got)
	}
	if got := Percentile(samples, 0.95); got == nil || math.Abs(*got-38.5) > 1e-9 {
		t.Fatalf("p95: %v", got)
	}
	if Percentile(nil, 0.5) != nil || Percentile(samples, -1) != nil {
		t.Fatal("unbounded percentile input accepted")
	}
	single := Percentile([]float64{7}, 0.95)
	if single == nil || *single != 7 {
		t.Fatalf("single sample: %v", single)
	}
	// Callers may combine minute samples; the input must not be reordered.
	if samples[0] != 40 || samples[1] != 10 {
		t.Fatal("percentile reordered the caller's samples")
	}
}

func TestAccumulatorAggregatesAndBoundsSamples(t *testing.T) {
	acc := NewAccumulator(60_000, 3)
	for i := 0; i < maxSamplesPerMinute+20; i++ {
		acc.AddReport(testReport(float64(i%100), i%2 == 0))
	}
	summary := acc.Summary()
	if summary.Validate() != nil {
		t.Fatal(summary.Validate())
	}
	if summary.CPUPercent.Valid != maxSamplesPerMinute || summary.CPUPercent.Missing != 0 || summary.MissedProbes != 0 {
		t.Fatalf("cpu aggregate: %+v", summary.CPUPercent)
	}
	for i := 0; i < maxRTTSamples+10; i++ {
		acc.AddRTT(float64(i))
	}
	for i := 0; i < maxSamplesPerMinute+10; i++ {
		acc.MissProbe()
	}
	acc.NoteReconnect()
	acc.AddTraffic(10, 5)
	summary = acc.Summary()
	if len(summary.RTTMillis) != maxRTTSamples || summary.MissedProbes != maxSamplesPerMinute || summary.Reconnects != 1 {
		t.Fatalf("bounded summary: %+v", summary)
	}
	if summary.InputBytes != 10 || summary.OutputBytes != 5 {
		t.Fatalf("traffic: %+v", summary)
	}
	if summary.Validate() != nil {
		t.Fatal(summary.Validate())
	}
}

func TestAccumulatorKeepsMissingReasonsAsMissing(t *testing.T) {
	acc := NewAccumulator(60_000, 1)
	report := terminalproto.HealthReport{
		SchemaVersion: terminalproto.Version, Sequence: 1, AgentInstanceID: testInstance(),
		SampledAt: 1, Scope: "container",
		CPUPercent:        terminalproto.Reading{Reason: terminalproto.InitialSample},
		Memory:            terminalproto.CapacityReading{Total: terminalproto.Reading{Reason: terminalproto.PermissionDenied}, Available: terminalproto.Reading{Reason: terminalproto.PermissionDenied}},
		Disk:              terminalproto.CapacityReading{Total: terminalproto.Reading{Reason: terminalproto.NotSupported}, Available: terminalproto.Reading{Reason: terminalproto.NotSupported}},
		HostUptimeSeconds: terminalproto.Reading{Reason: terminalproto.CollectionFailed},
	}
	acc.AddReport(report)
	summary := acc.Summary()
	if summary.CPUPercent.Valid != 0 || summary.CPUPercent.Missing != 1 || summary.MemoryTotalBytes.Missing != 1 || summary.DiskTotalBytes.Missing != 1 || summary.HostUptimeSeconds.Missing != 1 {
		t.Fatalf("missing values were fabricated: %+v", summary)
	}
	if summary.Scope != "container" || summary.Validate() != nil {
		t.Fatalf("scope/validation: %+v %v", summary, summary.Validate())
	}
}

func testReport(cpu float64, validMemory bool) terminalproto.HealthReport {
	memory := terminalproto.CapacityReading{Total: terminalproto.Reading{Reason: terminalproto.CollectionFailed}, Available: terminalproto.Reading{Reason: terminalproto.CollectionFailed}}
	if validMemory {
		total, available := float64(1024), float64(256)
		memory = terminalproto.CapacityReading{Total: terminalproto.Reading{Value: &total}, Available: terminalproto.Reading{Value: &available}}
	}
	return terminalproto.HealthReport{
		SchemaVersion:     terminalproto.Version,
		Sequence:          int64(cpu) + 1,
		AgentInstanceID:   testInstance(),
		SampledAt:         1,
		Scope:             "host",
		CPUPercent:        terminalproto.Reading{Value: &cpu},
		Memory:            memory,
		Disk:              memory,
		HostUptimeSeconds: terminalproto.Reading{Value: &cpu},
	}
}

func testInstance() string { return "instance0123456789abcdef" }

func TestDefaultCollectorProducesBoundedReadings(t *testing.T) {
	collector := NewCollector()
	first := collector.Collect(context.Background(), t.TempDir())
	if first.Scope != "host" && first.Scope != "container" {
		t.Fatal(first.Scope)
	}
	if first.CPUPercent.Value != nil || first.CPUPercent.Reason != terminalproto.InitialSample {
		t.Fatalf("first CPU reading: %+v", first.CPUPercent)
	}
	time.Sleep(10 * time.Millisecond)
	second := collector.Collect(context.Background(), t.TempDir())
	for name, reading := range map[string]terminalproto.Reading{
		"cpu": second.CPUPercent, "uptime": second.HostUptimeSeconds,
		"memory total": second.Memory.Total, "memory available": second.Memory.Available,
		"disk total": second.Disk.Total, "disk available": second.Disk.Available,
	} {
		if reading.Value == nil && reading.Reason == "" {
			t.Fatalf("%s had neither a value nor a reason", name)
		}
		if reading.Value != nil && reading.Reason != "" {
			t.Fatalf("%s had both a value and a reason", name)
		}
	}
}
