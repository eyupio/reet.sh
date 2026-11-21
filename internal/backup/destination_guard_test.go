package backup

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/objectstore"
	"github.com/eyupio/reet.sh/internal/store"
)

// A member's destination may not be on this machine or a private network;
// the instance's own may.
func TestMemberDestinationMustBePublic(t *testing.T) {
	fake := newFakeS3(t)
	svc, st, user := testService(t, nil)
	svc.cfg.AllowLocal = false
	in := func(endpoint string) RemoteInput {
		return RemoteInput{Name: ptr("mine"), Endpoint: ptr(endpoint), Bucket: ptr("bucket"),
			AccessKey: ptr("key"), SecretKey: ptr("secret"), PathStyle: rawBool(true)}
	}
	for _, endpoint := range []string{
		fake.server.URL, "http://localhost:9000", "https://127.0.0.1", "https://[::1]:9000",
		"https://169.254.169.254", "https://10.0.0.8:9000", "https://192.168.1.20", "https://100.64.0.1", "https://[fd00::1]",
	} {
		if _, err := svc.CreateRemote(user.ID, in(endpoint)); !errors.Is(err, objectstore.ErrAddress) {
			t.Fatalf("a member's destination at %s: %v, want ErrAddress", endpoint, err)
		}
		if _, err := svc.TestRemoteDraft(context.Background(), user.ID, in(endpoint)); !errors.Is(err, objectstore.ErrAddress) {
			t.Fatalf("a member's test of %s: %v, want ErrAddress", endpoint, err)
		}
	}
	if _, err := svc.CreateRemote(user.ID, in("https://s3.example.com")); err != nil {
		t.Fatalf("a member's public destination: %v", err)
	}
	// The instance's own destination on this machine is saved and works.
	if _, err := svc.CreateRemote(0, in(fake.server.URL)); err != nil {
		t.Fatalf("the instance's destination on this machine: %v", err)
	}
	if steps, err := svc.TestRemote(context.Background(), 0, "mine"); err != nil || len(steps) != 3 {
		t.Fatalf("the instance's test: %+v %v", steps, err)
	}

	// One stored before the rule, or under a name that resolves inward, is
	// refused as it is dialled, and the server is never asked.
	before := len(fake.keys())
	sealed, err := svc.cfg.Sealer.SealSecret("secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBackupRemote(store.BackupRemote{OwnerID: user.ID, Name: "inward", Endpoint: fake.server.URL,
		Bucket: "bucket", AccessKey: "key", SecretSealed: sealed, PathStyle: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.TestRemote(context.Background(), user.ID, "inward"); !errors.Is(err, objectstore.ErrAddress) {
		t.Fatalf("a stored member destination on this machine: %v, want ErrAddress", err)
	}
	addSkill(t, st, user.ID, "owner/repo/skill")
	item, err := svc.RunUser(context.Background(), user.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Remotes) != 0 || len(fake.keys()) != before {
		t.Fatalf("a member's copy reached this machine's store: %+v %v", item.Remotes, fake.keys())
	}

	// The operator may allow it.
	svc.cfg.AllowLocal = true
	if _, err := svc.TestRemote(context.Background(), user.ID, "inward"); err != nil {
		t.Fatalf("with local destinations allowed: %v", err)
	}
}

// A destination that accepts a connection and then says nothing must not
// hold the backup, and every backup queued behind it, for good.
func TestUploadToASilentDestinationTimesOut(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop); ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { <-stop; conn.Close() }()
		}
	}()
	old := uploadTimeout
	uploadTimeout = func(int) time.Duration { return 300 * time.Millisecond }
	t.Cleanup(func() { uploadTimeout = old })

	svc, _, _ := testService(t, nil)
	if _, err := svc.CreateRemote(0, RemoteInput{Name: ptr("silent"), Endpoint: ptr("http://" + ln.Addr().String()),
		Bucket: ptr("bucket"), AccessKey: ptr("key"), SecretKey: ptr("secret"), PathStyle: rawBool(true),
		// Keeping every copy leaves pruning, which has its own deadline, out of this.
		Keep: ptrInt(0)}); err != nil {
		t.Fatal(err)
	}
	type result struct {
		item Item
		err  error
	}
	done := make(chan result, 1)
	started := time.Now()
	go func() {
		item, err := svc.RunInstance(context.Background(), "test")
		done <- result{item, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatal(r.err)
		}
		if len(r.item.Remotes) != 0 || !r.item.Local {
			t.Fatalf("item = %+v", r.item)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("the backup was still waiting on the destination after %s", time.Since(started))
	}
	views, err := svc.remoteViews(0)
	if err != nil || len(views) != 1 || !strings.Contains(views[0].LastError, "deadline") {
		t.Fatalf("the destination's last error: %+v %v", views, err)
	}
}

func TestUploadTimeoutGrowsWithTheCopy(t *testing.T) {
	small, large := uploadTimeout(1<<10), uploadTimeout(512<<20)
	if small < 5*time.Minute || large <= small || large > 2*time.Hour {
		t.Fatalf("timeouts: %s for 1 KiB, %s for 512 MiB", small, large)
	}
}

func TestRenamingADestinationKeepsItWhenTheNameIsTaken(t *testing.T) {
	svc, st, _ := testService(t, nil)
	for _, name := range []string{"first", "second"} {
		if _, err := svc.CreateRemote(0, RemoteInput{Name: ptr(name), Endpoint: ptr("https://" + name + ".example.com"),
			Bucket: ptr("bucket"), AccessKey: ptr("key"), SecretKey: ptr("secret")}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.UpdateRemote(0, "first", RemoteInput{Name: ptr("second")}); err == nil {
		t.Fatal("a rename onto a taken name was accepted")
	}
	kept, ok, err := st.BackupRemoteByName(0, "first")
	if err != nil || !ok || kept.Endpoint != "https://first.example.com" || kept.SecretSealed == "" {
		t.Fatalf("the destination after a refused rename: %+v %v %v", kept, ok, err)
	}
	view, err := svc.UpdateRemote(0, "first", RemoteInput{Name: ptr("third")})
	if err != nil || view.Name != "third" {
		t.Fatalf("rename: %+v %v", view, err)
	}
	if _, ok, _ := st.BackupRemoteByName(0, "first"); ok {
		t.Fatal("the old name is still there")
	}
	moved, ok, err := st.BackupRemoteByName(0, "third")
	if err != nil || !ok || moved.Endpoint != "https://first.example.com" || moved.SecretSealed != kept.SecretSealed {
		t.Fatalf("the renamed destination: %+v %v %v", moved, ok, err)
	}
	all, err := st.BackupRemotes(0)
	if err != nil || len(all) != 2 {
		t.Fatalf("destinations: %+v %v", all, err)
	}
}
