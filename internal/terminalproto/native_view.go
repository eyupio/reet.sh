package terminalproto

import "strings"

// NativeViewFeature is a presentation change: the agent keeps running and a
// disposable native terminal client attaches to its existing conversation.
const NativeViewFeature = "native-chat-view-v1"

func SupportsNativeView(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == NativeViewFeature {
			return true
		}
	}
	return false
}

// NativeView asks the connector whether this exact running chat has a native
// terminal. No address, executable, password or command crosses the connection.
type NativeView struct{}

func (NativeView) kind() string { return "native_view" }
func (NativeView) valid() bool  { return true }

type NativeViewResult struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

func (NativeViewResult) kind() string { return "native_view_result" }
func (r NativeViewResult) valid() bool {
	return validChatText(r.Reason, 500) && (!r.Available || r.Reason == "")
}
