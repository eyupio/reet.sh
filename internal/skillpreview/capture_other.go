//go:build !linux && !darwin

package skillpreview

// Observe never claims presence or absence where executable-mode and no-follow
// reads are not verified.
func Observe(Roots, string) Capture {
	return Capture{SchemaVersion: Version, Adapter: Adapter, RootID: Unavailable, RootOwnership: "unknown", Presence: "unknown", Issues: []Issue{{Path: ".", Code: "platform_unsupported"}}}
}
