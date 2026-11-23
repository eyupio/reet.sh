//go:build linux || darwin

package terminalagent

import (
	"context"
	"fmt"
	"testing"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type fakeCommandsChat struct {
	*fakeChat
	commands []terminalproto.ACPCommand
}

func (f *fakeCommandsChat) Controls() terminalproto.ACPControls {
	return terminalproto.ACPControls{Model: "stub/model", Options: []terminalproto.ACPSetting{}}
}
func (f *fakeCommandsChat) SetOption(context.Context, string, string) error { return nil }
func (f *fakeCommandsChat) Steer(context.Context, string, []acpagent.Attachment) (string, error) {
	return "rejected", nil
}
func (f *fakeCommandsChat) Commands() []terminalproto.ACPCommand { return f.commands }

func TestCommandsActionAnswersTheAnnouncedList(t *testing.T) {
	sup := newChatSupervisor(t.Context(), terminalhost.ManagerOptions{Home: t.TempDir()})
	h := &chatHost{sup: sup, link: &chatLink{}}
	want := []terminalproto.ACPCommand{{Name: "review", Description: "Review changes", Hint: "branch"}}
	sup.mu.Lock()
	sup.sessions["session-0123456789"] = &chatSession{id: "session-0123456789", state: terminalproto.Running, process: &fakeCommandsChat{fakeChat: &fakeChat{done: make(chan struct{}, 1), closed: make(chan struct{})}, commands: want}}
	sup.mu.Unlock()
	answer, err := h.control(t.Context(), "session-0123456789", terminalproto.ACPControl{Action: "commands"})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(answer.Commands) != fmt.Sprint(want) {
		t.Fatalf("commands = %+v", answer.Commands)
	}
	// The list travels alone: a full model catalogue beside it could not fit
	// one frame.
	if answer.Controls.Model != "" || answer.Controls.Options == nil {
		t.Fatalf("controls rode along with the commands: %+v", answer.Controls)
	}
	m := terminalproto.Message{Version: terminalproto.Version, RequestID: "request-0123456789", SessionID: "session-0123456789", Generation: 1, Body: &answer}
	if _, err := terminalproto.Encode(m); err != nil {
		t.Fatal(err)
	}
	other, err := h.control(t.Context(), "session-0123456789", terminalproto.ACPControl{Action: "get"})
	if err != nil || other.Commands != nil {
		t.Fatalf("a get carried commands: %+v %v", other, err)
	}
}

// A controller that did not negotiate acp-commands-v1 never asks for the list,
// so one that does is a protocol error and is never answered with it.
func TestCommandsActionNeedsItsFeature(t *testing.T) {
	in := &terminalproto.ACPControl{Action: "commands"}
	live := terminalproto.ACPFeature + "," + terminalproto.ACPLiveControlsFeature
	if bodyFeatureError(in, terminalproto.ParseFeatures(live)) == nil {
		t.Fatal("commands accepted without acp-commands-v1")
	}
	if err := bodyFeatureError(in, terminalproto.ParseFeatures(live+","+terminalproto.ACPCommandsFeature)); err != nil {
		t.Fatal(err)
	}
	if err := bodyFeatureError(&terminalproto.ACPControl{Action: "get"}, terminalproto.ParseFeatures(live)); err != nil {
		t.Fatal(err)
	}
}
