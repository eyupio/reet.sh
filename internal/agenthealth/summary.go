package agenthealth

import (
	"math"
	"sort"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	maxSamplesPerMinute = 120
	maxRTTSamples       = 64
	maxRTTMillis        = 5000
)

type metricAgg struct {
	valid, missing int
	min, max, sum  float64
}

func (a *metricAgg) add(reading terminalproto.Reading, max float64) {
	if a.valid+a.missing >= maxSamplesPerMinute {
		return
	}
	if reading.Value == nil || !finite(*reading.Value, max) {
		a.missing++
		return
	}
	value := *reading.Value
	if a.valid == 0 {
		a.min, a.max = value, value
	} else {
		a.min = math.Min(a.min, value)
		a.max = math.Max(a.max, value)
	}
	a.valid++
	a.sum += value
}

func (a metricAgg) aggregate() terminalproto.Aggregate {
	return terminalproto.Aggregate{Valid: a.valid, Missing: a.missing, Min: a.min, Max: a.max, Sum: a.sum}
}

func finite(value, max float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= max
}

// Accumulator builds one cumulative controller-minute summary. It never fills
// gaps or carries a previous reading forward as a new observation.
type Accumulator struct {
	Minute     int64
	Generation int64
	Scope      string

	cpu             metricAgg
	memoryTotal     metricAgg
	memoryAvailable metricAgg
	diskTotal       metricAgg
	diskAvailable   metricAgg
	hostUptime      metricAgg
	rtt             []float64
	missedProbes    int
	reconnects      int
	inputBytes      int64
	outputBytes     int64
}

func NewAccumulator(minute, generation int64) *Accumulator {
	return &Accumulator{Minute: minute, Generation: generation, Scope: "host"}
}

func (a *Accumulator) AddReport(report terminalproto.HealthReport) {
	if report.Scope == "host" || report.Scope == "container" {
		a.Scope = report.Scope
	}
	a.cpu.add(report.CPUPercent, 100)
	a.memoryTotal.add(report.Memory.Total, float64(terminalproto.MaxCounter))
	a.memoryAvailable.add(report.Memory.Available, float64(terminalproto.MaxCounter))
	a.diskTotal.add(report.Disk.Total, float64(terminalproto.MaxCounter))
	a.diskAvailable.add(report.Disk.Available, float64(terminalproto.MaxCounter))
	a.hostUptime.add(report.HostUptimeSeconds, 100*366*24*60*60)
}

func (a *Accumulator) AddRTT(millis float64) {
	if len(a.rtt) >= maxRTTSamples || !finite(millis, maxRTTMillis) {
		return
	}
	a.rtt = append(a.rtt, millis)
}

func (a *Accumulator) MissProbe() {
	if a.missedProbes < maxSamplesPerMinute {
		a.missedProbes++
	}
}

func (a *Accumulator) NoteReconnect() {
	if a.reconnects < maxSamplesPerMinute {
		a.reconnects++
	}
}

func (a *Accumulator) AddTraffic(input, output int64) {
	if input < 0 || output < 0 || a.inputBytes > terminalproto.MaxCounter-input || a.outputBytes > terminalproto.MaxCounter-output {
		return
	}
	a.inputBytes += input
	a.outputBytes += output
}

func (a *Accumulator) Summary() terminalproto.HealthSummary {
	return terminalproto.HealthSummary{
		SchemaVersion:        terminalproto.Version,
		Scope:                a.Scope,
		CPUPercent:           a.cpu.aggregate(),
		MemoryTotalBytes:     a.memoryTotal.aggregate(),
		MemoryAvailableBytes: a.memoryAvailable.aggregate(),
		DiskTotalBytes:       a.diskTotal.aggregate(),
		DiskAvailableBytes:   a.diskAvailable.aggregate(),
		HostUptimeSeconds:    a.hostUptime.aggregate(),
		RTTMillis:            append([]float64(nil), a.rtt...),
		MissedProbes:         a.missedProbes,
		Reconnects:           a.reconnects,
		InputBytes:           a.inputBytes,
		OutputBytes:          a.outputBytes,
	}
}

// Percentile returns a linearly interpolated percentile, or nil for an empty
// sample set. Combined RTT percentiles must use the retained samples, never an
// average of per-minute percentiles.
func Percentile(samples []float64, p float64) *float64 {
	if len(samples) == 0 || p < 0 || p > 1 {
		return nil
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	if len(sorted) == 1 {
		value := sorted[0]
		return &value
	}
	position := p * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		value := sorted[lower]
		return &value
	}
	weight := position - float64(lower)
	value := sorted[lower]*(1-weight) + sorted[upper]*weight
	return &value
}
