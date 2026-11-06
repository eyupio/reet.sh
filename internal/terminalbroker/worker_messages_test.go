package terminalbroker

import (
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestCompanionQueueChecksPermissionAtDeliveryAndInterruptsOnlyExplicitly(t *testing.T) {
	for _, scenario := range []string{"revoked", "controller_disabled", "explicit_interrupt"} {
		t.Run(scenario, func(t *testing.T) {
			var working, enabled atomic.Bool
			working.Store(true)
			enabled.Store(true)
			var prompts, stops atomic.Int32
			b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true }, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
				switch m.Body.(type) {
				case *terminalproto.ACPControl:
					return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Working: working.Load(), Options: []terminalproto.ACPSetting{}}}, true
				case *terminalproto.ACPPrompt:
					prompts.Add(1)
				case *terminalproto.ACPCancel:
					stops.Add(1)
					working.Store(false)
				}
				return nil, true
			})
			b.SetWorkerMessagingPolicy(enabled.Load)
			session := chatSession(t, b, uid, agent)
			client, err := b.st.CreateMCPConnection(uid, "Companion test", "write", "companion-test-hash", time.Now().Add(time.Hour).UnixMilli())
			if err != nil {
				t.Fatal(err)
			}
			if err = b.st.SetMCPEnabled(uid, true); err != nil {
				t.Fatal(err)
			}
			subject := strconv.FormatInt(client.ID, 10)
			grant, err := b.st.SetWorkerGrantFor(uid, "mcp", subject, 0, store.WorkerGrant{Enabled: true, SessionID: session.ID})
			if err != nil {
				t.Fatal(err)
			}
			message, _, err := b.st.AdmitWorkerMessageFor(uid, "mcp", subject, store.WorkerMessage{ID: strings.Repeat("a", 32), SessionID: session.ID, Text: "Raise the PR when ready.", GrantRevision: grant.Revision}, "")
			if err != nil {
				t.Fatal(err)
			}
			if err = b.EnqueueChat(uid, session.ID, message.QueueID, message.Text, nil); err != nil {
				t.Fatal(err)
			}
			if prompts.Load() != 0 || stops.Load() != 0 {
				t.Fatal("default wait interrupted or prompted the busy worker")
			}
			if scenario == "explicit_interrupt" {
				receipt, err := b.st.WorkerMessageFor(uid, "mcp", subject, session.ID, message.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err = b.st.InterruptWorkerMessageFor(uid, "mcp", subject, session.ID, message.ID, receipt.Revision); err != nil {
					t.Fatal(err)
				}
				deadline := time.Now().Add(3 * time.Second)
				for time.Now().Before(deadline) {
					b.stepChatQueue(uid, session.ID)
					receipt, err = b.st.WorkerMessageFor(uid, "mcp", subject, session.ID, message.ID)
					if err != nil || receipt.State == "sent" {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				if err != nil || stops.Load() != 1 || prompts.Load() != 1 || receipt.State != "sent" {
					t.Fatal("stop-and-send outcome", stops.Load(), prompts.Load(), receipt, err)
				}
			} else {
				if scenario == "revoked" {
					if err = b.st.RevokeMCPConnection(uid, client.ID); err != nil {
						t.Fatal(err)
					}
				} else {
					enabled.Store(false)
				}
				working.Store(false)
				b.stepChatQueue(uid, session.ID)
				if prompts.Load() != 0 || stops.Load() != 0 {
					t.Fatal("policy change did not fence delivery")
				}
			}
		})
	}
}
