package api

import (
	"net/http"
	"strings"

	"github.com/eyupio/reet.sh/internal/gaffer"
)

// gafferWorkOrderCheckResponse is the check route's one answer shape: the
// parsed plan (or null, with the parser's own problems, when the text is not
// a sound work order), the caller's project and pathway matched by name, and
// anything else worth a note rather than a refusal.
type gafferWorkOrderCheckResponse struct {
	Order     *gaffer.WorkOrder         `json:"order"`
	Problems  []gaffer.WorkOrderProblem `json:"problems"`
	ProjectID string                    `json:"projectId"`
	PathwayID string                    `json:"pathwayId"`
	Notes     []string                  `json:"notes"`
}

// gafferWorkOrderCheck is the dry run behind both the paste and MCP routes:
// it parses the text, matches its project and pathway hints against the
// caller's own, and checks continues against the caller's own outcomes.
// Nothing is started and no model is asked anything.
func (s *server) gafferWorkOrderCheck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if !decodeStrictJSON(w, r, gaffer.MaxWorkOrderBytes+1024, &in, "That work order is invalid or too large.") {
		return
	}
	order, problems := gaffer.ParseWorkOrder(in.Text)
	out := gafferWorkOrderCheckResponse{Problems: []gaffer.WorkOrderProblem{}, Notes: []string{}}
	if problems != nil {
		// A work order the parser itself refused has no reliable plan to check
		// a project, pathway or continues hint against.
		out.Problems = problems
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, out)
		return
	}
	out.Order = &order
	uid := s.uid(r)
	if order.Project != "" {
		projects, err := s.st.ListFolderProjectsFor(uid)
		if err != nil {
			writeErr(w, 500, "Projects could not be read.")
			return
		}
		matched := false
		for _, p := range projects {
			if strings.EqualFold(p.Name, order.Project) {
				out.ProjectID, matched = p.ID, true
				break
			}
		}
		if !matched {
			out.Notes = append(out.Notes, "No project is named \""+order.Project+"\"; choose one yourself.")
		}
	}
	if order.Pathway != "" {
		pathways, err := s.st.GafferPathwaysFor(uid)
		if err != nil {
			writeErr(w, 500, "Pathways could not be read.")
			return
		}
		matched := false
		for _, p := range pathways {
			if strings.EqualFold(p.Name, order.Pathway) {
				matched = true
				if p.Enabled {
					out.PathwayID = p.ID
				} else {
					out.Problems = append(out.Problems, gaffer.WorkOrderProblem{Text: "The \"" + order.Pathway + "\" pathway is switched off."})
				}
				break
			}
		}
		if !matched {
			out.Notes = append(out.Notes, "No pathway is named \""+order.Pathway+"\"; choose one yourself.")
		}
	}
	if order.Continues != "" {
		// Never says whether the id belongs to someone else or does not exist:
		// Outcome(uid, id) already cannot return another account's outcome.
		o, err := s.gafferProviders.Outcome(uid, order.Continues)
		if err != nil || o.ArchivedAt > 0 || o.State == "stopped" {
			out.Problems = append(out.Problems, gaffer.WorkOrderProblem{Text: "continues must name a running, unarchived outcome of this account."})
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, out)
}

// gafferWorkReport is the work report for one of the caller's own outcomes,
// in the same Markdown-with-a-header style a work order is read in. The
// person's own copy always carries handover text, and this route is always
// the person's own copy, never a management-restricted MCP connection.
func (s *server) gafferWorkReport(w http.ResponseWriter, r *http.Request) {
	o, err := s.gafferProviders.Outcome(s.uid(r), r.PathValue("id"))
	if err != nil {
		gafferOutcomeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(gaffer.WorkReport(o, true)))
}
