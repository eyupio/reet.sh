package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/eyupio/reet.sh/internal/gaffer"
)

const workOrderNotRunning = "No running outcome of yours has that id. Copy a fresh report from the outcome and use its id."

// gafferWorkOrderCheck reads a pasted work order and says what it would do.
// It starts nothing and makes no model request.
func (s *server) gafferWorkOrderCheck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, gaffer.MaxWorkOrderBytes+1024)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeErr(w, 413, "The work order is over 64 KB. Shorten the briefs.")
			return
		}
		writeErr(w, 400, "That work order request is invalid.")
		return
	}
	uid := s.uid(r)
	order, problems := gaffer.ParseWorkOrder(in.Text)
	out := map[string]any{"order": nil, "problems": []gaffer.WorkOrderProblem{}, "projectId": "", "pathwayId": "", "notes": []string{}}
	w.Header().Set("Cache-Control", "no-store")
	if len(problems) > 0 {
		out["problems"] = problems
		writeJSON(w, 200, out)
		return
	}
	var notes []string
	if order.Project != "" {
		rows, err := s.st.ListFolderProjectsFor(uid)
		if err != nil {
			writeErr(w, 500, "Your projects could not be read.")
			return
		}
		var found []string
		for _, p := range rows {
			if strings.EqualFold(p.Name, order.Project) {
				found = append(found, p.ID)
			}
		}
		switch len(found) {
		case 1:
			out["projectId"] = found[0]
		case 0:
			notes = append(notes, "No project of yours is called “"+order.Project+"”. Choose a project when you approve it.")
		default:
			notes = append(notes, "More than one project of yours is called “"+order.Project+"” (the names differ only in letter case). Choose a project when you approve it.")
		}
	}
	if order.Pathway != "" {
		rows, err := s.gafferProviders.Pathways(uid)
		if err != nil {
			gafferError(w, err)
			return
		}
		on := []string{}
		for _, p := range rows {
			if !p.Enabled {
				continue
			}
			on = append(on, p.Name)
			if out["pathwayId"] == "" && (p.ID == order.Pathway || strings.EqualFold(p.Name, order.Pathway)) {
				out["pathwayId"] = p.ID
			}
		}
		if out["pathwayId"] == "" {
			text := "No pathway called “" + order.Pathway + "” is switched on."
			if len(on) > 0 {
				text += " The ones that are: " + strings.Join(on, ", ") + "."
			}
			problems = append(problems, gaffer.WorkOrderProblem{Line: 0, Text: text})
		}
	}
	if order.Continues != "" {
		// One sentence for not yours, not there, archived and finished, so nothing leaks.
		if o, err := s.gafferProviders.Outcome(uid, order.Continues); err != nil || o.ArchivedAt > 0 || o.State == "stopped" || o.Finished() {
			problems = append(problems, gaffer.WorkOrderProblem{Line: 0, Text: workOrderNotRunning})
		}
	}
	if len(problems) > 0 {
		out["problems"] = problems
		writeJSON(w, 200, out)
		return
	}
	if notes != nil {
		out["notes"] = notes
	}
	out["order"] = order
	writeJSON(w, 200, out)
}

// gafferWorkReport is an outcome as the Markdown an outside agent reads.
func (s *server) gafferWorkReport(w http.ResponseWriter, r *http.Request) {
	o, err := s.gafferProviders.Outcome(s.uid(r), r.PathValue("id"))
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(gaffer.WorkReport(o, true)))
}
