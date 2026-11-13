package terminalproto

// BaseRelation says how the repository's own base branch stood against the
// same branch on GitHub when a worktree's starting commit was chosen.
type BaseRelation string

const (
	// BaseSame: both sides are the same commit.
	BaseSame BaseRelation = "same"
	// BaseBehind: local is an ancestor of GitHub's.
	BaseBehind BaseRelation = "behind"
	// BaseNoLocal: the repository has no local branch of that name.
	BaseNoLocal BaseRelation = "no_local"
	// BaseAhead: local has commits GitHub lacks, and all of GitHub's.
	BaseAhead BaseRelation = "ahead"
	// BaseDiverged: each side has commits the other lacks.
	BaseDiverged BaseRelation = "diverged"
	// BaseFetchFailed: GitHub could not be reached, so the two sides were
	// never compared.
	BaseFetchFailed BaseRelation = "fetch_failed"
	// BaseNoRemote: there is no github.com origin, or it has no such branch;
	// today's local lookup applies.
	BaseNoRemote BaseRelation = "no_remote"
)

// BaseState is what a worktree started from. Start is always set on success.
type BaseState struct {
	Base     string       `json:"base,omitempty"`
	Relation BaseRelation `json:"relation,omitempty"`
	Local    string       `json:"local,omitempty"` // commit ids, 40 or 64 hex
	Remote   string       `json:"remote,omitempty"`
	Start    string       `json:"start,omitempty"`
}

// Valid reports a state a machine may send: a known relation, a starting
// commit, commit ids that are commit ids, and a base git would accept.
func (s BaseState) Valid() bool {
	switch s.Relation {
	case BaseSame, BaseBehind, BaseNoLocal, BaseAhead, BaseDiverged, BaseFetchFailed, BaseNoRemote:
	default:
		return false
	}
	validBase := s.Base == "" || validRefName(s.Base)
	validSides := (s.Local == "" || validCommitID(s.Local)) && (s.Remote == "" || validCommitID(s.Remote))
	return validBase && validSides && validCommitID(s.Start)
}

// validCommitID accepts a lower-case hex commit id, SHA-1 or SHA-256.
func validCommitID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for i := range s {
		b := s[i]
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}
