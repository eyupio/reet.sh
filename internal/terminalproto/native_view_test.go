package terminalproto

import "testing"

func TestNativeViewProtocolIsSeparateFromProcessSwitching(t *testing.T) {
	if SupportsNativeView(SessionModeFeature) || !SupportsNativeView(ACPFeature+", "+NativeViewFeature) {
		t.Fatal("native views must be negotiated independently")
	}
	for _, body := range []Body{NativeView{}, NativeViewResult{Available: true}, NativeViewResult{Reason: "This process has no native terminal."}} {
		wire, err := Encode(Message{Version: Version, Generation: 1, RequestID: "request-native-view-0001", SessionID: "session-native-view-0001", Body: body})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Decode(wire); err != nil {
			t.Fatal(err)
		}
	}
	if (NativeViewResult{Available: true, Reason: "unavailable"}).valid() {
		t.Fatal("contradictory availability accepted")
	}
}
