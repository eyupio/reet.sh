package setup

import (
	"fmt"

	"github.com/pelletier/go-toml/v2"
)

func supportedFormat(format, agent string) bool {
	return format == "json" || format == "jsonc" && agent == "opencode" || format == "toml" && agent == "codex"
}

func parseSource(data []byte, format string) (map[string]any, error) {
	if len(data) > maxConfigBytes {
		return nil, fmt.Errorf("file too large")
	}
	switch format {
	case "json":
		return parseConfig(data)
	case "jsonc":
		clean, err := stripJSONC(data)
		if err != nil {
			return nil, err
		}
		return parseConfig(clean)
	case "toml":
		var object map[string]any
		if err := toml.Unmarshal(data, &object); err != nil {
			return nil, err
		}
		nodes := 0
		if err := checkConfigTree(object, 0, &nodes); err != nil {
			return nil, err
		}
		return object, nil
	default:
		return nil, fmt.Errorf("unsupported format")
	}
}

// The TOML library bounds value nesting during decoding. Apply the inventory's
// tighter limits to the decoded tree too, including dotted-key/table depth.
func checkConfigTree(value any, depth int, nodes *int) error {
	*nodes++
	if depth > 64 || *nodes > 20000 {
		return fmt.Errorf("complexity limit")
	}
	switch value := value.(type) {
	case map[string]any:
		for _, child := range value {
			if err := checkConfigTree(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := checkConfigTree(child, depth+1, nodes); err != nil {
				return err
			}
		}
	}
	return nil
}

// Remove JSONC comments and trailing commas without touching quoted strings.
// Replacing bytes with spaces preserves token boundaries: 1/*x*/2 stays invalid.
// The strict JSON decoder still checks syntax, duplicates and resource limits.
func stripJSONC(data []byte) ([]byte, error) {
	out := append([]byte(nil), data...)
	quoted, escaped := false, false
	for i := 0; i < len(out); i++ {
		if quoted {
			if escaped {
				escaped = false
			} else if out[i] == '\\' {
				escaped = true
			} else if out[i] == '"' {
				quoted = false
			}
			continue
		}
		if out[i] == '"' {
			quoted = true
			continue
		}
		if out[i] != '/' || i+1 >= len(out) {
			continue
		}
		switch out[i+1] {
		case '/':
			for i < len(out) && out[i] != '\n' && out[i] != '\r' {
				out[i] = ' '
				i++
			}
		case '*':
			out[i], out[i+1] = ' ', ' '
			i += 2
			for i+1 < len(out) && !(out[i] == '*' && out[i+1] == '/') {
				if out[i] != '\n' && out[i] != '\r' {
					out[i] = ' '
				}
				i++
			}
			if i+1 >= len(out) {
				return nil, fmt.Errorf("unterminated comment")
			}
			out[i], out[i+1] = ' ', ' '
			i++
		}
	}
	quoted, escaped = false, false
	var previous byte
	for i := 0; i < len(out); i++ {
		c := out[i]
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
				previous = c
			}
			continue
		}
		if c == '"' {
			quoted = true
		}
		if c == ',' && previous != '[' && previous != '{' && previous != ',' && previous != ':' {
			j := i + 1
			for j < len(out) && jsonSpace(out[j]) {
				j++
			}
			if j < len(out) && (out[j] == ']' || out[j] == '}') {
				out[i] = ' '
			}
		}
		if !jsonSpace(c) {
			previous = c
		}
	}
	return out, nil
}

func jsonSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
