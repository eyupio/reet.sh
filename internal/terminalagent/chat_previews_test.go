//go:build linux || darwin

package terminalagent

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func previewPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 32, 24))
	im.Set(0, 0, color.RGBA{R: 220, A: 255})
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestPreviewSnapshotsSurviveRestartAndRefuseOtherSessions(t *testing.T) {
	home, layout := t.TempDir(), t.TempDir()
	store := newChatPreviewStore(layout, home)
	source := filepath.Join(home, "shot.png")
	data := previewPNG(t)
	os.WriteFile(source, data, 0600)
	ref := store.capture("session-preview0123456789", home, &acpagent.Preview{Path: source})
	if !ref.Valid() || ref.Reason != "" {
		t.Fatalf("capture: %+v", ref)
	}
	os.WriteFile(source, []byte("changed after read"), 0600)
	store = newChatPreviewStore(layout, home)
	part, err := store.read("session-preview0123456789", terminalproto.ACPPreview{ID: ref.ID})
	if err != nil || !bytes.Equal(part.Data, data) {
		t.Fatalf("snapshot changed: %+v %v", part, err)
	}
	if _, err = store.read("session-other0123456789", terminalproto.ACPPreview{ID: ref.ID}); err == nil {
		t.Fatal("cross-session read")
	}
	if _, err = store.read("session-preview0123456789", terminalproto.ACPPreview{ID: "../../outside"}); err == nil {
		t.Fatal("path accepted")
	}
	cached := filepath.Join(store.dir, "session-preview0123456789", ref.ID)
	os.WriteFile(cached, []byte("interrupted previous write"), 0600)
	repaired := store.capture("session-preview0123456789", home, &acpagent.Preview{Data: data, Name: "shot.png"})
	part, err = store.read("session-preview0123456789", terminalproto.ACPPreview{ID: repaired.ID})
	if err != nil || !bytes.Equal(part.Data, data) {
		t.Fatal("recapture did not repair corrupt snapshot", err)
	}
	expired := time.Now().Add(-previewCacheAge - time.Hour)
	os.Chtimes(cached, expired, expired)
	if _, err = store.read("session-preview0123456789", terminalproto.ACPPreview{ID: ref.ID}); err == nil {
		t.Fatal("expired snapshot served")
	}
}

func TestPreviewReadRefusesEscapesSpecialFilesAndUnsupportedBytes(t *testing.T) {
	home := t.TempDir()
	store := newChatPreviewStore(t.TempDir(), home)
	os.Symlink("/etc/passwd", filepath.Join(home, "escape.png"))
	if err := syscall.Mkfifo(filepath.Join(home, "pipe.png"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []*acpagent.Preview{{Path: filepath.Join(home, "pipe.png")}, {Path: filepath.Join(home, "escape.png")}, {Data: []byte("<svg/>")}, {Data: make([]byte, terminalproto.MaxACPPreviewBytes+1)}} {
		ref := store.capture("session-preview0123456789", home, p)
		if ref.ID != "" || ref.Reason == "" {
			t.Fatalf("unsafe preview: %+v", ref)
		}
	}
}

func TestPreviewDeliveryRequiresNegotiationWithoutChangingTurnState(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	sup := newChatSupervisor(ctx, terminalhost.ManagerOptions{Home: t.TempDir(), Layout: terminalhost.Layout{Dir: t.TempDir()}})
	out := &sent{}
	sup.attach(ctx, 1, out.send, true, true, true, false)
	cs := &chatSession{id: "session-preview0123456789", state: terminalproto.Running}
	sup.sessions[cs.id] = cs
	sup.publish(cs, acpagent.Event{Kind: "preview", ToolCallID: "call-image", Preview: &acpagent.Preview{Data: previewPNG(t), Name: "shot.png"}})
	if len(out.msgs) != 0 {
		t.Fatal("preview leaked to old controller")
	}
	sup.attach(ctx, 2, out.send, true, true, true, true)
	sup.publish(cs, acpagent.Event{Kind: "preview", ToolCallID: "call-image", Preview: &acpagent.Preview{Data: previewPNG(t), Name: "shot.png"}})
	if len(out.msgs) != 1 {
		t.Fatalf("preview not delivered: %+v", out.msgs)
	}
	event := out.msgs[0].Body.(*terminalproto.ACPEvent)
	if event.Preview == nil || !event.Preview.Valid() || event.Preview.Reason != "" || cs.lastStatus != nil {
		t.Fatalf("preview altered state or lost bytes: %+v", event)
	}
}

func TestPreviewCachePrunesExpiredAndOldestSnapshots(t *testing.T) {
	store := newChatPreviewStore(t.TempDir(), t.TempDir())
	dir := filepath.Join(store.dir, "session-older0123456789")
	os.MkdirAll(dir, 0700)
	for _, name := range []string{"a", "b", "c"} {
		path := filepath.Join(dir, name)
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		f.Truncate(24 << 20)
		f.Close()
	}
	store.mu.Lock()
	store.pruneLocked(terminalproto.MaxACPPreviewBytes)
	store.mu.Unlock()
	var total int64
	filepath.WalkDir(store.dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			i, _ := d.Info()
			total += i.Size()
		}
		return nil
	})
	if total+terminalproto.MaxACPPreviewBytes > previewCacheBytes {
		t.Fatal("cache quota exceeded", total)
	}
}

func TestPreviewCachePrunesExpiredSnapshots(t *testing.T) {
	store := newChatPreviewStore(t.TempDir(), t.TempDir())
	dir := filepath.Join(store.dir, "session-expired0123456")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	fresh, stale := filepath.Join(dir, "fresh"), filepath.Join(dir, "stale")
	for _, path := range []string{fresh, stale} {
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-previewCacheAge - time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.pruneLocked(0)
	store.mu.Unlock()
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("a snapshot past the age bound was kept")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("a fresh snapshot was pruned: %v", err)
	}
}

func TestPreviewCachePrunesPastTheFileBound(t *testing.T) {
	store := newChatPreviewStore(t.TempDir(), t.TempDir())
	original := previewCacheFiles
	previewCacheFiles = 3
	t.Cleanup(func() { previewCacheFiles = original })
	dir := filepath.Join(store.dir, "session-files012345678")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, 4)
	for i, name := range []string{"a", "b", "c", "d"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		// Oldest first, so the eviction order is unambiguous.
		at := time.Now().Add(-time.Duration(4-i) * time.Minute)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	store.mu.Lock()
	store.pruneLocked(0)
	store.mu.Unlock()
	// Three files plus the free slot the next capture needs: the two oldest go.
	for _, gone := range paths[:2] {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Fatalf("a snapshot past the file bound was kept: %s", filepath.Base(gone))
		}
	}
	for _, kept := range paths[2:] {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("a newer snapshot was pruned: %s", filepath.Base(kept))
		}
	}
}
