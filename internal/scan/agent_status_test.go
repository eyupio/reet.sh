package scan

import (
	"strings"
	"testing"
)

func TestAgentStatusProblemsNameTheCause(t *testing.T) {
	if got := (AgentStatus{Connected: true}).Problems(); got != nil {
		t.Fatalf("a working agent reported problems: %v", got)
	}
	refused := strings.Join(AgentStatus{CycleError: "controller 403 Forbidden: machine enrollment is missing or revoked"}.Problems(), "\n")
	if !strings.Contains(refused, "Sync is failing: controller 403") || !strings.Contains(refused, "reet agent enroll") {
		t.Fatalf("refusal lacks its cause or next step:\n%s", refused)
	}
	network := strings.Join(AgentStatus{ConfigPath: "/x/agent.json", PrivateError: "dial tcp: lookup reet.example: no such host"}.Problems(), "\n")
	if !strings.Contains(network, "Private connection is failing") || !strings.Contains(network, "/x/agent.json") {
		t.Fatalf("network failure lacks its cause or next step:\n%s", network)
	}
	// A stale private error must not be shown once the connection is back.
	if got := (AgentStatus{Connected: true, PrivateError: "old"}).Problems(); got != nil {
		t.Fatalf("stale private error shown: %v", got)
	}
}
