package api

import (
	"encoding/json"
	"reflect"
	"testing"
)

func withBuiltFeatures(t *testing.T, names ...string) {
	t.Helper()
	old := builtFeatures
	builtFeatures = names
	t.Cleanup(func() { builtFeatures = old })
}

// A controller with no built parts answers as one built before the list
// existed: no "features" field, not an empty one.
func TestVersionOmitsFeaturesWhenNothingIsBuilt(t *testing.T) {
	withBuiltFeatures(t)
	_, body := do(t, newSrv(t, deadFetcher(t), ""), "GET", "/api/version", "")
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	if _, has := doc["features"]; has {
		t.Fatalf("an empty list was sent: %s", body)
	}
}

func TestVersionListsExactlyTheBuiltFeatures(t *testing.T) {
	withBuiltFeatures(t, "fence", "copy-origin")
	_, body := do(t, newSrv(t, deadFetcher(t), ""), "GET", "/api/version", "")
	var doc struct {
		Features []string `json:"features"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Features, []string{"fence", "copy-origin"}) {
		t.Fatalf("features: %s", body)
	}
}

// What this build really holds is what is advertised: the copy's source and
// fenced markers and the live-source guard are all here, and nothing else yet.
func TestTheCompiledFeatureListIsExactlyWhatIsBuilt(t *testing.T) {
	if !reflect.DeepEqual(builtFeatures, []string{"copy-origin", "fence"}) {
		t.Fatalf("this build advertises %v", builtFeatures)
	}
}
