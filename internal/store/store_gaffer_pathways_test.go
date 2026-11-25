package store

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func pathway(n int, name string) GafferPathway {
	return GafferPathway{ID: hexID('p', n) + fmt.Sprint(n), Name: name, Description: "d", Enabled: true,
		Stages: []GafferPathwayStage{{Kind: "code", Tier: "standard", Skills: []string{"s1"}},
			{Kind: "custom", Name: "Lint", Prompt: "Lint it.", Agent: &GafferStageAgent{Tool: "claude", Model: "m"}}}}
}

func TestPathwayRoundTrip(t *testing.T) {
	s, uid, _ := templateStore(t)
	saved, err := s.SaveGafferPathwayFor(uid, pathway(1, "Ship"), 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GafferPathwayFor(uid, saved.ID)
	if err != nil || !reflect.DeepEqual(got, saved) {
		t.Fatalf("got %+v, %v; want %+v", got, err, saved)
	}
	if list, err := s.GafferPathwaysFor(uid); err != nil || len(list) != 1 || !reflect.DeepEqual(list[0], saved) {
		t.Fatalf("list %+v, %v", list, err)
	}
}

func TestPathwayNameIsUniqueWithoutCase(t *testing.T) {
	s, uid, other := templateStore(t)
	if _, err := s.SaveGafferPathwayFor(uid, pathway(1, "Ship"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveGafferPathwayFor(uid, pathway(2, "ship"), 0); !errors.Is(err, ErrGafferPathwayName) {
		t.Fatalf("got %v", err)
	}
	if _, err := s.SaveGafferPathwayFor(other, pathway(3, "ship"), 0); err != nil {
		t.Fatal(err)
	}
}

func TestPathwayStaleSaveIsRefused(t *testing.T) {
	s, uid, _ := templateStore(t)
	p, err := s.SaveGafferPathwayFor(uid, pathway(1, "Ship"), 0)
	if err != nil {
		t.Fatal(err)
	}
	p.Name = "First"
	first, err := s.SaveGafferPathwayFor(uid, p, p.UpdatedAt)
	if err != nil || first.UpdatedAt <= p.UpdatedAt {
		t.Fatalf("%+v, %v", first, err)
	}
	p.Name = "Second"
	if _, err = s.SaveGafferPathwayFor(uid, p, p.UpdatedAt); !errors.Is(err, ErrGafferPathwayChanged) {
		t.Fatalf("got %v", err)
	}
	if got, _ := s.GafferPathwayFor(uid, p.ID); got.Name != "First" {
		t.Fatalf("name %q", got.Name)
	}
}

func TestPathwayLimit(t *testing.T) {
	s, uid, _ := templateStore(t)
	for i := 0; i < MaxGafferPathways; i++ {
		if _, err := s.SaveGafferPathwayFor(uid, pathway(i, fmt.Sprint("P", i)), 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SaveGafferPathwayFor(uid, pathway(99, "Extra"), 0); !errors.Is(err, ErrGafferPathwayLimit) {
		t.Fatalf("got %v", err)
	}
}

func TestPathwayOfAnotherAccountIsNotFound(t *testing.T) {
	s, uid, other := templateStore(t)
	p, err := s.SaveGafferPathwayFor(uid, pathway(1, "Ship"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.GafferPathwayFor(other, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read: %v", err)
	}
	if _, err = s.SaveGafferPathwayFor(other, p, p.UpdatedAt); !errors.Is(err, ErrNotFound) {
		t.Fatalf("save: %v", err)
	}
	if err = s.DeleteGafferPathwayFor(other, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete: %v", err)
	}
	if err = s.DeleteGafferPathwayFor(uid, p.ID); err != nil {
		t.Fatal(err)
	}
}
