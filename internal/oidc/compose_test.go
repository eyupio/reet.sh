package oidc

import (
	"os"
	"regexp"
	"testing"
)

// A setting the server reads but the supplied Compose file does not hand to
// the container cannot be set by anyone who deploys from it: REET_OIDC=off in
// .env would change nothing. Every variable the configuration reads is
// listed under the service's environment.
func TestEveryVariableTheConfigurationReadsReachesTheContainer(t *testing.T) {
	src, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`"(REET_OIDC[A-Z_]*)"`).FindAllStringSubmatch(string(src), -1) {
		name := m[1]
		if !regexp.MustCompile(`(?m)^\s+` + name + `:`).Match(compose) {
			t.Errorf("docker-compose.yml does not pass %s to the container", name)
		}
	}
}
