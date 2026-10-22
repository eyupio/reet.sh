package setup

import "strings"

func summariseSettings(object map[string]any) *SettingsSummary {
	_, model := object["model"]
	_, defaultModel := object["defaultModel"]
	_, hooks := object["hooks"]
	_, permissions := object["permissions"]
	_, packages := object["packages"]
	summary := &SettingsSummary{
		ModelConfigured: model || defaultModel, HooksConfigured: hooks,
		PermissionsConfigured: permissions, PackagesConfigured: packages,
		EnvironmentFields: objectSize(object["env"]),
	}
	if fields, ok := object["env"].(map[string]any); ok {
		for _, value := range fields {
			if pureReference(value) {
				summary.ReferenceFields++
			} else {
				summary.LiteralFields++
			}
		}
	}
	return summary
}

func summariseMCP(name string, value any, agent string) MCPSummary {
	m := MCPSummary{Name: name, Transport: "unknown", State: "unsupported"}
	object, ok := value.(map[string]any)
	if !ok {
		m.Issue = "invalid_mcp_declaration"
		return m
	}
	command, commandOK := object["command"].(string)
	url, urlOK := object["url"].(string)
	m.CommandConfigured = commandOK && command != ""
	m.URLConfigured = urlOK && url != ""
	args, argsOK := object["args"].([]any)
	if argsOK {
		m.ArgumentCount = len(args)
	}
	typeName, _ := object["type"].(string)
	envKey := "env"
	if agent == "opencode" {
		envKey = "environment"
		commands, ok := object["command"].([]any)
		m.CommandConfigured = ok && len(commands) > 0 && nonemptyString(commands[0])
		if m.CommandConfigured {
			m.ArgumentCount = len(commands) - 1
		}
		switch typeName {
		case "local":
			m.Transport = "stdio"
		case "remote":
			m.Transport = "remote"
		}
	} else {
		switch typeName {
		case "stdio", "sse", "http":
			m.Transport = typeName
		case "":
			if m.CommandConfigured {
				m.Transport = "stdio"
			} else if m.URLConfigured {
				m.Transport = "remote"
			}
		}
		// Gemini distinguishes HTTP transport with httpUrl.
		if agent == "gemini" && nonemptyString(object["httpUrl"]) {
			m.URLConfigured, m.Transport = true, "http"
		}
	}
	m.EnvironmentFields = objectSize(object[envKey])
	m.HeaderFields = objectSize(object["headers"])
	for _, key := range []string{envKey, "headers"} {
		if fields, ok := object[key].(map[string]any); ok {
			for _, value := range fields {
				if pureReference(value) {
					m.ReferenceFields++
				} else {
					m.LiteralFields++
				}
			}
		}
	}
	valid := m.Transport == "stdio" && m.CommandConfigured && !m.URLConfigured || m.Transport != "stdio" && m.Transport != "unknown" && m.URLConfigured && !m.CommandConfigured
	for _, key := range []string{"url", "type", "httpUrl"} {
		if value, exists := object[key]; exists {
			_, ok := value.(string)
			valid = valid && ok
		}
	}
	if value, exists := object["command"]; exists && agent != "opencode" {
		_, ok := value.(string)
		valid = valid && ok
	}
	for _, key := range []string{envKey, "headers"} {
		if value, exists := object[key]; exists {
			fields, ok := value.(map[string]any)
			valid = valid && ok
			for _, field := range fields {
				_, ok := field.(string)
				valid = valid && ok
			}
		}
	}
	if value, exists := object["args"]; exists {
		values, ok := value.([]any)
		valid = valid && ok && stringList(values)
	}
	if agent == "opencode" && m.CommandConfigured {
		valid = valid && stringList(object["command"].([]any))
	}
	if valid {
		m.State = "summarised"
	} else {
		m.Issue = "unsupported_or_incomplete_mcp_declaration"
	}
	return m
}

func objectSize(value any) int {
	if object, ok := value.(map[string]any); ok {
		return len(object)
	}
	return 0
}

func nonemptyString(value any) bool {
	s, ok := value.(string)
	return ok && s != ""
}

func stringList(values []any) bool {
	for _, value := range values {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}

// Only unambiguous whole-value references are counted as references. Templates,
// defaults and mixed literal/reference values remain conservatively literal.
func pureReference(value any) bool {
	s, ok := value.(string)
	if !ok {
		return false
	}
	var name string
	switch {
	case strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}"):
		name = s[2 : len(s)-1]
	case strings.HasPrefix(s, "{env:") && strings.HasSuffix(s, "}"):
		name = s[5 : len(s)-1]
	default:
		return false
	}
	if name == "" {
		return false
	}
	for i, c := range name {
		if !(c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
