package clientconfig

import (
	"os"
	"strings"
	"testing"
)

func TestEnrolmentAsksForACodeAndSavesNothing(t *testing.T) {
	f := newAuthFixture(t, true, "reet_enrol_typed")
	got, err := ResolveEnrolment("", "", "", "https://reet.example", nil, &f.out)
	if err != nil || got.Code != "reet_enrol_typed" || got.Token != "" || f.prompts != 1 {
		t.Fatalf("prompt: %+v %v (%d prompts)", got, err, f.prompts)
	}
	if !strings.Contains(f.out.String(), "Enrolment code") || strings.Contains(f.out.String(), "Personal API token") {
		t.Fatalf("prompt wording: %q", f.out.String())
	}
	if path, _ := TokenPath(); fileExists(path) {
		t.Fatal("enrolment saved a token")
	}
}

func TestEnrolmentCodeFromAFlagOrTheEnvironmentWins(t *testing.T) {
	f := newAuthFixture(t, true)
	t.Setenv("REET_TOKEN", "personal")
	if got, err := ResolveEnrolment("reet_enrol_flag", "", "", "", nil, &f.out); err != nil || got.Code != "reet_enrol_flag" || got.Token != "" {
		t.Fatalf("flag: %+v %v", got, err)
	}
	t.Setenv("REET_ENROL_CODE", "reet_enrol_env")
	if got, err := ResolveEnrolment("", "", "", "", nil, &f.out); err != nil || got.Code != "reet_enrol_env" || got.Token != "" {
		t.Fatalf("environment: %+v %v", got, err)
	}
	if f.prompts != 0 {
		t.Fatal("prompted although a code was given")
	}
	if _, err := ResolveEnrolment("not-a-code", "", "", "", nil, &f.out); err == nil {
		t.Fatal("accepted a value that is not an enrolment code")
	}
}

// The personal token still enrols, from any place it was read before, but
// never by prompting, and with a warning.
func TestEnrolmentStillTakesAPersonalTokenWithAWarning(t *testing.T) {
	f := newAuthFixture(t, true)
	for _, c := range []struct {
		flag, env, saved string
		source           Source
	}{
		{flag: "from-flag", source: SourceFlag},
		{env: "from-env", source: SourceEnv},
		{saved: "from-saved", source: SourceSaved},
	} {
		t.Setenv("REET_TOKEN", c.env)
		path, _ := TokenPath()
		_ = os.Remove(path)
		if c.saved != "" {
			if _, err := SaveToken(c.saved); err != nil {
				t.Fatal(err)
			}
		}
		f.out.Reset()
		got, err := ResolveEnrolment("", c.flag, "", "", nil, &f.out)
		if err != nil || got.Code != "" || got.Token != c.flag+c.env+c.saved || got.Source != c.source {
			t.Fatalf("%+v: %+v %v", c, got, err)
		}
		if !strings.Contains(f.out.String(), "personal API token") || !strings.Contains(f.out.String(), "enrolment code") {
			t.Fatalf("no warning: %q", f.out.String())
		}
	}
	if f.prompts != 0 {
		t.Fatal("prompted although a token was found")
	}
}

// With nothing given and no terminal, enrolment goes ahead without a
// credential, for an instance that needs none.
func TestEnrolmentWithoutTerminalOrCredentialSendsNone(t *testing.T) {
	f := newAuthFixture(t, false)
	got, err := ResolveEnrolment("", "", "", "", nil, &f.out)
	if err != nil || got.Code != "" || got.Token != "" || f.prompts != 0 {
		t.Fatalf("%+v %v", got, err)
	}
}

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }
