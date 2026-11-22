package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/eyupio/reet.sh/internal/skillbuddy"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

const buddySystemKey = "skill_buddy_system_v1"

var buddySystemID = regexp.MustCompile(`^builtin:[a-z][a-z0-9-]{0,59}$`)

type buddySystemConfig struct {
	Version   int64              `json:"version"`
	Overrides []skillbuddy.Skill `json:"overrides"`
}

func (s *server) buddySystem() (out []skillbuddy.Skill, config buddySystemConfig, err error) {
	raw, err := s.st.GetSetting(buddySystemKey)
	if err != nil {
		return nil, config, err
	}
	if raw != "" {
		if err = json.Unmarshal([]byte(raw), &config); err != nil {
			return nil, config, err
		}
	}
	out = skillbuddy.Builtins()
	index := map[string]int{}
	for i, sk := range out {
		index[sk.ID] = i
	}
	for _, sk := range config.Overrides {
		if at, ok := index[sk.ID]; ok {
			out[at] = sk
		} else {
			index[sk.ID] = len(out)
			out = append(out, sk)
		}
	}
	return
}
func (s *server) buddyAdminGet(w http.ResponseWriter, r *http.Request) {
	sk, config, err := s.buddySystem()
	if err != nil {
		writeErr(w, 500, "System skills could not be read.")
		return
	}
	writeJSON(w, 200, map[string]any{"skills": sk, "defaults": skillbuddy.Builtins(), "version": config.Version})
}
func (s *server) buddyAdminSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Skill   skillbuddy.Skill `json:"skill"`
		Version int64            `json:"version"`
		Reset   bool             `json:"reset"`
	}
	if !terminalJSON(w, r, &in) {
		return
	}
	sk := in.Skill
	if !buddySystemID.MatchString(sk.ID) {
		writeErr(w, 400, "Use a system ID such as builtin:code-review.")
		return
	}
	if !in.Reset {
		curated := sk.Source == "curated"
		plain := func(v string) bool {
			return !strings.ContainsFunc(v, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) })
		}
		if len(sk.Stages) > 8 {
			writeErr(w, 400, "Choose at most eight Factory stages.")
			return
		}
		if strings.TrimSpace(sk.Name) == "" || len(sk.Name) > 80 || !plain(sk.Name) || len(sk.Description) > 600 || !plain(sk.Description) || len(sk.Keywords) > 600 || len(sk.Attribution) > 1000 || !skillbuddy.ValidStage(sk.Stage) || sk.Stage == "" || (!curated && strings.TrimSpace(sk.Instructions) == "") || len(sk.Instructions) > 1000 {
			writeErr(w, 400, "Give a name, stage and up to 1,000 bytes of instructions. Keep metadata on one line.")
			return
		}
		sk.MatchPhrases = strings.TrimSpace(sk.MatchPhrases)
		if len(sk.MatchPhrases) > 600 || !plain(sk.MatchPhrases) {
			writeErr(w, 400, "Keep task phrases on one line and within 600 bytes.")
			return
		}
		for _, stage := range sk.Stages {
			if !skillbuddy.ValidStage(stage) || stage == "" {
				writeErr(w, 400, "Choose valid Factory stages.")
				return
			}
		}
		if !curated && len(skillbuddy.Prompt(sk)) > 1500 {
			writeErr(w, 400, "Shorten the instructions or attribution so the guidance fits in a session message.")
			return
		}
		sk.Source = "builtin"
		sk.State = ""
		if curated {
			u, e := url.Parse(sk.URL)
			if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || len(sk.URL) > 1000 {
				writeErr(w, 400, "Use an HTTPS source URL without credentials.")
				return
			}
			sk.Source = "curated"
			sk.Instructions = ""
		} else {
			sk.URL = ""
		}
		sk.Reason = ""
		sk.Score = 0
		raw, _ := json.Marshal(sk)
		sum := sha256.Sum256(raw)
		sk.Revision = hex.EncodeToString(sum[:8])
	}
	// A settings compare-and-swap keeps two administrators from losing each other's edits.
	_, config, err := s.buddySystem()
	if err != nil {
		writeErr(w, 500, "System skills could not be read.")
		return
	}
	if config.Version != in.Version {
		writeErr(w, 409, "System skills changed. Reload before saving.")
		return
	}
	next := buddySystemConfig{Version: config.Version + 1, Overrides: []skillbuddy.Skill{}}
	for _, old := range config.Overrides {
		if old.ID != sk.ID {
			next.Overrides = append(next.Overrides, old)
		}
	}
	if !in.Reset {
		next.Overrides = append(next.Overrides, sk)
	}
	if len(next.Overrides) > 64 {
		writeErr(w, 400, "Keep at most 64 system skill overrides.")
		return
	}
	raw, _ := json.Marshal(next)
	oldRaw, err := s.st.GetSetting(buddySystemKey)
	if err != nil {
		writeErr(w, 500, "System skills could not be saved.")
		return
	}
	var current buddySystemConfig
	if oldRaw != "" {
		if json.Unmarshal([]byte(oldRaw), &current) != nil {
			writeErr(w, 500, "System skills could not be read.")
			return
		}
	}
	if current.Version != in.Version {
		writeErr(w, 409, "System skills changed. Reload before saving.")
		return
	}
	if err = s.st.CompareSetting(buddySystemKey, oldRaw, string(raw)); err != nil {
		writeErr(w, 409, "System skills changed. Reload before saving.")
		return
	}
	s.event(s.uid(r), "admin", "system-skill-change", "", map[string]any{"skill": sk.ID, "reset": in.Reset, "revision": sk.Revision})
	s.buddyAdminGet(w, r)
}
