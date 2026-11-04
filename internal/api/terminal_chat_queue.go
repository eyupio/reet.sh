package api

import (
	"errors"
	"net/http"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (s *server) terminalChatQueue(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.terminalChatTarget(w, r); !ok {
		return
	}
	uid, session := s.uid(r), r.PathValue("id")
	var err error
	if r.Method == http.MethodPost {
		var in struct {
			ID     string   `json:"id"`
			Text   string   `json:"text"`
			Images []string `json:"images"`
		}
		if !terminalJSON(w, r, &in) {
			return
		}
		err = s.terminals.EnqueueChat(uid, session, in.ID, in.Text, in.Images)
	} else if r.Method == http.MethodPatch {
		var in struct {
			ID          string `json:"id"`
			Revision    int64  `json:"revision"`
			Action      string `json:"action"`
			Text        string `json:"text"`
			Acknowledge bool   `json:"acknowledge"`
		}
		if !terminalJSON(w, r, &in) {
			return
		}
		if !terminalproto.ValidID(in.ID) || in.Revision < 1 {
			writeErr(w, 400, "invalid queued message")
			return
		}
		err = s.terminals.ChangeChatQueue(uid, session, in.ID, in.Revision, in.Action, in.Text, in.Acknowledge)
	}
	if errors.Is(err, store.ErrChatQueueConflict) || errors.Is(err, store.ErrChatQueueFull) {
		writeErr(w, 409, err.Error())
		return
	}
	if err != nil {
		s.terminalChatError(w, err)
		return
	}
	queue, err := s.terminals.ChatQueue(uid, session)
	if err != nil {
		s.terminalChatError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"queue": queue})
}
