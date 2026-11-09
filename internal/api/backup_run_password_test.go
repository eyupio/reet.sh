package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/backup"
)

const runPath = "/api/v2/admin/backups/run"

func copiesIn(t *testing.T, tg target) []string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(tg.dir, "backups", "instance-*"))
	return m
}

// runRequest sends the run route with a raw body (or none) and a JSON type.
func runRequest(t *testing.T, tg target, token string, body *string) reply {
	t.Helper()
	var rd io.Reader = http.NoBody
	if body != nil {
		rd = strings.NewReader(*body)
	}
	req, _ := http.NewRequest("POST", tg.url+runPath, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	return doImport(t, req)
}

func str(s string) *string { return &s }

func openedWith(t *testing.T, tg target, r reply, password string) (backup.Manifest, error) {
	t.Helper()
	id, _ := r.v["id"].(string)
	raw, err := os.ReadFile(filepath.Join(tg.dir, "backups", id+".reetbackup"))
	if err != nil {
		matches := copiesIn(t, tg)
		t.Fatalf("copy %q not found (%v); copies: %v", id, err, matches)
	}
	m, err := backup.ManifestOf(raw)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = backup.OpenArchive(raw, password, importKey)
	return m, err
}

func TestTheRunRouteSealsOneCopyWithAPasswordOnTheRequest(t *testing.T) {
	tg := newTarget(t, false)
	pw := "a password for the team's copy"
	r := runRequest(t, tg, "owner-token", str(`{"password":"`+pw+`"}`))
	if r.code != 200 {
		t.Fatalf("%d %s", r.code, r.text)
	}
	m, err := openedWith(t, tg, r, pw)
	if err != nil || m.Protection != backup.ProtectionPassword {
		t.Fatalf("protection %q, open with the password: %v", m.Protection, err)
	}
	if _, err := openedWith(t, tg, r, ""); err == nil {
		t.Fatal("the copy opened with the controller's key alone")
	}
}

// No body, an empty object and a null password all behave as the route did.
func TestTheRunRouteWithNoPasswordBehavesAsBefore(t *testing.T) {
	for name, body := range map[string]*string{"no body": nil, "empty object": str(`{}`), "null password": str(`{"password":null}`)} {
		t.Run(name, func(t *testing.T) {
			tg := newTarget(t, false)
			r := runRequest(t, tg, "owner-token", body)
			if r.code != 200 {
				t.Fatalf("%d %s", r.code, r.text)
			}
			m, err := openedWith(t, tg, r, "")
			if err != nil || m.Protection == backup.ProtectionPassword {
				t.Fatalf("a copy with no password was protected with one: %q %v", m.Protection, err)
			}
		})
	}
}

func TestTheRunRouteRefusesABadPasswordAndTakesNothing(t *testing.T) {
	tg := newTarget(t, false)
	before := len(copiesIn(t, tg))
	for _, body := range []string{`{"password":""}`, `{"password":"   "}`, `{"password":"short"}`, `{"password":"11 chars!!!"}`} {
		r := runRequest(t, tg, "owner-token", str(body))
		if r.code != http.StatusBadRequest || !strings.Contains(r.text, "a copy password needs at least 12 characters") {
			t.Fatalf("%s: %d %s", body, r.code, r.text)
		}
	}
	// Malformed bodies are refused too, rather than quietly sealing a copy
	// with the stored password the caller meant to replace.
	for _, body := range []string{`{"passwod":"a long enough password"}`, `{"password":12345678901234}`, `not json`, `[]`} {
		if r := runRequest(t, tg, "owner-token", str(body)); r.code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, r.code, r.text)
		}
	}
	if after := len(copiesIn(t, tg)); after != before {
		t.Fatalf("a refused request took %d copies", after-before)
	}
}

func TestTheRunRouteIsTheOwnersEvenWithAPassword(t *testing.T) {
	tg := newTarget(t, true)
	before := len(copiesIn(t, tg))
	r := runRequest(t, tg, "member-token", str(`{"password":"a password for the team's copy"}`))
	if r.code != http.StatusForbidden {
		t.Fatalf("a member: %d %s", r.code, r.text)
	}
	if len(copiesIn(t, tg)) != before {
		t.Fatal("a refused request took a copy")
	}
}

// The password is in no log line, no activity entry, no response and no row.
func TestTheRequestPasswordIsInNoLogNoActivityEntryNoResponseAndNoRow(t *testing.T) {
	tg := newTarget(t, false)
	secret := "very-distinctive-password-77"
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	r := runRequest(t, tg, "owner-token", str(`{"password":"`+secret+`"}`))
	if r.code != 200 {
		t.Fatalf("%d %s", r.code, r.text)
	}
	// A refused one too, because errors are where passwords leak.
	bad := "tiny-"
	runRequest(t, tg, "owner-token", str(`{"password":"`+bad+`"}`))

	if strings.Contains(r.text, secret) || strings.Contains(logs.String(), secret) || strings.Contains(logs.String(), bad+`"`) {
		t.Fatalf("the password is in the response or a log line:\n%s", logs.String())
	}
	evs, err := tg.st.ListEvents(100, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range evs {
		enc, _ := json.Marshal(e)
		if strings.Contains(string(enc), secret) {
			t.Fatalf("an activity entry holds the password: %s", enc)
		}
		if e.Action == "backup-run" && strings.Contains(e.Detail, `"passwordOnRequest":true`) {
			found = true
			if e.At == 0 {
				t.Fatalf("the entry has no time: %+v", e)
			}
		}
	}
	if !found {
		t.Fatal("the activity log does not say a copy was taken with a password given on the request")
	}
	// Who: the entry belongs to the owner who made the request.
	owner, _, _ := tg.st.OwnerUser()
	mine, err := tg.st.ListEventsFor(owner.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	attributed := false
	for _, e := range mine {
		attributed = attributed || (e.Action == "backup-run" && strings.Contains(e.Detail, `"passwordOnRequest":true`))
	}
	if !attributed {
		t.Fatal("the entry is not attributed to the owner who took the copy")
	}
	snap := filepath.Join(tg.dir, "snapshot.db")
	if err := tg.st.VacuumInto(t.Context(), snap); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(snap); bytes.Contains(raw, []byte(secret)) {
		t.Fatal("a database row holds the password")
	}
}

// features names the capability exactly when the route accepts the body.
func TestVersionListsCopyPasswordRequestAndTheRouteAcceptsTheBody(t *testing.T) {
	tg := newTarget(t, false)
	resp, err := http.Get(tg.url + "/api/version")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v struct {
		Features []string `json:"features"`
	}
	json.NewDecoder(resp.Body).Decode(&v)
	listed := false
	for _, f := range v.Features {
		listed = listed || f == "copy-password-request"
	}
	accepts := runRequest(t, tg, "owner-token", str(`{"password":"short"}`)).code == http.StatusBadRequest
	if !listed || !accepts {
		t.Fatalf("listed %v, route reads the body %v", listed, accepts)
	}
}
