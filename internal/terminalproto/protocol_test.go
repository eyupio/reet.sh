package terminalproto

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

const testID = "0123456789abcdef0123456789abcdef"

func testCapabilities() Capabilities {
	return Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "tester", TerminalEnabled: true, HealthEnabled: true, Readiness: Ready}
}
func testHealth() HealthReport {
	missing := Reading{Reason: CollectionFailed}
	return HealthReport{Version, 1, testID, 1, "host", Reading{Reason: InitialSample}, CapacityReading{missing, missing}, CapacityReading{missing, missing}, missing}
}
func testMessage(body Body) Message {
	m := Message{Version: Version, RequestID: testID, Generation: 1, Body: body}
	switch body.kind() {
	case "create", "end", "session_changed":
		m.SessionID = testID
	case "attach", "resize", "renew_attachment", "detach", "history":
		m.SessionID = testID
		m.AttachmentID = testID
		m.AttachmentGeneration = 1
	}
	return m
}
func TestRoundTripEveryOperation(t *testing.T) {
	for _, body := range []Body{
		Hello{testID, testCapabilities()}, Reconcile{Complete: true, Sessions: []SessionSnapshot{{ID: testID, Title: "Shell", State: Running}}},
		Create{Title: "Shell", Cols: 80, Rows: 24}, Attach{Cols: 80, Rows: 24, LeaseSeconds: 60}, Resize{100, 30}, RenewAttachment{60}, Detach{}, End{}, History{Lines: 2000},
		Probe{1, testID}, ProbeResult{1, testID}, testHealth(), Result{Code: OK}, SessionChanged{State: Running},
	} {
		t.Run(body.kind(), func(t *testing.T) {
			m := testMessage(body)
			data, err := Encode(m)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Decode(data)
			if err != nil {
				t.Fatal(err)
			}
			again, err := Encode(got)
			if err != nil || !bytes.Equal(data, again) {
				t.Fatalf("round trip changed: %s / %s (%v)", data, again, err)
			}
		})
	}
}
func TestRejectMalformedAndAmbiguousControl(t *testing.T) {
	valid, _ := Encode(testMessage(Create{Title: "Shell", Cols: 80, Rows: 24}))
	cases := map[string][]byte{
		"version":          bytes.Replace(valid, []byte(`"version":1`), []byte(`"version":2`), 1),
		"operation":        bytes.Replace(valid, []byte(`"type":"create"`), []byte(`"type":"exec"`), 1),
		"unknown envelope": bytes.Replace(valid, []byte(`"version":1`), []byte(`"version":1,"token":"secret"`), 1),
		"unknown body":     bytes.Replace(valid, []byte(`"cols":80`), []byte(`"cols":80,"command":"secret"`), 1),
		"duplicate":        bytes.Replace(valid, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1),
		"case alias":       bytes.Replace(valid, []byte(`"cols":80`), []byte(`"cols":80,"COLS":80`), 1),
		"null body":        bytes.Replace(valid, []byte(`{"title":"Shell","cols":80,"rows":24}`), []byte(`null`), 1),
		"bad id":           bytes.Replace(valid, []byte(testID), []byte(`../../etc/passwd`), 1),
		"trailing":         append(append([]byte{}, valid...), []byte(` {}`)...),
		"oversized":        bytes.Repeat([]byte(" "), MaxControlBytes+1),
		"utf8":             append(append([]byte{}, valid...), 0xff),
		"null":             []byte(`null`), "array": []byte(`[]`), "empty": nil,
		"deep": []byte(strings.Repeat("[", 34) + strings.Repeat("]", 34)),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(data); err != ErrInvalid {
				t.Fatalf("wanted sanitized error, got %v", err)
			}
		})
	}
	// Padding exactly to the frame limit is accepted; one extra byte is not.
	boundary := append(append([]byte{}, valid...), bytes.Repeat([]byte(" "), MaxControlBytes-len(valid))...)
	if _, err := Decode(boundary); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(append(boundary, ' ')); err == nil {
		t.Fatal("oversized accepted")
	}
}
func TestValidationBoundariesAndBinding(t *testing.T) {
	if !ValidTitle(strings.Repeat("界", 80)) || ValidTitle(strings.Repeat("界", 81)) {
		t.Fatal("titles must count characters")
	}
	for _, title := range []string{"", "  ", "new\nline", "escape\x1b[0m", "hidden\u202e", string([]byte{255})} {
		if ValidTitle(title) {
			t.Fatalf("accepted title %q", title)
		}
	}
	for _, size := range [][2]int{{2, 1}, {500, 300}} {
		if !ValidDimensions(size[0], size[1]) {
			t.Fatal(size)
		}
	}
	for _, size := range [][2]int{{1, 1}, {501, 1}, {80, 0}, {80, 301}} {
		if ValidDimensions(size[0], size[1]) {
			t.Fatal(size)
		}
	}
	for _, n := range []int{1, MaxBinaryBytes} {
		if ValidateBinary(make([]byte, n)) != nil {
			t.Fatal(n)
		}
	}
	for _, n := range []int{0, MaxBinaryBytes + 1} {
		if ValidateBinary(make([]byte, n)) == nil {
			t.Fatal(n)
		}
	}
	for _, mutate := range []func(*Message){
		func(m *Message) { m.Generation = 0 }, func(m *Message) { m.Generation = MaxCounter + 1 },
		func(m *Message) { m.SessionID = "" }, func(m *Message) { m.AttachmentID = "" }, func(m *Message) { m.AttachmentGeneration = 0 },
		func(m *Message) { m.Body = Attach{Cols: 80, Rows: 24, LeaseSeconds: 61} }, func(m *Message) { m.Body = (*Attach)(nil) },
	} {
		m := testMessage(Attach{Cols: 80, Rows: 24, LeaseSeconds: 60})
		mutate(&m)
		if _, err := Encode(m); err == nil {
			t.Fatal("invalid binding accepted")
		}
	}
	m := testMessage(Probe{1, testID})
	m.SessionID = testID
	if m.Validate() == nil {
		t.Fatal("probe accepted session binding")
	}
	r := Reconcile{Complete: true, Sessions: []SessionSnapshot{{testID, "Shell", Running, nil}, {testID, "Other", Running, nil}}}
	if r.valid() {
		t.Fatal("duplicate reconciliation session")
	}
	c := testCapabilities()
	c.TerminalEnabled = false
	if c.Validate() == nil {
		t.Fatal("ready without consent")
	}
	c.Readiness = DisabledLocally
	if c.Validate() != nil {
		t.Fatal("monitoring without terminals should be valid")
	}
}
func TestSessionStateTransitions(t *testing.T) {
	want := map[SessionState][]SessionState{
		Creating: {Creating, Running, EndRequested, Exited, Lost}, Running: {Running, EndRequested, Exited, Lost},
		EndRequested: {EndRequested, Ended, Exited, Lost}, Ended: {Ended}, Exited: {Exited}, Lost: {Lost},
	}
	for from, allowed := range want {
		for to := range want {
			expected := false
			for _, a := range allowed {
				if a == to {
					expected = true
				}
			}
			if CanTransition(from, to) != expected {
				t.Fatalf("%s -> %s", from, to)
			}
		}
	}
	if CanTransition("garbage", Running) || CanTransition(Running, "garbage") {
		t.Fatal("unknown state")
	}
}
func TestHealthNullSemanticsAndBounds(t *testing.T) {
	h := testHealth()
	if h.Validate() != nil {
		t.Fatal("missing readings must be representable")
	}
	for _, mutate := range []func(*HealthReport){
		func(h *HealthReport) { h.CPUPercent = Reading{} },
		func(h *HealthReport) { v := math.NaN(); h.CPUPercent = Reading{Value: &v} },
		func(h *HealthReport) { v := 101.0; h.CPUPercent = Reading{Value: &v} },
		func(h *HealthReport) { v := 0.0; h.CPUPercent = Reading{Value: &v, Reason: InitialSample} },
		func(h *HealthReport) {
			total, avail := 1.0, 2.0
			h.Memory = CapacityReading{Reading{Value: &total}, Reading{Value: &avail}}
		},
		func(h *HealthReport) { h.Sequence = 0 }, func(h *HealthReport) { h.AgentInstanceID = "" }, func(h *HealthReport) { h.Scope = "system?" },
	} {
		bad := h
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid resource accepted")
		}
	}
	summary := HealthSummary{SchemaVersion: Version, Scope: "host", CPUPercent: Aggregate{Valid: 2, Missing: 1, Min: 10, Max: 30, Sum: 40}, RTTMillis: []float64{10, 20}}
	if summary.Validate() != nil {
		t.Fatal("valid aggregate")
	}
	for _, mutate := range []func(*HealthSummary){
		func(s *HealthSummary) { s.CPUPercent.Sum = 61 }, func(s *HealthSummary) { s.CPUPercent.Valid = 0 },
		func(s *HealthSummary) { s.CPUPercent.Valid = math.MaxInt; s.CPUPercent.Missing = math.MaxInt },
		func(s *HealthSummary) { s.RTTMillis = []float64{math.Inf(1)} }, func(s *HealthSummary) { s.RTTMillis = make([]float64, 65) },
		func(s *HealthSummary) { s.InputBytes = -1 }, func(s *HealthSummary) { s.MissedProbes = 121 },
	} {
		bad := summary
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid summary accepted")
		}
	}
	data, _ := json.Marshal(h)
	if !bytes.Contains(data, []byte(`"value":null`)) {
		t.Fatal("missing measurement silently omitted")
	}
}
func FuzzDecode(f *testing.F) {
	valid, _ := Encode(testMessage(Create{Title: "Shell", Cols: 80, Rows: 24}))
	f.Add(valid)
	f.Add([]byte(`{"body":null}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := Decode(data)
		if err != nil {
			if err != ErrInvalid {
				t.Fatal("unsanitized error")
			}
			return
		}
		out, err := Encode(m)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Decode(out); err != nil {
			t.Fatal(err)
		}
	})
}
