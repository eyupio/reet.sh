package terminalproto

import "testing"

func TestParseFeatures(t *testing.T) {
	set := ParseFeatures(" acp-v1 ,, acp-model-v1 ,  ")
	if !set.Has(ACPFeature) || !set.Has(ACPModelFeature) {
		t.Fatal("parsed features missing a named feature")
	}
	if set.Has("") || set.Has("acp-history-v1") {
		t.Fatal("parsed features gained a feature")
	}
	if set.Raw() != " acp-v1 ,, acp-model-v1 ,  " {
		t.Fatal("raw header changed", set.Raw())
	}
	if ParseFeatures("").Has(ACPFeature) {
		t.Fatal("an empty header negotiated a feature")
	}
}
