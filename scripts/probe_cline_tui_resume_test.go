package scripts

import (
	"os/exec"
	"testing"
)

func TestClineTUIProbeVerdicts(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3 is required for the optional Cline diagnostic probe")
	}
	if output, err := exec.Command(python, "-B", "test_probe_cline_tui_resume.py").CombinedOutput(); err != nil {
		t.Fatalf("Cline probe verdict regression: %v\n%s", err, output)
	}
}
