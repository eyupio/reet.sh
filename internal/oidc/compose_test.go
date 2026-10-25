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
	// A secret is better read from a file than from the environment, and the
	// documentation offers it: the container has to be able to receive it.
	if !regexp.MustCompile(`(?m)^\s+REET_OIDC_CLIENT_SECRET_FILE:`).Match(compose) {
		t.Error("docker-compose.yml does not pass REET_OIDC_CLIENT_SECRET_FILE to the container")
	}
}
