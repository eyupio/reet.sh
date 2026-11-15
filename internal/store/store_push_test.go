package store

import (
	"errors"
	"fmt"
	"testing"
)

func TestPushSubscriptionsBelongToOnePersonEach(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ann, err := s.CreateUser("ann@example.test", "Ann", "owner", "x")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := s.CreateUser("bob@example.test", "Bob", "member", "x")
	if err != nil {
		t.Fatal(err)
	}
	const phone = "https://fcm.googleapis.com/fcm/send/phone"
	first, err := s.SavePushSubscription(ann.ID, phone, "Chrome on Android", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Waiting || !first.Ended || first.Label != "Chrome on Android" || first.CreatedAt == 0 {
		t.Fatalf("saved %+v", first)
	}
	// The same browser again changes its choices, and adds nothing.
	again, err := s.SavePushSubscription(ann.ID, phone, "Chrome on Android", true, false)
	if err != nil || again.ID != first.ID || again.Ended {
		t.Fatalf("saved again %+v, %v", again, err)
	}
	if users, _ := s.PushUsers(); len(users) != 1 || users[0] != ann.ID {
		t.Fatalf("push users %v", users)
	}
	// Bob cannot remove Ann's phone.
	if ok, err := s.DeletePushSubscription(bob.ID, phone, first.ID); err != nil || ok {
		t.Fatalf("Bob removed Ann's phone: %v %v", ok, err)
	}
	// Bob signs in on that phone: it is his now, and Ann has none.
	if _, err := s.SavePushSubscription(bob.ID, phone, "Chrome on Android", true, true); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.PushSubscriptionsFor(ann.ID); len(list) != 0 {
		t.Fatalf("Ann still has %d", len(list))
	}
	if list, _ := s.PushSubscriptionsFor(bob.ID); len(list) != 1 || list[0].Endpoint != phone {
		t.Fatalf("Bob has %+v", list)
	}
	if err := s.MarkPushDelivered(phone); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.PushSubscriptionsFor(bob.ID); list[0].LastOKAt == 0 {
		t.Fatal("a delivery was not recorded")
	}
	if err := s.DropPushEndpoint(phone); err != nil {
		t.Fatal(err)
	}
	if users, _ := s.PushUsers(); len(users) != 0 {
		t.Fatalf("push users after the endpoint went: %v", users)
	}
	for i := range MaxPushSubscriptions {
		if _, err := s.SavePushSubscription(ann.ID, fmt.Sprintf("https://fcm.googleapis.com/fcm/send/%d", i), "", true, true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SavePushSubscription(ann.ID, "https://fcm.googleapis.com/fcm/send/one-more", "", true, true); !errors.Is(err, ErrPushLimit) {
		t.Fatalf("a device past the limit: %v", err)
	}
}
