package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/store"
)

// skillInventory is the report of a machine that may not write: what the
// managed folders hold, changing nothing. Every copy of an Active or removed
// name is reported under that name, in any case: an Active copy that is reet's
// is installed and one that differs has failed; a removed name still present
// is installed, so the controller does not take it for gone. Only other
// skills are unknown. A folder the pass would leave alone says so, as it does
// in a pass.
func skillInventory(home string, d store.DesiredState, unmanaged []string) store.SkillReport {
	active, removals, refused := checkDesired(d)
	want := map[string]store.DesiredSkill{}
	for _, a := range active {
		want[strings.ToLower(a.Name)] = a
	}
	gone := map[string]string{}
	for _, rm := range removals {
		gone[strings.ToLower(rm.Name)] = rm.Name
	}
	claimed := claimedNames(active, removals, refused)
	rep := store.SkillReport{Version: d.Version, Blocked: "allow-apply", Entries: refused}
	if !filepath.IsAbs(home) {
		return rep
	}
	r := &reconciler{home: home, digests: map[string]copyDigest{}, entries: refused}
	roots := managedRoots(home, unmanaged)
	if roots[0].handled() {
		r.sharedReal = roots[0].real
	}
	for _, i := range reconcileOrder {
		f, root := newFolderRun(home, i, roots[i]), roots[i]
		if root.why != "" {
			r.leftAlone(f, root.why, active, removals)
			continue
		}
		if !root.handled() {
			continue
		}
		names, err := skillEntries(f.dir)
		if err != nil {
			continue
		}
		for _, lower := range slices.Sorted(maps.Keys(names)) {
			for _, name := range names[lower] {
				if a, ok := want[lower]; ok {
					if r.copyAt(f, name).is(a.Digest) {
						r.add(f.label, a.Name, "installed", a.Digest, "", "")
					} else {
						r.add(f.label, a.Name, "failed", "", "allow-apply is off", "")
					}
				} else if removed, ok := gone[lower]; ok {
					r.add(f.label, removed, "installed", r.copyAt(f, name).all, "", "")
				} else if digest, ok := r.skillAt(f, name); ok && !claimed[lower] {
					r.add(f.label, name, "unknown", digest, "", "")
				}
			}
		}
	}
	rep.Entries = r.entries
	return rep
}

// controllerBundles fetches an Active skill's revision from the controller,
// checking it as every private install does.
func controllerBundles(c agentConfig) bundleFetcher {
	return func(ctx context.Context, s store.DesiredSkill) ([]skillbundle.ContentFile, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// The download is named by bundle and revision; the skill's name,
		// which may hold a space or any letter, names only its folder.
		source := "private:" + s.Bundle + ":" + s.Revision + ":" + s.Digest
		return privateSkillFiles(c.Server, c.Token, store.SyncSkill{Slug: s.Bundle, SourceType: "private", Source: source})
	}
}

// desiredCache is <config>.desired: the last desired state a poll carried,
// with the controller and machine it was for.
type desiredCache struct {
	Server  string             `json:"server"`
	Agent   string             `json:"agent"`
	Desired store.DesiredState `json:"desired"`
}

// readDesiredSkills returns the desired state the last poll that carried one
// left, or nil. One kept for another controller or enrollment is not used.
func readDesiredSkills(config string, c agentConfig) *store.DesiredState {
	b, err := os.ReadFile(config + ".desired")
	if err != nil {
		return nil
	}
	var kept desiredCache
	if json.Unmarshal(b, &kept) != nil || kept.Server != c.Server || kept.Agent != c.ID {
		return nil
	}
	return &kept.Desired
}

// unsentCache is <config>.unsent: entries no report has delivered yet that a
// later pass would not report again, with the controller and machine they
// are for.
type unsentCache struct {
	Server  string                   `json:"server"`
	Agent   string                   `json:"agent"`
	Entries []store.SkillReportEntry `json:"entries"`
}

// unsentKept caps the entries kept while reports fail, keeping the newest.
const unsentKept = 500

func readUnsentSkills(config string, c agentConfig) []store.SkillReportEntry {
	b, err := os.ReadFile(config + ".unsent")
	if err != nil {
		return nil
	}
	var kept unsentCache
	if json.Unmarshal(b, &kept) != nil || kept.Server != c.Server || kept.Agent != c.ID {
		return nil
	}
	return kept.Entries
}

// withUnsent adds the entries kept until delivered: what earlier reports
// carried but never delivered, and news this pass passed on that its report
// does not carry, such as an edit whose skill then failed. Each
// goes just before this report's entry for the same skill and folder, which
// the controller records last, or at the end. An installed entry of the same
// revision says nothing an unsent edit does not, so it is left out.
func withUnsent(rep store.SkillReport, unsent []store.SkillReportEntry) store.SkillReport {
	// An entry this report already carries is not added twice.
	unsent = slices.DeleteFunc(slices.Clone(unsent), func(u store.SkillReportEntry) bool { return slices.Contains(rep.Entries, u) })
	if len(unsent) == 0 {
		return rep
	}
	key := func(e store.SkillReportEntry) string { return e.Folder + "\x00" + strings.ToLower(e.Name) }
	waiting := map[string][]store.SkillReportEntry{}
	for _, u := range unsent {
		waiting[key(u)] = append(waiting[key(u)], u)
	}
	entries := make([]store.SkillReportEntry, 0, len(rep.Entries)+len(unsent))
	for _, e := range rep.Entries {
		before := waiting[key(e)]
		delete(waiting, key(e))
		entries = append(entries, before...)
		if n := len(before); n > 0 && e.State == "installed" && before[n-1].State == "edited" && before[n-1].Digest == e.Digest {
			continue
		}
		entries = append(entries, e)
	}
	for _, u := range unsent {
		if _, left := waiting[key(u)]; left {
			entries = append(entries, u)
		}
	}
	rep.Entries = entries
	return rep
}

// readInstalledSkills reads what reet last installed. A file that cannot be
// read as that is moved aside and said so, and the pass starts afresh: a
// copy that differs from reet's is then reported edited once.
func readInstalledSkills(config string, now time.Time, out io.Writer) map[string]string {
	installed := map[string]string{}
	b, err := os.ReadFile(config + ".installed")
	if errors.Is(err, fs.ErrNotExist) {
		return installed
	}
	if err == nil && json.Unmarshal(b, &installed) == nil && installed != nil {
		return installed
	}
	if err != nil {
		fmt.Fprintf(out, "Skills: could not read %s.installed (%v); every copy is checked afresh.\n", config, err)
		return map[string]string{}
	}
	aside := fmt.Sprintf("%s.installed.corrupt-%d", config, now.UnixMilli())
	if err := os.Rename(config+".installed", aside); err != nil {
		fmt.Fprintf(out, "Skills: %s.installed is damaged and could not be moved aside (%v); every copy is checked afresh.\n", config, err)
	} else {
		fmt.Fprintf(out, "Skills: %s.installed was damaged and is now %s; every copy is checked afresh.\n", config, filepath.Base(aside))
	}
	return map[string]string{}
}

// skillReportsSent holds, per enrollment file, the last skill report this
// process posted, so an unchanged report is not sent again.
var skillReportsSent = struct {
	sync.Mutex
	last map[string]string
}{last: map[string]string{}}

// reconcileAgentSkills runs one reconcile with fresh, the desired state this
// poll carried, or else cached, the one an earlier poll left. With neither it
// does nothing. With allow-apply off it only reports what is there.
func reconcileAgentSkills(ctx context.Context, c agentConfig, config string, fresh, cached *store.DesiredState, version string, now time.Time, out io.Writer) error {
	var errs []error
	d := cached
	if fresh != nil {
		d = fresh
		was, _ := json.Marshal(cached)
		is, _ := json.Marshal(fresh)
		if !bytes.Equal(was, is) {
			if err := writePrivate(config+".desired", desiredCache{Server: c.Server, Agent: c.ID, Desired: *fresh}); err != nil {
				errs = append(errs, fmt.Errorf("could not keep the skills this machine should have: %w", err))
			}
		}
	}
	if d == nil {
		return nil
	}
	// Read the local opt-in fresh, so allow-apply takes effect on the next
	// cycle without restarting the agent.
	local, err := loadAgentConfig(config)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	unsent := readUnsentSkills(config, c)
	keepUnsent := func() {
		if len(unsent) > unsentKept {
			unsent = unsent[len(unsent)-unsentKept:]
		}
		if err := writePrivate(config+".unsent", unsentCache{Server: c.Server, Agent: c.ID, Entries: unsent}); err != nil {
			errs = append(errs, fmt.Errorf("could not keep what this pass did until it is reported: %w", err))
		}
	}
	var rep store.SkillReport
	if local.AllowApply {
		installed := readInstalledSkills(config, now, out)
		// Each edit or removal is kept the moment its backup is made, so an
		// agent stopped before the report still reports it.
		opts := reconcileOptions{unmanaged: c.Projects,
			news: func(e store.SkillReportEntry) {
				unsent = append(unsent, e)
				keepUnsent()
			},
			retract: func(e store.SkillReportEntry) {
				if i := slices.Index(unsent, e); i >= 0 {
					unsent = slices.Delete(unsent, i, i+1)
					keepUnsent()
				}
			}}
		var next map[string]string
		rep, next = reconcileWith(ctx, c.Home, *d, installed, controllerBundles(c), now, opts)
		if !maps.Equal(installed, next) {
			if err := writePrivate(config+".installed", next); err != nil {
				errs = append(errs, err)
			}
		}
	} else {
		rep = skillInventory(c.Home, *d, c.Projects)
	}
	var undelivered []string
	if err := postSkillReport(ctx, c, config, withUnsent(rep, unsent), version); err != nil {
		errs = append(errs, err)
		// Until the controller has heard of them, these backups may hold the
		// only copy of someone's work.
		for _, u := range unsent {
			if u.Backup != "" {
				undelivered = append(undelivered, fromTilde(c.Home, u.Backup))
			}
		}
	} else if err := os.Remove(config + ".unsent"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		// The controller has every entry kept there now.
		errs = append(errs, err)
	}
	if local.AllowApply {
		errs = append(errs, pruneBackups(c.Home, now, c.Projects, undelivered...))
	}
	return errors.Join(errs...)
}

// fromTilde turns a report's ~/ path back into the path under home.
func fromTilde(home, p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(home, filepath.FromSlash(rest))
	}
	return p
}

// postSkillReport sends rep unless this process already sent the same one.
// It returns nil once the controller has rep.
func postSkillReport(ctx context.Context, c agentConfig, config string, rep store.SkillReport, version string) error {
	b, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	skillReportsSent.Lock()
	same := skillReportsSent.last[config] == string(b)
	skillReportsSent.Unlock()
	if same {
		return nil
	}
	if err := agentRequestContext(ctx, c, "/api/v2/sync/skills/report", json.RawMessage(b), nil, version); err != nil {
		return fmt.Errorf("skills report not sent; it is sent again next cycle: %w", err)
	}
	skillReportsSent.Lock()
	skillReportsSent.last[config] = string(b)
	skillReportsSent.Unlock()
	return nil
}
