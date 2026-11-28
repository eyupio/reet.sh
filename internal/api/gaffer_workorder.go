package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/store"
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
		id, note, err := s.workOrderProject(uid, order.Project)
		if err != nil {
			writeErr(w, 500, "Your projects could not be read.")
			return
		}
		out["projectId"] = id
		if note != "" {
			notes = append(notes, note)
		}
	}
	if order.Pathway != "" {
		p, problem, err := s.workOrderPathway(uid, order.Pathway)
		if err != nil {
			gafferError(w, err)
			return
		}
		out["pathwayId"] = p.ID
		if problem != "" {
			problems = append(problems, gaffer.WorkOrderProblem{Line: 0, Text: problem})
		}
	}
	if order.Continues != "" && !s.workOrderRunning(uid, order.Continues) {
		problems = append(problems, gaffer.WorkOrderProblem{Line: 0, Text: workOrderNotRunning})
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

// workOrderProject finds the project a work order names, in any letter case.
// The id is empty, with a note, when none or more than one of the person's
// projects has that name.
func (s *server) workOrderProject(uid int64, name string) (id, note string, err error) {
	rows, err := s.st.ListFolderProjectsFor(uid)
	if err != nil {
		return "", "", err
	}
	var found []string
	for _, p := range rows {
		if strings.EqualFold(p.Name, name) {
			found = append(found, p.ID)
		}
	}
	switch len(found) {
	case 1:
		return found[0], "", nil
	case 0:
		return "", "No project of yours is called “" + name + "”. Choose a project when you approve it.", nil
	}
	return "", "More than one project of yours is called “" + name + "” (the names differ only in letter case). Choose a project when you approve it.", nil
}

// workOrderPathway finds the switched-on pathway a work order names, by id or
// by name in any case. With none, the problem says which are on.
func (s *server) workOrderPathway(uid int64, name string) (found store.GafferPathway, problem string, err error) {
	rows, err := s.gafferProviders.Pathways(uid)
	if err != nil {
		return found, "", err
	}
	on := []string{}
	for _, p := range rows {
		if !p.Enabled {
			continue
		}
		on = append(on, p.Name)
		if found.ID == "" && (p.ID == name || strings.EqualFold(p.Name, name)) {
			found = p
		}
	}
	if found.ID != "" {
		return found, "", nil
	}
	problem = "No pathway called “" + name + "” is switched on."
	if len(on) > 0 {
		problem += " The ones that are: " + strings.Join(on, ", ") + "."
	}
	return found, problem, nil
}

// workOrderRunning says whether an outcome of the person's can still be
// continued. One answer for not yours, not there, archived and finished, so
// nothing leaks.
func (s *server) workOrderRunning(uid int64, id string) bool {
	o, err := s.gafferProviders.Outcome(uid, id)
	return err == nil && o.ArchivedAt == 0 && o.State != "stopped" && !o.Finished()
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
