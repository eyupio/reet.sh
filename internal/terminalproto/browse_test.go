package terminalproto

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func browseResult() BrowseResult {
	return BrowseResult{
		Path: "/home/u/code", Home: "/home/u", Parent: "/home/u",
		Repo: "none", Entries: []BrowseEntry{{Name: "api", Dir: true, RepoRoot: true}, {Name: ".cache", Dir: true, Hidden: true}},
		Scanned: 2,
	}
}

func TestBrowseFeatureToken(t *testing.T) {
	for _, value := range []string{"", "launch-v1", "browse-v10", "prefix-browse-v1"} {
		if SupportsBrowse(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsBrowse("launch-v1, browse-v1") || BrowseFeature != "browse-v1" {
		t.Fatal("feature not recognized")
	}
}

func TestBrowseRoundTripIsStrict(t *testing.T) {
	for _, body := range []Body{
		Browse{Path: "/home/u", After: "m", Filter: "ap", Hidden: true}, Browse{}, browseResult(),
		BrowseResult{Reason: BrowseNotFound},
	} {
		encoded, err := Encode(testMessage(body))
		if err != nil {
			t.Fatal(body.kind(), err)
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatal(body.kind(), err)
		}
		again, err := Encode(decoded)
		if err != nil || !bytes.Equal(encoded, again) {
			t.Fatal("round trip changed", string(encoded), string(again), err)
		}
	}
	encoded, _ := Encode(testMessage(Browse{Path: "/home/u"}))
	if _, err := Decode(bytes.Replace(encoded, []byte(`"path":`), []byte(`"depth":2,"path":`), 1)); err == nil {
		t.Fatal("unknown field accepted")
	}
	encoded, _ = Encode(testMessage(browseResult()))
	if _, err := Decode(bytes.Replace(encoded, []byte(`"home":`), []byte(`"token":"x","home":`), 1)); err == nil {
		t.Fatal("unknown result field accepted")
	}
	if _, err := Decode(bytes.Replace(encoded, []byte(`"name":"api"`), []byte(`"name":"api","size":1`), 1)); err == nil {
		t.Fatal("unknown entry field accepted")
	}
}

func TestBrowseRequestLimits(t *testing.T) {
	valid := []Browse{{}, {Path: "/"}, {Path: "/home/u"}, {After: strings.Repeat("a", 255)}, {Filter: strings.Repeat("a", 64)}, {Path: "/home/u", Hidden: true}}
	for _, b := range valid {
		if !b.valid() {
			t.Error("rejected", b)
		}
	}
	invalid := map[string]Browse{
		"after 256 bytes":   {After: strings.Repeat("a", 256)},
		"filter 65 bytes":   {Filter: strings.Repeat("a", 65)},
		"relative path":     {Path: "home/u"},
		"dot path":          {Path: "."},
		"control in path":   {Path: "/home/u\n"},
		"control in after":  {After: "a\x00"},
		"control in filter": {Filter: "a\tb"},
		"invalid utf8":      {Filter: "\xff"},
		"long path":         {Path: "/" + strings.Repeat("a", MaxDirBytes)},
	}
	for name, b := range invalid {
		if b.valid() {
			t.Error("accepted", name)
		}
		if _, err := Encode(testMessage(b)); err == nil {
			t.Error("encoded", name)
		}
	}
}

func TestBrowseResultLimits(t *testing.T) {
	entries := func(n int, name string) []BrowseEntry {
		out := make([]BrowseEntry, n)
		for i := range out {
			out[i] = BrowseEntry{Name: name, Dir: true}
		}
		return out
	}
	with := func(f func(*BrowseResult)) BrowseResult {
		r := browseResult()
		f(&r)
		return r
	}
	if !with(func(r *BrowseResult) { r.Entries = entries(MaxBrowseEntries, strings.Repeat("a", 20)) }).valid() {
		t.Error("200 entries refused")
	}
	invalid := map[string]BrowseResult{
		"201 entries":         with(func(r *BrowseResult) { r.Entries = entries(MaxBrowseEntries+1, "a") }),
		"256 byte name":       with(func(r *BrowseResult) { r.Entries = entries(1, strings.Repeat("a", 256)) }),
		"newline in name":     with(func(r *BrowseResult) { r.Entries = entries(1, "a\nb") }),
		"empty name":          with(func(r *BrowseResult) { r.Entries = entries(1, "") }),
		"slash in name":       with(func(r *BrowseResult) { r.Entries = entries(1, "a/b") }),
		"dot name":            with(func(r *BrowseResult) { r.Entries = entries(1, "..") }),
		"inside, no repo top": with(func(r *BrowseResult) { r.Repo = "inside" }),
		"none with repo top":  with(func(r *BrowseResult) { r.RepoTop = "/home/u" }),
		"unknown repo":        with(func(r *BrowseResult) { r.Repo = "yes" }),
		"empty repo":          with(func(r *BrowseResult) { r.Repo = "" }),
		"relative path":       with(func(r *BrowseResult) { r.Path = "code" }),
		"relative home":       with(func(r *BrowseResult) { r.Home = "u" }),
		"relative parent":     with(func(r *BrowseResult) { r.Parent = "u" }),
		"relative repo top":   with(func(r *BrowseResult) { r.Repo, r.RepoTop = "root", "u" }),
		"unknown reason":      {Reason: "boom"},
		"reason with entries": {Reason: BrowseDenied, Entries: entries(1, "a")},
		"reason with paths":   {Reason: BrowseDenied, Path: "/home/u"},
		"reason with repo":    {Reason: BrowseDenied, Repo: "none"},
		"reason with more":    {Reason: BrowseDenied, More: true},
		"negative scanned":    with(func(r *BrowseResult) { r.Scanned = -1 }),
		"negative omitted":    with(func(r *BrowseResult) { r.Omitted = -1 }),
		"over reply budget":   with(func(r *BrowseResult) { r.Entries = entries(MaxBrowseEntries, strings.Repeat("a", 255)) }),
	}
	for name, r := range invalid {
		if r.valid() {
			t.Error("accepted", name)
		}
	}
	for _, reason := range []BrowseReason{BrowseNotFound, BrowseNotDir, BrowseOutsideHome, BrowseDenied, BrowseUnreadable, BrowseConflict} {
		if !(BrowseResult{Reason: reason}).valid() {
			t.Error("refused", reason)
		}
	}
	ok := with(func(r *BrowseResult) {
		r.Repo, r.RepoTop, r.More, r.Partial, r.Omitted = "inside", "/home/u", true, true, 5
	})
	if !ok.valid() {
		t.Error("refused inside a repository")
	}
	// A full page of ordinary names fits the reply budget; the agent stops a
	// page early (More) when longer names would not.
	full := with(func(r *BrowseResult) {
		r.Entries = make([]BrowseEntry, MaxBrowseEntries)
		for i := range r.Entries {
			r.Entries[i] = BrowseEntry{Name: strings.Repeat("a", 16), Dir: true, RepoRoot: true}
		}
	})
	data, err := json.Marshal(full)
	if err != nil || len(data) > MaxBrowseReplyBytes || !full.valid() {
		t.Fatal(len(data), MaxBrowseReplyBytes, err)
	}
	if _, err := Encode(testMessage(full)); err != nil {
		t.Fatal(err)
	}
}

func TestBrowseIsComparable(t *testing.T) {
	a, b := Browse{Path: "/home/u", After: "x", Filter: "y", Hidden: true}, Browse{Path: "/home/u", After: "x", Filter: "y", Hidden: true}
	if a != b {
		t.Fatal("equal requests differ")
	}
	b.Hidden = false
	if a == b {
		t.Fatal("difference not seen")
	}
	if (BrowseEntry{Name: "a", Dir: true}) != (BrowseEntry{Name: "a", Dir: true}) {
		t.Fatal("entries not comparable")
	}
}

func TestBrowseOmitsEmptyKeys(t *testing.T) {
	data, _ := json.Marshal(Browse{})
	if string(data) != `{}` {
		t.Fatal(string(data))
	}
}

func TestCapabilitiesBrowseRequiresTerminalControl(t *testing.T) {
	c := testCapabilities()
	legacy, _ := json.Marshal(c)
	if bytes.Contains(legacy, []byte("browse")) {
		t.Fatal("new field sent to old server", string(legacy))
	}
	c.TerminalControl = false
	c.Browse = true
	if c.Validate() == nil {
		t.Fatal("browse without terminal control accepted")
	}
	c.TerminalControl = true
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(c)
	if !bytes.Contains(data, []byte(`"browse":true`)) {
		t.Fatal(string(data))
	}
}
