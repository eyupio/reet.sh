package terminalproto

import (
	"encoding/json"
	"strings"
)

// ACPCommandsFeature negotiates the "commands" live control: the slash
// commands a chat's agent last announced. Without it a controller never asks
// and a connector never answers.
const ACPCommandsFeature = "acp-commands-v1"

const (
	MaxACPCommands                = 64
	MaxACPCommandNameBytes        = 64
	MaxACPCommandDescriptionBytes = 300
	MaxACPCommandHintBytes        = 120
	// MaxACPCommandsBytes bounds one encoded list. Escaping can make each
	// byte of text six, so the count and field limits alone do not keep a
	// reply inside MaxControlBytes.
	MaxACPCommandsBytes = 48 << 10
)

func SupportsACPCommands(features string) bool {
	for _, f := range strings.Split(features, ",") {
		if strings.TrimSpace(f) == ACPCommandsFeature {
			return true
		}
	}
	return false
}

// ACPCommand is one slash command an agent announced. Name has no leading
// slash. All three fields are the agent's untrusted text, shown, never run.
type ACPCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Hint        string `json:"hint"`
}

func (c ACPCommand) Valid() bool {
	if c.Name == "" || len(c.Name) > MaxACPCommandNameBytes {
		return false
	}
	for _, r := range c.Name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ':' || r == '_' || r == '-') {
			return false
		}
	}
	return validChatText(c.Description, MaxACPCommandDescriptionBytes) && validChatText(c.Hint, MaxACPCommandHintBytes)
}

// ValidACPCommands checks a whole list: the count, each entry, unique names
// and the encoded size. A nil list is an empty one.
func ValidACPCommands(list []ACPCommand) bool {
	if len(list) > MaxACPCommands {
		return false
	}
	seen := map[string]bool{}
	for _, c := range list {
		if !c.Valid() || seen[c.Name] {
			return false
		}
		seen[c.Name] = true
	}
	b, err := json.Marshal(list)
	return err == nil && len(b) <= MaxACPCommandsBytes
}
