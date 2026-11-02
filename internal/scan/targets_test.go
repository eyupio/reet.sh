package scan

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListTargetChoicesPutsThisHostFirst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"canWrite":true,"targets":[
			{"mode":"additive","target":{"id":"t1","name":"claude","host":"other"}},
			{"mode":"bidirectional","target":{"id":"t2","name":"codex","host":"ns1","project":"/srv/app"}}]}`))
	}))
	defer srv.Close()
	choices, err := ListTargetChoices(srv.URL, "", "ns1")
	if err != nil || len(choices) != 2 || choices[0].ID != "t2" || choices[1].ID != "t1" {
		t.Fatalf("choices = %+v err = %v", choices, err)
	}
	if choices[0].Label != "codex (ns1, /srv/app, bidirectional)" {
		t.Fatalf("label = %q", choices[0].Label)
	}
}

func TestListTargetChoicesRefusesReadOnlyAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"canWrite":false,"targets":[]}`))
	}))
	defer srv.Close()
	if _, err := ListTargetChoices(srv.URL, "", ""); err == nil {
		t.Fatal("expected error")
	}
}
