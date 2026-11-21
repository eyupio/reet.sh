package api

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

func settingEnabled(st *store.Store, key string) bool {
	v, _ := st.GetSetting(key)
	return v != "0"
}

func (s *server) probeSkillsSh(ctx context.Context) bool {
	if s.f.PageBase == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.f.PageBase, "/")+"/", nil)
	if err != nil {
		return false
	}
	client := s.f.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode == http.StatusOK
}

func (s *server) adminStatus(w http.ResponseWriter, r *http.Request) {
	skills, counts, err := s.st.ListFor(s.uid(r), store.Filter{})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	stages := map[string]int{"use": 0, "try": 0, "shelf": 0}
	for _, sk := range skills {
		if sk.Used {
			stages["use"]++
		} else if sk.Bookmarked {
			stages["try"]++
		} else {
			stages["shelf"]++
		}
	}
	envs, err := s.st.ListEnvironmentsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	events, err := s.st.ListAllEvents(100, "")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	size, _ := s.st.Size()
	topicsSynced, _ := s.st.GetSetting("topics_last_synced")
	topicsError, _ := s.st.GetSetting("topics_last_error")
	metaOK, _ := s.st.GetSetting("meta_last_ok_at")
	metaError, _ := s.st.GetSetting("meta_last_error")
	instanceName, _ := s.st.GetSetting("instance_name")
	pruneRaw, _ := s.st.GetSetting("prune_server_environments_days")
	pruneDays, _ := strconv.Atoi(strings.TrimSpace(pruneRaw))
	writeJSON(w, 200, map[string]any{
		"version": s.version,
		"commit":  s.commit,
		"db": map[string]any{
			"path": filepath.Join(s.st.Path(), "reet.db"),
			"size": size,
		},
		"counts":        counts,
		"stages":        stages,
		"environments":  envs,
		"events":        events,
		"skillsSh":      s.probeSkillsSh(r.Context()),
		"metaLastOK":    metaOK,
		"metaLastError": metaError,
		"topics": map[string]any{
			"lastSynced": topicsSynced,
			"lastError":  topicsError,
		},
		"settings": map[string]any{
			"refreshEnabled":              settingEnabled(s.st, "refresh_enabled"),
			"topicsEnabled":               settingEnabled(s.st, "topics_enabled"),
			"linkListsEnabled":            s.linkListsEnabled(),
			"rollupIndividual":            s.rollupIndividual(),
			"pruneServerEnvironmentsDays": pruneDays,
			"instanceName":                instanceName,
		},
	})
}

func (s *server) adminSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshEnabled   *bool   `json:"refreshEnabled"`
		TopicsEnabled    *bool   `json:"topicsEnabled"`
		LinkListsEnabled *bool   `json:"linkListsEnabled"`
		RollupIndividual *bool   `json:"rollupIndividual"`
		PruneDays        *int    `json:"pruneServerEnvironmentsDays"`
		InstanceName     *string `json:"instanceName"`
	}
	if !decode(w, r, &in) {
		return
	}
	detail := map[string]any{}
	if in.RefreshEnabled != nil {
		value := "0"
		if *in.RefreshEnabled {
			value = "1"
		}
		if err := s.st.SetSetting("refresh_enabled", value); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		detail["refreshEnabled"] = *in.RefreshEnabled
	}
	if in.TopicsEnabled != nil {
		value := "0"
		if *in.TopicsEnabled {
			value = "1"
		}
		if err := s.st.SetSetting("topics_enabled", value); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		detail["topicsEnabled"] = *in.TopicsEnabled
	}
	if in.LinkListsEnabled != nil {
		value := "0"
		if *in.LinkListsEnabled {
			value = "1"
		}
		if err := s.st.SetSetting("link_lists_enabled", value); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		detail["linkListsEnabled"] = *in.LinkListsEnabled
	}
	if in.RollupIndividual != nil {
		value := "0"
		if *in.RollupIndividual {
			value = "1"
		}
		if err := s.st.SetSetting("rollup_individual", value); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		detail["rollupIndividual"] = *in.RollupIndividual
	}
	if in.PruneDays != nil {
		days := *in.PruneDays
		if days < 0 || days > 3650 {
			writeErrCode(w, 400, errInvalidArgument, "pruneServerEnvironmentsDays", "choose 0 to 3650 days", "0 turns automatic pruning off.")
			return
		}
		if err := s.st.SetSetting("prune_server_environments_days", strconv.Itoa(days)); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		detail["pruneServerEnvironmentsDays"] = days
	}
	if in.InstanceName != nil {
		if err := s.st.SetSetting("instance_name", strings.TrimSpace(*in.InstanceName)); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		detail["instanceName"] = strings.TrimSpace(*in.InstanceName)
	}
	s.eventRequest(r, "settings", "", detail)
	writeJSON(w, 200, map[string]any{"ok": true})
}
