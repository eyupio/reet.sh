package setup

import "fmt"

// ValidatePortableSettings applies the existing setup export policy to a
// library upload. Unlike encrypted snapshots, a portable item cannot opt into
// credentials, hooks, project state or unlisted fields. Bytes are not rewritten.
func ValidatePortableSettings(agent, format, base string, data []byte) error {
	if !supportedFormat(format, agent) {
		return fmt.Errorf("unsupported settings format")
	}
	policy := settingsExportPolicy(agent, base)
	if policy.Selection == "unsupported" {
		return fmt.Errorf("unsupported settings application")
	}
	object, err := parseSource(data, format)
	if err != nil {
		return err
	}
	selected, _ := selectSettingsContent(object, policy, false)
	if len(object) == 0 || len(selected) != len(object) {
		return fmt.Errorf("settings contain fields excluded by the portable export policy")
	}
	return nil
}
