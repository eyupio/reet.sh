package systeminstall

import (
	"os/exec"
	"strings"
	"testing"
)

func present(names ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, candidate := range names {
			if candidate == name {
				return "/usr/bin/" + name, nil
			}
		}
		return "", exec.ErrNotFound
	}
}

func TestDetectUsesSudoOnlyWhenNeeded(t *testing.T) {
	node := Request{Linux: []string{"nodejs", "npm"}, Brew: []string{"node"}}
	plan, ok := detect("linux", node, func() int { return 1000 }, present("apt-get", "sudo"))
	if !ok || strings.Join(plan.Command, " ") != "sudo apt-get install -y nodejs npm" {
		t.Fatalf("user install = %+v, %t", plan, ok)
	}
	plan, ok = detect("linux", node, func() int { return 0 }, present("apt-get", "sudo"))
	if !ok || strings.Join(plan.Command, " ") != "apt-get install -y nodejs npm" {
		t.Fatalf("root install = %+v, %t", plan, ok)
	}
	if _, ok := detect("linux", node, func() int { return 1000 }, present("apt-get")); ok {
		t.Fatal("apt-get without root or sudo must not be offered")
	}
	plan, ok = detect("darwin", node, func() int { return 501 }, present("brew"))
	if !ok || strings.Join(plan.Command, " ") != "brew install node" {
		t.Fatalf("brew install = %+v, %t", plan, ok)
	}
	if _, ok := detect("windows", node, func() int { return 0 }, present("apt-get")); ok {
		t.Fatal("windows has no supported system installer")
	}
}

func TestDetectFallsThroughMissingManager(t *testing.T) {
	plan, ok := detect("linux", Request{Linux: []string{"git"}}, func() int { return 1000 }, present("apt-get", "apk", "sudo"))
	if !ok || strings.Join(plan.Command, " ") != "sudo apt-get install -y git" {
		t.Fatalf("first manager = %+v, %t", plan, ok)
	}
	plan, ok = detect("linux", Request{Linux: []string{"git"}}, func() int { return 0 }, present("apk"))
	if !ok || strings.Join(plan.Command, " ") != "apk add git" {
		t.Fatalf("fallback manager = %+v, %t", plan, ok)
	}
}
