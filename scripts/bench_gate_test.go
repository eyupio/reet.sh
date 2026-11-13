package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gate runs bench-gate.py with a stand-in benchstat that answers with the
// comparison given, in benchstat's own CSV shape, and returns what the gate
// said, whether it failed, and what it wrote to --failed-to.
func gate(t *testing.T, rows string) (output string, failed bool, names string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	dir := t.TempDir()
	csv := "goos: linux\n,base.txt,,new.txt,,,\n,sec/op,CI,sec/op,CI,vs base,P\n" + rows + "geomean,0.002,,0.002,,+1.00%,\n\n,base.txt,,new.txt,,,\n,B/op,CI,B/op,CI,vs base,P\n"
	stub := "#!/bin/sh\ncat <<'CSV'\n" + csv + "CSV\n"
	if err := os.WriteFile(filepath.Join(dir, "benchstat"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	samples := "BenchmarkListForCompact-2 300 3000000 ns/op\nBenchmarkListSkillsAPI/compact-2 300 2000000 ns/op\nBenchmarkCommunity-2 300 1000000 ns/op\n"
	for _, name := range []string{"base.txt", "new.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(samples), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	list := filepath.Join(dir, "failed.txt")
	// A list left by an earlier comparison must never name a benchmark now.
	if err := os.WriteFile(list, []byte("Stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, "bench-gate.py", filepath.Join(dir, "base.txt"), filepath.Join(dir, "new.txt"), "15", "--failed-to", list)
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	written, _ := os.ReadFile(list)
	return string(out), err != nil, string(written)
}

// The job measures again only what the gate names, so the names have to be ones
// `go test -bench` can be given: the benchmark itself, without the processor
// count or the sub-benchmark.
func TestTheBenchGateNamesWhatFailedSoItCanBeMeasuredAgain(t *testing.T) {
	out, failed, names := gate(t, "ListForCompact-2,0.003,1%,0.0036,1%,+19.63%,p=0.035 n=6+7\n"+
		"ListSkillsAPI/compact-2,0.002,1%,0.0026,1%,+30.00%,p=0.001 n=8\n"+
		"Community-2,0.001,1%,0.001,1%,~,p=0.800 n=8\n")
	if !failed {
		t.Fatalf("two regressions over the threshold passed:\n%s", out)
	}
	if names != "ListForCompact\nListSkillsAPI\n" {
		t.Fatalf("named %q", names)
	}
}

func TestTheBenchGateNamesNothingWhenNothingRegressed(t *testing.T) {
	for name, rows := range map[string]string{
		"under the threshold": "ListForCompact-2,0.003,1%,0.0033,1%,+10.00%,p=0.001 n=8\n",
		"not significant":     "ListForCompact-2,0.003,1%,0.0036,1%,+19.63%,p=0.200 n=8\n",
		"faster":              "ListForCompact-2,0.003,1%,0.002,1%,-33.00%,p=0.001 n=8\n",
	} {
		out, failed, names := gate(t, rows)
		if failed || names != "" {
			t.Fatalf("%s: failed=%v names=%q\n%s", name, failed, names, out)
		}
	}
}

// A runner whose processor count changes between samples gives one benchmark
// two names, and benchstat then compares fewer samples than were taken. The
// run that prompted this compared six against seven of eight.
func TestTheBenchmarksRunWithAFixedProcessorCount(t *testing.T) {
	script, err := os.ReadFile("bench-compare.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), `-test.cpu "$bench_cpu"`) {
		t.Fatal("bench-compare.sh lets the runner choose the processor count")
	}
}

// One comparison over the threshold is not yet a regression: the job measures
// what failed again and fails on the second answer.
func TestTheBenchmarkJobMeasuresAFailureAgainBeforeItFails(t *testing.T) {
	workflow, err := os.ReadFile("../.github/workflows/benchmarks.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	first := strings.Index(text, `--failed-to "$out/failed.txt"`)
	again := strings.Index(text, `bench-compare.sh base pr "$out/confirm"`)
	second := strings.Index(text, `bench-gate.py "$out/confirm/base.txt" "$out/confirm/new.txt" 15`)
	if first < 0 || again < first || second < again {
		t.Fatalf("the job does not compare, measure again and compare again in that order (%d, %d, %d)", first, again, second)
	}
}
