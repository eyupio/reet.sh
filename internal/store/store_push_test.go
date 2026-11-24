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
	if _, err := s.DeletePushSubscription(bob.ID, phone, 0); err != nil {
		t.Fatal(err)
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

func TestPushSubscriptionTheServiceDroppedIsKeptAsDeadThenReplaced(t *testing.T) {
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
	const old, fresh = "https://fcm.googleapis.com/fcm/send/old", "https://fcm.googleapis.com/fcm/send/fresh"
	phone, err := s.SavePushSubscription(ann.ID, old, "Chrome on Android", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPushDead(old); err != nil {
		t.Fatal(err)
	}
	list, _ := s.PushSubscriptionsFor(ann.ID)
	if len(list) != 1 || list[0].DeadAt == 0 {
		t.Fatalf("a dead subscription was not kept and marked: %+v", list)
	}
	// Nobody is woken for a dead device alone.
	if users, _ := s.PushUsers(); len(users) != 0 {
		t.Fatalf("push users with only a dead device: %v", users)
	}
	// Bob cannot take over Ann's record.
	if _, err := s.ReplacePushSubscription(bob.ID, phone.ID, "", fresh); !errors.Is(err, ErrPushNotFound) {
		t.Fatalf("Bob replaced Ann's device: %v", err)
	}
	// Replaced by id, the record keeps its name and choices and is live again.
	got, err := s.ReplacePushSubscription(ann.ID, phone.ID, "", fresh)
	if err != nil || got.ID != phone.ID || got.Endpoint != fresh || got.DeadAt != 0 || got.Label != "Chrome on Android" || got.Ended {
		t.Fatalf("replaced %+v, %v", got, err)
	}
	// Replaced by its old address, as the service worker does.
	const third = "https://fcm.googleapis.com/fcm/send/third"
	if got, err := s.ReplacePushSubscription(ann.ID, 0, fresh, third); err != nil || got.ID != phone.ID || got.Endpoint != third {
		t.Fatalf("replaced by address %+v, %v", got, err)
	}
	if _, err := s.ReplacePushSubscription(ann.ID, 0, "", third); !errors.Is(err, ErrPushNotFound) {
		t.Fatalf("a replacement naming nothing: %v", err)
	}
	// Saving the same address again also brings a dead record back.
	_ = s.MarkPushDead(third)
	if again, err := s.SavePushSubscription(ann.ID, third, "Chrome on Android", true, false); err != nil || again.DeadAt != 0 {
		t.Fatalf("saved again %+v, %v", again, err)
	}
}
