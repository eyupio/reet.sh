package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

type importItem struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Host    string `json:"host"`
	Agent   string `json:"agent"`
	Project string `json:"project"`
	Path    string `json:"path"`
}

// importEntry marks one installed skill as used, keeping existing flags and notes.
// Without newItems a sighting only joins a live library skill: one of a name
// the library does not have returns no id and changes nothing.
func (s *server) importEntry(userID int64, item importItem, newItems bool, request *http.Request) (string, bool, error) {
	name, source := strings.TrimSpace(item.Name), strings.TrimSpace(item.Source)
	if name == "" {
		return "", false, errors.New("empty name")
	}
	var rf ref.Ref
	local := false
	privateWritten, privatePublished := false, false
	if source == "" && item.Host != "" && item.Agent != "" {
		wanted, ok, err := s.st.LibraryLocationIdentityFor(userID, item.Host, item.Agent, item.Project, name)
		if err != nil {
			return "", false, err
		}
		if ok && wanted.ItemID != "" && !strings.HasPrefix(wanted.ID, "hosted/") {
			id, err := s.st.RecordLibraryObservationFor(userID, wanted.ItemID, item.Host, item.Agent, item.Project, item.Path)
			return id, false, err
		}
		if ok && strings.HasPrefix(wanted.ID, "hosted/") {
			username, err := s.st.PublishingUsername(userID)
			if err != nil {
				return "", false, err
			}
			// Ownership and the saved private target authorize this identity;
			// the public-input parser deliberately rejects unpublished drafts.
			candidate, err := s.hostedRef(request, username, name)
			if err == nil && candidate.ID == wanted.ID {
				written, err := s.st.HostedSkillsFor(userID)
				if err != nil {
					return "", false, err
				}
				for _, skill := range written {
					if skill.Slug == name {
						rf, privateWritten, privatePublished = candidate, true, skill.Published
					}
				}
			}
		}
	}
	if strings.Contains(source, "://") {
		if parsed, err := s.parseSkillInput(request, strings.TrimRight(source, "/")+"/"+name); err == nil && parsed.SourceType == "hosted" {
			rf = parsed
		}
	}
	if source != "" && rf.ID == "" {
		source = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(source, "https://github.com/"), "http://github.com/"), ".git")
		if r, err := ref.Parse(source + "/" + name); err == nil {
			rf = r
			// A scan reports the folder the skills CLI made, which is named
			// after the skill and not always what the catalogue calls it. When
			// the shelf has no skill by the folder's name but has one from this
			// source that installs into it, that is the skill the scan found.
			if _, exact, err := s.st.GetFor(userID, r.ID); err != nil {
				return "", false, err
			} else if !exact {
				known, ok, err := s.st.SkillInstalledAsFor(userID, r.Source, name)
				if err != nil {
					return "", false, err
				}
				if ok {
					rf = known
				}
			}
		}
	}
	if rf.ID == "" {
		if r, err := s.parseSkillInput(request, name); err != nil {
			if strings.ContainsAny(name, "/\\\t\r\n") {
				return "", false, fmt.Errorf("can't tell what %q is", name)
			}
			local = true
		} else {
			rf = r
		}
	}
	id := rf.ID
	if local {
		id = "local/" + name
	}
	if !newItems {
		// Before any shelf row is written: a new one would make a library item.
		live, err := s.st.LiveLibrarySkillFor(userID, name, id)
		if err != nil || !live {
			return "", false, err
		}
	}
	_, existed, err := s.st.GetFor(userID, id)
	if err != nil {
		return id, false, err
	}
	used := true
	flags := store.Patch{}
	if !existed {
		flags.Used = &used
	}
	var sk store.Skill
	if local {
		sk, err = s.st.UpsertLocalFor(userID, name, flags)
	} else {
		sk, err = s.st.UpsertFor(userID, rf, flags)
		// A scanned copy of a name the library already has is a sighting of
		// that skill on the machine, not a new source. Like any sighting it
		// never moves the skill's status: flags are set only on a new row, as
		// for the local row it is written as.
		if errors.Is(err, store.ErrSkillNameTaken) && item.Host != "" && item.Agent != "" {
			local, id = true, "local/"+name
			if _, existed, err = s.st.GetFor(userID, id); err != nil {
				return id, false, err
			}
			flags = store.Patch{}
			if !existed {
				flags.Used = &used
			}
			sk, err = s.st.UpsertLocalFor(userID, name, flags)
		}
	}
	if err != nil {
		return id, false, err
	}
	if privateWritten {
		if err = s.st.SetHostedSkillVisible(sk.ID, privatePublished); err != nil {
			return id, false, err
		}
	} else if !local {
		s.enqueue(rf)
	}
	if item.Host != "" && item.Agent != "" {
		sourcePath := item.Path
		if sourcePath != "" {
			sourcePath = strings.TrimRight(sourcePath, "/") + "/" + name
		}
		if err := s.st.RecordEnvironmentFor(userID, sk.ID, item.Host, item.Agent, item.Project, item.Path, sourcePath); err != nil {
			return id, false, err
		}
	}
	return sk.ID, !existed, nil
}

func (s *server) importSkills(w http.ResponseWriter, r *http.Request) {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	var in struct {
		Machine      store.MachineInfo `json:"machine"`
		Items        []importItem      `json:"items"`
		Text         string            `json:"text"`
		Prune        bool              `json:"prune"`
		Environments []struct {
			Host    string   `json:"host"`
			Agent   string   `json:"agent"`
			Project string   `json:"project"`
			Path    string   `json:"path"`
			Skills  []string `json:"skills"`
			Errors  []string `json:"errors"`
		} `json:"environments"`
	}
	if !decode(w, r, &in) {
		return
	}
	// Resolve legacy names once before IDs, pruning and inventories are computed.
	aliases, err := s.st.MachineAliasesFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	canonicalize := func(host string) string {
		host = strings.TrimSpace(host)
		if canonical, ok := aliases[strings.ToLower(host)]; ok {
			return canonical
		}
		return host
	}
	in.Machine.Host = canonicalize(in.Machine.Host)
	for i := range in.Items {
		in.Items[i].Host = canonicalize(in.Items[i].Host)
	}
	for i := range in.Environments {
		in.Environments[i].Host = canonicalize(in.Environments[i].Host)
	}
	imported, created := 0, 0
	errs := []string{}
	for _, env := range in.Environments {
		for _, problem := range env.Errors {
			errs = append(errs, env.Agent+": "+problem)
		}
	}
	// A reference snapshot must never capture a scan midway through its writes.
	for _, env := range in.Environments {
		if err := s.st.TouchEnvironmentFor(s.uid(r), env.Host, env.Agent, env.Project, env.Path); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if err := s.st.RecordInventoryFor(s.uid(r), store.EnvironmentIDFor(s.uid(r), env.Host, env.Agent, env.Project), false, in.Machine); err != nil {
			errs = append(errs, err.Error())
		}
	}

	for _, item := range in.Items {
		if item.Host != "" && item.Agent != "" {
			if err := s.st.TouchEnvironmentFor(s.uid(r), item.Host, item.Agent, item.Project, item.Path); err != nil {
				errs = append(errs, err.Error())
				continue
			}
			if err := s.st.RecordInventoryFor(s.uid(r), store.EnvironmentIDFor(s.uid(r), item.Host, item.Agent, item.Project), false, in.Machine); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	seen := map[string]bool{}
	itemIDs := map[string]string{}
	handle := func(item importItem) {
		id, wasCreated, err := s.importEntry(s.uid(r), item, scanMakesItems(item, in.Machine.Host), r)
		switch {
		case err != nil:
			errs = append(errs, err.Error())
		case id == "":
			// A sighting the scan makes no item of. From a managed folder it
			// is New on a machine through the skills report; from Codex's
			// folder, which reet leaves alone, it appears nowhere.
		default:
			if !seen[id] {
				seen[id] = true
				imported++
				if wasCreated {
					created++
				}
			}
		}
		found := id != "" && err == nil
		if found {
			key := store.EnvironmentIDFor(s.uid(r), item.Host, item.Agent, item.Project) + "\x00" + item.Name
			itemIDs[key] = id
		}
	}
	for _, item := range in.Items {
		handle(item)
	}
	for _, line := range strings.Split(in.Text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimLeft(line, "-*"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		handle(importItem{Name: line})
	}
	pruned := 0
	if in.Prune && len(errs) == 0 {
		for _, env := range in.Environments {
			if err := s.st.TouchEnvironmentFor(s.uid(r), env.Host, env.Agent, env.Project, env.Path); err != nil {
				errs = append(errs, err.Error())
				continue
			}
			envID := store.EnvironmentIDFor(s.uid(r), env.Host, env.Agent, env.Project)
			keep := []string{}
			for _, name := range env.Skills {
				if id, ok := itemIDs[envID+"\x00"+name]; ok {
					keep = append(keep, id)
				}
			}
			n, err := s.st.PruneEnvironmentFor(s.uid(r), envID, keep)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			pruned += int(n)
		}
	}
	if in.Machine.Host != "" {
		if err := s.st.RegisterMachineFor(s.uid(r), in.Machine); err != nil {
			errs = append(errs, err.Error())
		}
	}
	for _, env := range in.Environments {
		if err := s.st.TouchEnvironmentFor(s.uid(r), env.Host, env.Agent, env.Project, env.Path); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if err := s.st.RecordInventoryFor(s.uid(r), store.EnvironmentIDFor(s.uid(r), env.Host, env.Agent, env.Project), in.Prune && len(errs) == 0, in.Machine); err != nil {
			errs = append(errs, err.Error())
		}
	}
	for _, env := range in.Environments {
		if err := s.st.RecordScanErrorFor(s.uid(r), store.EnvironmentIDFor(s.uid(r), env.Host, env.Agent, env.Project), strings.Join(env.Errors, "; ")); err != nil {
			errs = append(errs, err.Error())
		}
	}
	source, action := "import", "import"
	if r.Header.Get("X-Reet-Source") == "scan" {
		source, action = "scan", "scan"
	}
	detail := map[string]any{"imported": imported, "created": created, "pruned": pruned}
	if source == "scan" && created == 0 && pruned == 0 && len(errs) == 0 {
		// A machine scans every minute. A scan that found nothing new is
		// recorded once for the same counts, then again after six hours, so the
		// log shows the machine is still reporting without a row a minute.
		if err := s.st.AddEventUnlessRepeatedFor(s.uid(r), source, action, detail, 6*time.Hour); err != nil && s.debug {
			log.Printf("event store action=%s error=%v", action, err)
		}
	} else {
		s.event(s.uid(r), source, action, "", detail)
	}
	writeJSON(w, 200, map[string]any{"imported": imported, "created": created, "pruned": pruned, "errors": errs, "user": s.currentUser(r).Name})
}

func (s *server) environments(w http.ResponseWriter, r *http.Request) {
	envs, err := s.st.SyncEnvironmentsFor(s.uid(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	machines, err := s.st.GroupMachinesFor(s.uid(r), envs)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	q := r.URL.Query()
	emptyOnly := q.Get("empty_only") == "1" || q.Get("empty_only") == "true"
	agent := strings.TrimSpace(q.Get("agent"))
	if emptyOnly || agent != "" {
		filtered := make([]store.SyncEnvironment, 0, len(envs))
		for _, env := range envs {
			if emptyOnly && env.SkillCount != 0 {
				continue
			}
			if agent != "" && !strings.EqualFold(env.Agent, agent) {
				continue
			}
			filtered = append(filtered, env)
		}
		envs = filtered
	}
	writeJSON(w, 200, map[string]any{"environments": envs, "machines": machines})
}
