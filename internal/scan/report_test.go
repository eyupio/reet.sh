package scan

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunReportDryRunNeedsNoServer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r, err := RunReport(ReportOptions{Dir: t.TempDir(), Host: "test-machine", DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !r.DryRun || r.Sent || r.Host != "test-machine" {
		t.Fatalf("report = %+v", r)
	}
	var out bytes.Buffer
	RenderResult(&out, r)
	if !strings.Contains(out.String(), "dry run, nothing sent") {
		t.Fatalf("dry run summary = %q", out.String())
	}
}

func TestRunReportRefusesAnInteractiveLessScanWithoutServer(t *testing.T) {
	_, err := RunReport(ReportOptions{Dir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "server URL required") {
		t.Fatalf("err = %v", err)
	}
}

func TestRenderFindingsAndResult(t *testing.T) {
	var out bytes.Buffer
	RenderFindings(&out, []Found{
		{Name: "alpha", Source: "owner/repo", Agent: "claude"},
		{Name: "beta", Agent: ""},
	})
	got := out.String()
	for _, want := range []string{"alpha", "owner/repo", "claude", "beta", "(unknown source)", "(lock file)"} {
		if !strings.Contains(got, want) {
			t.Errorf("findings missing %q:\n%s", want, got)
		}
	}
	out.Reset()
	RenderResult(&out, Report{Sent: true, Host: "laptop", Created: 2, Ignored: 1, Pruned: 3, Imported: 6, User: "sam", Errors: []string{"one skipped"}})
	got = out.String()
	for _, want := range []string{"2 new, 1 ignored, 3 pruned on laptop", "reported as sam", "reported 6 skills", "skipped: one skipped"} {
		if !strings.Contains(got, want) {
			t.Errorf("result missing %q:\n%s", want, got)
		}
	}
}
