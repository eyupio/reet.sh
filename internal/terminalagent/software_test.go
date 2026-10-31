//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func softwareApplyRequest() *terminalproto.SoftwareApply {
	return &terminalproto.SoftwareApply{Tool: "opencode", Action: terminalproto.SoftwareActionUpdate, ExpectedState: terminalproto.SoftwareInstalled, ExpectedVersion: "1.18.0", ExpectedManaged: true}
}

func appliedSoftwareResult(t *testing.T) terminalproto.SoftwareApplyResult {
	t.Helper()
	entry := terminalproto.SoftwareEntry{
		Tool: "opencode", State: terminalproto.SoftwareInstalled, Version: "1.19.5", Manager: "reet",
		Package: "opencode-ai", Path: "/home/me/.reet/software/opencode/versions/1.19.5/opencode",
		Platform: "linux", Arch: "amd64", Source: "verified_tarball", Evidence: "verified_sha256",
		Managed: true, Operations: []terminalproto.SoftwareOperation{terminalproto.SoftwarePlanUpdate, terminalproto.SoftwarePlanRemove},
		CheckedAt: 1,
	}
	return terminalproto.SoftwareApplyResult{Tool: "opencode", Action: terminalproto.SoftwareActionUpdate, State: terminalproto.SoftwareApplyApplied, Version: "1.19.5", Previous: "1.18.0", Entry: &entry}
}

func TestSoftwareApplyNeedsNegotiation(t *testing.T) {
	original := applySoftware
	applySoftware = func(context.Context, string, terminalproto.SoftwareApply, func(string) (string, error), ...terminalhost.SoftwareSource) terminalproto.SoftwareApplyResult {
		t.Fatal("ran without software-apply-v1")
		return terminalproto.SoftwareApplyResult{}
	}
	t.Cleanup(func() { applySoftware = original })
	handle, got := modelsConnector(t, terminalproto.SoftwareFeature, nil)
	if err := handle(t.Context(), message("software-apply-00001", softwareApplyRequest())); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatalf("error = %v", err)
	}
	nothingSent(t, got)
}

func TestSoftwareApplyRunsAsideAndAnswers(t *testing.T) {
	original := applySoftware
	called := make(chan terminalproto.SoftwareApply, 1)
	applySoftware = func(_ context.Context, home string, req terminalproto.SoftwareApply, _ func(string) (string, error), sources ...terminalhost.SoftwareSource) terminalproto.SoftwareApplyResult {
		if home == "" {
			t.Error("apply had no home")
		}
		if len(sources) != 1 || sources[0].Client == nil || !strings.HasSuffix(sources[0].Path, agentsoftware.OpenCodeLinuxX64.ArchiveSHA256) {
			t.Errorf("mirror source = %+v", sources)
		}
		called <- req
		return appliedSoftwareResult(t)
	}
	t.Cleanup(func() { applySoftware = original })
	waitSoftwareIdle(t)
	handle, got := modelsConnector(t, terminalproto.SoftwareApplyFeature, nil)
	start := time.Now()
	if err := handle(t.Context(), message("software-apply-00002", softwareApplyRequest())); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("the dispatcher waited for the download")
	}
	select {
	case req := <-called:
		if req.ExpectedVersion != "1.18.0" || !req.ExpectedManaged {
			t.Fatalf("request lost evidence: %+v", req)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("apply did not run")
	}
	select {
	case reply := <-got:
		result, ok := reply.Body.(terminalproto.SoftwareApplyResult)
		if !ok || result.State != terminalproto.SoftwareApplyApplied || result.Version != "1.19.5" {
			t.Fatalf("reply = %+v", reply.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no reply")
	}
}

func TestSoftwareApplyConsentOff(t *testing.T) {
	original := applySoftware
	applySoftware = func(context.Context, string, terminalproto.SoftwareApply, func(string) (string, error), ...terminalhost.SoftwareSource) terminalproto.SoftwareApplyResult {
		t.Fatal("ran with consent off")
		return terminalproto.SoftwareApplyResult{}
	}
	t.Cleanup(func() { applySoftware = original })
	handle, got := modelsConnector(t, terminalproto.SoftwareApplyFeature, func(opts *terminalhost.ManagerOptions) {
		opts.Consent = func() error { return errors.New("terminal access off") }
	})
	if err := handle(t.Context(), message("software-apply-00003", softwareApplyRequest())); err != nil {
		t.Fatal(err)
	}
	select {
	case reply := <-got:
		result, ok := reply.Body.(terminalproto.SoftwareApplyResult)
		if !ok || result.State != terminalproto.SoftwareApplyRefused || result.Reason != terminalproto.SoftwareApplyOff {
			t.Fatalf("reply = %+v", reply.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no reply")
	}
}

func waitSoftwareIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for softwareRunning.Load() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("software work still running")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
