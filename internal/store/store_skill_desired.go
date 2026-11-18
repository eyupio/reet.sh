package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"time"
)

// SkillGrace is how long a change waits before machines are told about it, so
// an Undo inside it sends nothing.
const SkillGrace = 10 * time.Second

type DesiredSkill struct{ Name, Item, Revision, Bundle, Digest string }
type DesiredRemoval struct{ Name, Reason string }

// DesiredState is what every machine of an account makes its skills match.
type DesiredState struct {
	Version string
	Active  []DesiredSkill
	Remove  []DesiredRemoval
}

// DesiredSkillsFor computes the skills every machine should have at now. An
// item's effective status is its previous status for SkillGrace after its
// last change, and an item deleted inside that window still counts, so a
// change reaches machines only once it has held for SkillGrace. An Active
// item without content is held: in neither list. A name that is effectively
// Active is never on the remove list.
//
// An item archived and then deleted gets a fresh 'deleted' removal row (its
// archived one is replaced), so its name leaves Remove for SkillGrace and the
// Version changes twice: once when the archive's grace ends and the name
// appears, once when the delete's grace ends and the reason changes. Machines
// treat a name's absence from Remove as "do nothing", so the gap is harmless.
func (s *Store) DesiredSkillsFor(uid int64, now time.Time) (DesiredState, error) {
	return desiredSkills(s.db, uid, now)
}

func desiredSkills(q privateSkillQuery, uid int64, now time.Time) (DesiredState, error) {
	at, grace := now.UnixMilli(), SkillGrace.Milliseconds()
	state := DesiredState{Active: []DesiredSkill{}, Remove: []DesiredRemoval{}}
	rows, err := q.Query(`SELECT id,name,status,prev_status,status_at,deleted_at,revision_id,bundle_id,digest FROM library_items WHERE user_id=? AND item_type=?`, uid, LibrarySkill)
	if err != nil {
		return state, err
	}
	active := map[string]bool{}
	for rows.Next() {
		var id, name, status, prev, revision, bundle, digest string
		var statusAt, deletedAt int64
		if err = rows.Scan(&id, &name, &status, &prev, &statusAt, &deletedAt, &revision, &bundle, &digest); err != nil {
			break
		}
		if deletedAt > 0 && at-deletedAt >= grace {
			continue
		}
		if at-statusAt < grace && prev != "" {
			status = prev
		}
		if status != SkillActive {
			continue
		}
		active[strings.ToLower(name)] = true
		if revision != "" {
			state.Active = append(state.Active, DesiredSkill{Name: name, Item: id, Revision: revision, Bundle: bundle, Digest: digest})
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return state, err
	}
	removals, err := skillRemovalsOf(q, uid)
	if err != nil {
		return state, err
	}
	for _, r := range removals {
		if at-r.CreatedAt >= grace && !active[strings.ToLower(r.Name)] {
			state.Remove = append(state.Remove, DesiredRemoval{Name: r.Name, Reason: r.Reason})
		}
	}
	slices.SortFunc(state.Active, func(a, b DesiredSkill) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.Item, b.Item)
	})
	slices.SortFunc(state.Remove, func(a, b DesiredRemoval) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	raw, err := json.Marshal([]any{state.Active, state.Remove})
	if err != nil {
		return state, err
	}
	sum := sha256.Sum256(raw)
	state.Version = hex.EncodeToString(sum[:])
	return state, nil
}
