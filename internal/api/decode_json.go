package api

import (
	"encoding/json"
	"io"
	"net/http"
)

// decodeStrictJSON reads exactly one JSON value of at most limit bytes into out.
// An unknown field, trailing data or an oversized body answers 400 with message.
func decodeStrictJSON(w http.ResponseWriter, r *http.Request, limit int64, out any, message string) bool {
	return decodeJSONBody(w, r, limit, out, message, true)
}

// decodeAgentJSON is decodeStrictJSON for what an agent sends on the skills
// protocol: a newer agent may add fields, which this controller ignores.
func decodeAgentJSON(w http.ResponseWriter, r *http.Request, limit int64, out any, message string) bool {
	return decodeJSONBody(w, r, limit, out, message, false)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, limit int64, out any, message string, strict bool) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	if strict {
		d.DisallowUnknownFields()
	}
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		writeErr(w, 400, message)
		return false
	}
	return true
}
