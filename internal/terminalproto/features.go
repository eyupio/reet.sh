package terminalproto

import "strings"

// FeatureSet is a negotiated feature header parsed once. The connector reads
// the header on every control message and session change, so keeping the set
// avoids splitting the same string for each gate.
type FeatureSet struct {
	raw string
	set map[string]struct{}
}

// ParseFeatures parses a comma-separated feature header. Names are trimmed and
// empty names are dropped, matching the Supports helpers.
func ParseFeatures(raw string) FeatureSet {
	f := FeatureSet{raw: raw}
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if f.set == nil {
			f.set = make(map[string]struct{})
		}
		f.set[name] = struct{}{}
	}
	return f
}

// Has reports whether the peer negotiated the named feature.
func (f FeatureSet) Has(feature string) bool {
	_, ok := f.set[feature]
	return ok
}

// Raw is the header as received, for helpers that still take the string.
func (f FeatureSet) Raw() string { return f.raw }
