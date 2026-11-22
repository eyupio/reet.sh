package api

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	// Marshal first: encoding/json escapes <, > and & in strings, and a
	// failure never leaves half an object on the wire.
	data, err := json.Marshal(v)
	if err == nil {
		data = gzipJSON(w, code, data)
	}
	w.WriteHeader(code)
	if err == nil {
		_, _ = w.Write(data)
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// writeErrCode keeps the legacy error string and adds machine-readable fields
// so MCP clients can tell the agent what to change instead of retrying blindly.
func writeErrCode(w http.ResponseWriter, code int, errCode, field, msg, hint string) {
	out := map[string]string{"error": msg, "code": errCode, "message": msg}
	if field != "" {
		out["field"] = field
	}
	if hint != "" {
		out["hint"] = hint
	}
	writeJSON(w, code, out)
}

const (
	errConfirmRequired      = "confirm_required"
	errEnvironmentHasSkills = "environment_has_skills"
	errEnvironmentNotFound  = "environment_not_found"
	errLocalSkillInList     = "local_skill_not_allowed_in_list"
	errAmbiguousSkillID     = "ambiguous_skill_id"
	errSkillNotFound        = "skill_not_found"
	errListNotFound         = "list_not_found"
	errTagNotFound          = "tag_not_found"
	errInvalidArgument      = "invalid_argument"
	errReadOnly             = "read_only"
	errConflict             = "conflict"
	errNoMatchingSkills     = "no_matching_skills"
	errInvalidField         = "invalid_field"
)

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}
