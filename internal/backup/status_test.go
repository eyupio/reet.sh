package backup

import (
	"context"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func TestStatusUsesSavedDestinationsAndSchedule(t *testing.T) {
	fake := newFakeS3(t)
	for _, owner := range []string{"instance", "user"} {
		t.Run(owner, func(t *testing.T) {
			svc, _, user := testService(t, nil)
			ownerID := int64(0)
			if owner == "user" {
				ownerID = user.ID
			}
			// This is what startup records when destinations and schedules are
			// configured later through the administration page.
			svc.cfg.Findings = []Finding{
				{Severity: "warning", Code: "backup.no_offsite", Message: "stale off-site warning"},
				{Severity: "info", Code: "backup.instance_off", Message: "stale schedule warning"},
				{Severity: "info", Code: "backup.key_beside_data", Message: "keep the key elsewhere"},
			}
			if _, err := svc.CreateRemote(ownerID, RemoteInput{
				Name: ptr("cloudflare"), Endpoint: ptr(fake.server.URL), Bucket: ptr("bucket"),
				AccessKey: ptr("key"), SecretKey: ptr("secret"), PathStyle: rawBool(true),
			}); err != nil {
				t.Fatal(err)
			}
			for _, destination := range []string{store.DestinationBoth, store.DestinationRemote} {
				if _, err := svc.SaveSettings(ownerID, SettingsInput{EverySeconds: 86400, Keep: 31, Destination: destination}); err != nil {
					t.Fatal(err)
				}
				status, err := svc.Status(context.Background(), ownerID)
				if err != nil {
					t.Fatal(err)
				}
				for _, f := range status.Findings {
					if f.Code == "backup.no_offsite" || f.Code == "backup.instance_off" {
						t.Fatalf("%s: stale finding with usable saved destination and enabled schedule: %+v", destination, f)
					}
					if ownerID != 0 && f.Code == "backup.key_beside_data" {
						t.Fatal("instance key guidance disclosed to user")
					}
				}
			}
			// Disabling the destination must bring back exactly one actionable
			// remote-only error, rather than a startup warning or duplicates.
			if _, err := svc.UpdateRemote(ownerID, "cloudflare", RemoteInput{Enabled: new(false)}); err != nil {
				t.Fatal(err)
			}
			status, err := svc.Status(context.Background(), ownerID)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, f := range status.Findings {
				if f.Code == "backup.no_offsite" {
					count++
					if f.Severity != "error" {
						t.Fatalf("remote-only without a usable remote must be an error: %+v", f)
					}
				}
			}
			if count != 1 {
				t.Fatalf("want one off-site finding, got %d: %+v", count, status.Findings)
			}
		})
	}
}

func TestStatusEnvironmentRemoteUsesEffectiveConnection(t *testing.T) {
	svc, _, _ := testService(t, newFakeS3(t))
	if _, err := svc.SaveSettings(0, SettingsInput{EverySeconds: 86400, Keep: 31, Destination: store.DestinationRemote}); err != nil {
		t.Fatal(err)
	}
	status, err := svc.Status(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Remotes) != 1 || status.Remotes[0].Endpoint != svc.cfg.S3.Endpoint || status.Remotes[0].Bucket != svc.cfg.S3.Bucket || !anyUsableRemote(status.Remotes) {
		t.Fatalf("environment remote lost effective connection fields: %+v", status.Remotes)
	}
	for _, f := range status.Findings {
		if f.Code == "backup.no_offsite" {
			t.Fatalf("usable environment remote was reported missing: %+v", f)
		}
	}
}

func TestInstanceRemoteOnlyStatusHasNoLocalCopies(t *testing.T) {
	svc, _, _ := testService(t, newFakeS3(t))
	if _, err := svc.SaveSettings(0, SettingsInput{EverySeconds: 86400, Keep: 31, Destination: store.DestinationRemote}); err != nil {
		t.Fatal(err)
	}
	item, err := svc.RunInstance(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	status, err := svc.Status(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if item.Local || len(status.Items) != 1 || status.Items[0].Local || len(status.Items[0].Remotes) != 1 {
		t.Fatalf("remote-only instance should have one off-site copy and no local copy: %+v, %+v", item, status.Items)
	}
	local, err := svc.LocalItems(0)
	if err != nil || len(local) != 0 {
		t.Fatalf("remote-only instance retained local data: %+v, %v", local, err)
	}
}
