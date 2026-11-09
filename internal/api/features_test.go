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

// What this build really holds is what is advertised, and only names the
// contract reserves.
func TestTheCompiledFeatureListUsesOnlyReservedNames(t *testing.T) {
	reserved := map[string]bool{"fence": true, "copy-origin": true, "restore-public-url": true}
	for _, f := range builtFeatures {
		if !reserved[f] {
			t.Fatalf("%q is not a name the contract reserves", f)
		}
	}
}
