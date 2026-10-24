package api

import (
	"errors"
	"net/http"

	"github.com/eyupio/reet.sh/internal/skillplan"
	"github.com/eyupio/reet.sh/internal/store"
)

// Ownership records are the library's claim that a skill folder on one machine
// holds files it provably published. Browsers adopt a reviewed preview and release
// a record; neither can name a file, choose a path or touch a machine.
func (s *server) registerSkillContentReceipts(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/skill-content-receipts", s.privateSkillAccess(s.skillContentReceipts))
	mux.HandleFunc("POST /api/v2/skill-content-receipts", s.privateSkillAccess(s.skillContentReceiptAdopt))
	mux.HandleFunc("GET /api/v2/skill-content-receipts/{id}", s.privateSkillAccess(s.skillContentReceiptGet))
	mux.HandleFunc("DELETE /api/v2/skill-content-receipts/{id}", s.privateSkillAccess(s.skillContentReceiptRelease))
}

func skillContentReceiptError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrSkillContentReceiptInvalid):
		writeErr(w, 400, "Supply a request key, a captured preview with its review hash, and the library revision it was installed from.")
	case errors.Is(err, store.ErrSkillContentReceiptMissing), errors.Is(err, store.ErrSkillContentPreviewMissing):
		writeErr(w, 404, "Ownership record or preview unavailable.")
	case errors.Is(err, skillplan.ErrNotClaimable):
		writeErr(w, 409, "That revision does not match this folder's SKILL.md, so nothing can be claimed.")
	case errors.Is(err, store.ErrSkillContentReceiptCapacity):
		writeErr(w, 409, "Ownership record limit reached (1024 per account).")
	case errors.Is(err, store.ErrSkillContentReceipt), errors.Is(err, store.ErrSkillContentPreview), errors.Is(err, store.ErrSkillContentTargetConflict),
		errors.Is(err, store.ErrSkillContentTargetScope), errors.Is(err, store.ErrContentObservationStale), errors.Is(err, store.ErrRemoteNotFound):
		writeErr(w, 409, "This ownership record is unavailable, blocked, changed or already exists. Preview again, and release existing ownership before adopting again.")
	default:
		privateSkillError(w, err)
	}
}

func (s *server) skillContentReceipts(w http.ResponseWriter, r *http.Request) {
	receipts, err := s.st.SkillContentReceiptsFor(s.uid(r))
	if err != nil {
		skillContentReceiptError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"receipts": receipts})
}

func (s *server) skillContentReceiptGet(w http.ResponseWriter, r *http.Request) {
	out, err := s.st.SkillContentReceiptFor(s.uid(r), r.PathValue("id"))
	if err != nil {
		skillContentReceiptError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

func (s *server) skillContentReceiptAdopt(w http.ResponseWriter, r *http.Request) {
	var in store.SkillContentAdopt
	if !decodeStrictJSON(w, r, 16<<10, &in, "Invalid or oversized ownership record request.") {
		return
	}
	out, err := s.st.AdoptSkillContentPreviewFor(s.uid(r), in)
	if err != nil {
		skillContentReceiptError(w, err)
		return
	}
	// The record is already committed; the event is a convenience trail of ids and counts.
	s.eventRequest(r, "skill-content-adopt", "", map[string]any{"name": out.Name, "files": out.FileCount, "receiptId": out.ID})
	writeJSON(w, 200, out)
}

func (s *server) skillContentReceiptRelease(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ExpectedVersion *string `json:"expectedVersion"`
	}
	if !decodeStrictJSON(w, r, 4096, &in, "Invalid or oversized ownership record request.") {
		return
	}
	if in.ExpectedVersion == nil {
		skillContentReceiptError(w, store.ErrSkillContentReceiptInvalid)
		return
	}
	uid, id := s.uid(r), r.PathValue("id")
	// The name comes from the list, whose summaries also cover a damaged record:
	// an item read refuses one, and a damaged record must stay releasable.
	name := ""
	if receipts, err := s.st.SkillContentReceiptsFor(uid); err == nil {
		for _, receipt := range receipts {
			if receipt.ID == id {
				name = receipt.Name
			}
		}
	}
	if err := s.st.ReleaseSkillContentReceiptFor(uid, id, *in.ExpectedVersion); err != nil {
		skillContentReceiptError(w, err)
		return
	}
	s.eventRequest(r, "skill-content-release", "", map[string]any{"name": name, "receiptId": id})
	writeJSON(w, 200, map[string]bool{"removed": true})
}
