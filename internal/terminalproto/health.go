package terminalproto

import "math"

type MissingReason string

const (
	InitialSample    MissingReason = "initial_sample"
	NotSupported     MissingReason = "unsupported"
	PermissionDenied MissingReason = "permission_denied"
	CollectionFailed MissingReason = "collection_failed"
)

// Reading's units come from its containing field. Missing is explicit, never a
// fabricated zero; no free-form collector diagnostic can carry sensitive data.
type Reading struct {
	Value  *float64      `json:"value"`
	Reason MissingReason `json:"reason,omitempty"`
}

func finiteRange(v, max float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= max
}
func (r Reading) valid(max float64) bool {
	if r.Value != nil {
		return r.Reason == "" && finiteRange(*r.Value, max)
	}
	return r.Reason == InitialSample || r.Reason == NotSupported || r.Reason == PermissionDenied || r.Reason == CollectionFailed
}

type CapacityReading struct {
	Total     Reading `json:"totalBytes"`
	Available Reading `json:"availableBytes"`
}

func (r CapacityReading) valid() bool {
	return r.Total.valid(float64(MaxCounter)) && r.Available.valid(float64(MaxCounter)) && (r.Total.Value == nil || r.Available.Value == nil || *r.Available.Value <= *r.Total.Value)
}

type HealthReport struct {
	SchemaVersion     int             `json:"schemaVersion"`
	Sequence          int64           `json:"sequence"`
	AgentInstanceID   string          `json:"agentInstanceId"`
	SampledAt         int64           `json:"sampledAt"` // host Unix milliseconds; not an RTT clock
	Scope             string          `json:"scope"`     // host or container
	CPUPercent        Reading         `json:"cpuPercent"`
	Memory            CapacityReading `json:"memory"`
	Disk              CapacityReading `json:"disk"`
	HostUptimeSeconds Reading         `json:"hostUptimeSeconds"`
}

func (HealthReport) kind() string  { return "health_report" }
func (h HealthReport) valid() bool { return h.Validate() == nil }
func (h HealthReport) Validate() error {
	if h.SchemaVersion != Version || !ValidCounter(h.Sequence) || !ValidID(h.AgentInstanceID) || !ValidCounter(h.SampledAt) || h.Scope != "host" && h.Scope != "container" || !h.CPUPercent.valid(100) || !h.Memory.valid() || !h.Disk.valid() || !h.HostUptimeSeconds.valid(100*366*24*60*60) {
		return ErrInvalid
	}
	return nil
}

// Aggregate holds min/max/sum over valid readings, in the named field's units.
// Empty aggregates have zero min/max/sum and a separate missing count. Consumers
// derive a mean only when Valid > 0. Percentiles must use RTTMillis samples.
type Aggregate struct {
	Valid   int     `json:"valid"`
	Missing int     `json:"missing"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Sum     float64 `json:"sum"`
}

func (a Aggregate) valid(max float64) bool {
	if a.Valid < 0 || a.Valid > 120 || a.Missing < 0 || a.Missing > 120 || a.Valid+a.Missing > 120 || !finiteRange(a.Min, max) || !finiteRange(a.Max, max) || math.IsNaN(a.Sum) || math.IsInf(a.Sum, 0) || a.Sum < 0 {
		return false
	}
	if a.Valid == 0 {
		return a.Min == 0 && a.Max == 0 && a.Sum == 0
	}
	// Summation and multiplication can round differently for valid samples.
	lower, upper := a.Min*float64(a.Valid), a.Max*float64(a.Valid)
	epsilon := math.Max(1, upper) * 1e-12
	return a.Min <= a.Max && a.Sum >= lower-epsilon && a.Sum <= upper+epsilon
}

// HealthSummary is a controller-built, cumulative minute snapshot, not a wire
// report accepted from the agent. At most 64 RTT samples per minute are kept;
// combined percentiles use those samples, never averages of percentiles.
type HealthSummary struct {
	SchemaVersion        int       `json:"schemaVersion"`
	Scope                string    `json:"scope"`
	CPUPercent           Aggregate `json:"cpuPercent"`
	MemoryTotalBytes     Aggregate `json:"memoryTotalBytes"`
	MemoryAvailableBytes Aggregate `json:"memoryAvailableBytes"`
	DiskTotalBytes       Aggregate `json:"diskTotalBytes"`
	DiskAvailableBytes   Aggregate `json:"diskAvailableBytes"`
	HostUptimeSeconds    Aggregate `json:"hostUptimeSeconds"`
	RTTMillis            []float64 `json:"rttMillis"`
	MissedProbes         int       `json:"missedProbes"`
	Reconnects           int       `json:"reconnects"`
	InputBytes           int64     `json:"inputBytes"`
	OutputBytes          int64     `json:"outputBytes"`
}

func (h HealthSummary) Validate() error {
	if h.SchemaVersion != Version || h.Scope != "host" && h.Scope != "container" || !h.CPUPercent.valid(100) || !h.MemoryTotalBytes.valid(float64(MaxCounter)) || !h.MemoryAvailableBytes.valid(float64(MaxCounter)) || !h.DiskTotalBytes.valid(float64(MaxCounter)) || !h.DiskAvailableBytes.valid(float64(MaxCounter)) || !h.HostUptimeSeconds.valid(100*366*24*60*60) || len(h.RTTMillis) > 64 || h.MissedProbes < 0 || h.MissedProbes > 120 || h.Reconnects < 0 || h.Reconnects > 120 || h.InputBytes < 0 || h.InputBytes > MaxCounter || h.OutputBytes < 0 || h.OutputBytes > MaxCounter {
		return ErrInvalid
	}
	for _, v := range h.RTTMillis {
		if !finiteRange(v, 5000) {
			return ErrInvalid
		}
	}
	return nil
}
