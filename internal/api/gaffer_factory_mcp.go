package api

import (
	"context"
	"errors"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/store"
)

// The Factory over MCP. Two tools read the outcomes and their pipelines; the
// rest are the person's own controls, offered only to a connection with
// management scope on a writer account. They are the same calls the Factory
// page makes, so the same rules hold: nothing here starts an outcome, answers
// an approval, runs a command or merges anything itself.

var mcpFactoryReads = map[string]bool{"reet_list_factory": true, "reet_get_factory_outcome": true, "reet_list_factory_projects": true}
var mcpFactoryWrites = map[string]bool{
	"reet_move_pipeline_on": true, "reet_retry_pipeline": true, "reet_set_pipeline_merge": true,
	"reet_approve_pipeline_merge": true, "reet_stop_outcome": true, "reet_report_outcome": true,
	"reet_start_outcome": true, "reet_add_pipelines": true, "reet_tell_pipeline_agent": true, "reet_send_pipeline_back": true,
}

// mcpPipelines reads the pipelines an MCP client asks to start: a title, a
// self-contained brief and optional stage kinds. A client never chooses YOLO,
// folder trust or a merge that happens by itself; those stay the person's.
func mcpPipelines(raw any) ([]gaffer.OutcomeSession, error) {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, errors.New("Give at least one pipeline, each with a title and a prompt.")
	}
	out := []gaffer.OutcomeSession{}
	for _, entry := range list {
		item, _ := entry.(map[string]any)
		title, _ := item["title"].(string)
		prompt, _ := item["prompt"].(string)
		piece := gaffer.OutcomeSession{Title: title, Prompt: prompt}
		if kinds, ok := item["stages"].([]any); ok {
			for _, kind := range kinds {
				name, _ := kind.(string)
				piece.Stages = append(piece.Stages, gaffer.OutcomeStageInput{Kind: name})
			}
		}
		if len(piece.Stages) == 0 {
			for _, kind := range []string{"code", "review", "test", "pr", "babysit", "merge"} {
				piece.Stages = append(piece.Stages, gaffer.OutcomeStageInput{Kind: kind})
			}
		}
		out = append(out, piece)
	}
	return out, nil
}

func mcpFactoryTool(name string) bool { return mcpFactoryReads[name] || mcpFactoryWrites[name] }

// mcpOutcome is an outcome as an MCP client reads it. Words taken from
// sessions (stage handovers and Gaffer's report) go only to a connection that
// may also control the Factory.
func (s *server) mcpOutcome(uid int64, o store.GafferOutcome, full bool) map[string]any {
	approvals := "ask"
	if o.Yolo {
		approvals = "yolo"
	}
	pipelines := []map[string]any{}
	for _, piece := range o.Sessions {
		row := map[string]any{"key": piece.Key, "title": piece.Title, "state": piece.State, "note": piece.Note, "updated_at": piece.UpdatedAt}
		if piece.SessionID != "" {
			row["session_id"] = piece.SessionID
		}
		stages := []map[string]any{}
		for i, stage := range piece.Stages {
			item := map[string]any{"kind": stage.Kind, "state": stage.State, "agent": stage.Tool}
			if stage.Model != "" {
				item["model"] = stage.Model
			}
			if stage.Effort != "" {
				item["effort"] = stage.Effort
			}
			if i == piece.Stage && stage.State != "done" {
				item["in_hand"] = true
				row["stage_in_hand"] = stage.Kind
			}
			if stage.StartedAt > 0 {
				item["started_at"] = stage.StartedAt
			}
			if stage.EndedAt > 0 {
				item["ended_at"] = stage.EndedAt
			}
			if stage.ClosedBy != "" {
				item["closed_by"] = stage.ClosedBy
			}
			if stage.AgentState != "" {
				item["agent_state"] = stage.AgentState
			}
			if stage.AgentActiveAt > 0 {
				item["agent_active_at"] = stage.AgentActiveAt
			}
			if stage.AgentUsageReported {
				item["agent_tokens"] = stage.AgentInputTokens + stage.AgentOutputTokens
				if stage.AgentCachedTokens > 0 {
					item["agent_cached_tokens"] = stage.AgentCachedTokens
				}
			}
			if stage.ContextSize > 0 {
				item["context_used"], item["context_size"] = stage.ContextUsed, stage.ContextSize
			}
			if full && stage.Summary != "" {
				item["handover"] = stage.Summary
			}
			if stage.Kind == "merge" {
				row["merge"] = "waits_for_approval"
				if gaffer.MergesByItself(o, piece) {
					row["merge"] = "by_itself_when_green"
				} else if piece.MergeApproved {
					row["merge"] = "approved"
				}
			}
			stages = append(stages, item)
		}
		if len(stages) > 0 {
			row["stages"] = stages
		}
		if piece.ConversationID != "" {
			if g, err := s.st.GafferGoalFor(uid, piece.ConversationID); err == nil {
				row["goal"] = map[string]any{"state": g.State, "note": g.Note, "reviews_used": g.UsedReviews, "reviews_max": g.MaxReviews, "expires_at": g.ExpiresAt, "waiting_on_agent": g.WaitingOnWorker, "review_failed": g.ReviewFailed, "last_reviewed_at": g.LastReviewedAt}
			}
		}
		pipelines = append(pipelines, row)
	}
	out := map[string]any{
		"id": o.ID, "revision": o.Revision, "objective": o.Objective, "state": o.State, "note": o.Note,
		"project": o.ProjectName, "agent": o.Tool, "approvals": approvals, "reports_by_itself": o.AutoReport,
		"created_at": o.CreatedAt, "updated_at": o.UpdatedAt, "pipelines": pipelines, "machine_id": o.AgentID,
	}
	if o.Report != nil {
		report := map[string]any{"at": o.Report.At, "by_itself": o.Report.Auto, "proposals": len(o.Report.Proposals)}
		if full {
			report["text"] = o.Report.Text
		}
		out["report"] = report
	}
	return out
}

var errFactoryForbidden = errors.New("The Factory needs a writer account, and changing it needs a connection with management access.")

func (s *server) executeMCPFactory(ctx context.Context, access mcpAccess, name string, args map[string]any) (map[string]any, error) {
	if access.User.Role == "viewer" || mcpFactoryWrites[name] && !access.canWrite() {
		return nil, errFactoryForbidden
	}
	// The browser's routes are stopped by the fence before they run; these
	// tools arrive as a POST to /mcp, which the fence lets through to be
	// checked per tool, so the check is made here.
	if mcpFactoryWrites[name] {
		if f, err := s.st.Fence(); err != nil || f.Fenced {
			if err != nil {
				return nil, errors.New("this controller cannot tell whether it has moved, so it is not accepting changes")
			}
			return nil, errors.New(fenceMessage(f))
		}
	}
	out, err := s.runMCPFactory(ctx, access, name, args)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errors.New("That outcome is not available to this account. List the Factory to see the outcomes there are.")
	}
	return out, err
}

func (s *server) runMCPFactory(ctx context.Context, access mcpAccess, name string, args map[string]any) (map[string]any, error) {
	uid, full := access.User.ID, access.canWrite()
	str := func(key string) string { v, _ := args[key].(string); return v }
	if name == "reet_list_factory" {
		rows, err := s.gafferProviders.Outcomes(uid)
		if err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, o := range rows {
			out = append(out, s.mcpOutcome(uid, o, false))
		}
		// The builds answer "is this controller, or that machine, new enough
		// for a feature": a client otherwise has to guess from side effects.
		machines := []map[string]any{}
		if agents, err := s.st.SyncAgentsFor(uid); err == nil {
			for _, a := range agents {
				machines = append(machines, map[string]any{"id": a.ID, "name": a.Host, "version": a.Version, "commit": a.Commit, "last_seen": a.LastSeen})
			}
		}
		return map[string]any{"outcomes": out, "controller": map[string]any{"version": s.version, "commit": s.commit}, "machines": machines}, nil
	}
	if name == "reet_list_factory_projects" {
		rows, err := s.st.ListFolderProjectsFor(uid)
		if err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, row := range rows {
			out = append(out, map[string]any{"id": row.ID, "name": row.Name, "machine": row.Host, "git": row.IsRepo})
		}
		return map[string]any{"projects": out}, nil
	}
	id, key := str("outcome_id"), str("pipeline")
	if name == "reet_get_factory_outcome" {
		o, err := s.gafferProviders.Outcome(uid, id)
		if err != nil {
			return nil, err
		}
		return map[string]any{"outcome": s.mcpOutcome(uid, o, full)}, nil
	}
	confirmed := args["confirm"] == true
	needsConfirm := name == "reet_tell_pipeline_agent" || name == "reet_start_outcome" || name == "reet_add_pipelines" || name == "reet_send_pipeline_back" || name == "reet_move_pipeline_on" || name == "reet_approve_pipeline_merge" || name == "reet_stop_outcome"
	if needsConfirm && !confirmed {
		return nil, errors.New("Pass confirm=true only when the user has asked for exactly this. Nothing was changed.")
	}
	// An outcome on YOLO has agents that act without approvals. What they are
	// told, and what joins them, is decided on the Factory page by the person.
	if name == "reet_tell_pipeline_agent" || name == "reet_add_pipelines" || name == "reet_send_pipeline_back" {
		cur, err := s.gafferProviders.Outcome(uid, id)
		if err != nil {
			return nil, err
		}
		if cur.Yolo {
			return nil, errors.New("This outcome runs on YOLO, so its agents act without approvals. Telling them, adding pipelines and sending work back are done on the Factory page. Nothing was changed.")
		}
	}
	if !s.terminalEnabled() && name != "reet_stop_outcome" && name != "reet_set_pipeline_merge" {
		return nil, errors.New("Remote sessions are disabled on this controller, so the Factory cannot start or steer any.")
	}
	// The record moves while pipelines run, so a control that checks the
	// revision is applied to the record as it now stands and tried again if
	// a pipeline stepped in between.
	latest := func(act func(revision int64) (store.GafferOutcome, error)) (store.GafferOutcome, error) {
		for attempt := 0; ; attempt++ {
			cur, err := s.gafferProviders.Outcome(uid, id)
			if err != nil {
				return cur, err
			}
			o, err := act(cur.Revision)
			if !errors.Is(err, store.ErrGafferOutcomeBusy) || attempt >= 4 {
				return o, err
			}
		}
	}
	if name == "reet_tell_pipeline_agent" {
		m, err := s.gafferProviders.TellPipelineAgent(ctx, uid, id, key, str("request_id"), str("text"))
		if err != nil {
			return nil, err
		}
		s.event(uid, "mcp", "mcp-factory-control", "", map[string]any{"connection": access.Connection.Name, "tool": name, "outcome": id, "pipeline": key, "message": m.ID, "state": m.State})
		m.Text = ""
		return map[string]any{"message": m, "applied": true}, nil
	}
	var o store.GafferOutcome
	var err error
	switch name {
	case "reet_start_outcome":
		pipelines, perr := mcpPipelines(args["pipelines"])
		if perr != nil {
			return nil, perr
		}
		// The person's usual limits; no YOLO, no folder trust and no merge by itself can be asked for here.
		o, err = s.gafferProviders.CreateOutcome(uid, gaffer.OutcomeInput{Objective: str("objective"), ProjectID: str("project_id"), Tool: str("agent"), Sessions: pipelines, MaxReviews: 12, Hours: 24, Confirmed: true, AutoReport: args["reports_by_itself"] == true})
		id = o.ID
	case "reet_add_pipelines":
		pipelines, perr := mcpPipelines(args["pipelines"])
		if perr != nil {
			return nil, perr
		}
		o, err = s.gafferProviders.AddPipelines(uid, id, gaffer.AddPipelinesInput{Confirmed: true, Sessions: pipelines})
	case "reet_send_pipeline_back":
		o, err = s.gafferProviders.SendPipelineBack(uid, id, key, str("stage"), str("reason"))
	case "reet_move_pipeline_on":
		o, err = s.gafferProviders.MovePipelineOn(uid, id, key, str("stage"))
	case "reet_retry_pipeline":
		o, err = latest(func(revision int64) (store.GafferOutcome, error) {
			return s.gafferProviders.RetryPipeline(uid, id, key, revision)
		})
	case "reet_set_pipeline_merge":
		// A client may only make a pipeline wait. Letting one merge by itself is the
		// person's own choice, made on the Factory page.
		if mode := str("mode"); mode != "" && mode != "wait" {
			return nil, errors.New("Over MCP a pipeline can only be made to wait for approval. Letting it merge by itself is set on the Factory page.")
		}
		o, err = s.gafferProviders.SetPipelineAutoMerge(uid, id, key, false, false)
	case "reet_approve_pipeline_merge":
		o, err = latest(func(revision int64) (store.GafferOutcome, error) {
			return s.gafferProviders.ApproveMerge(uid, id, key, revision)
		})
	case "reet_stop_outcome":
		// Stopping is everything the person means by it: nothing more starts,
		// and nothing still moving is looked after.
		o, err = latest(func(revision int64) (store.GafferOutcome, error) {
			return s.gafferProviders.StopFollowingOutcome(uid, id, revision)
		})
	case "reet_report_outcome":
		o, err = s.gafferProviders.ReviewOutcome(ctx, gaffer.Principal{UserID: uid, CanUseLocal: access.User.Role == "owner" || access.User.Role == "admin"}, id, 0)
	}
	if err != nil {
		return nil, err
	}
	s.event(uid, "mcp", "mcp-factory-control", "", map[string]any{"connection": access.Connection.Name, "tool": name, "outcome": id, "pipeline": key})
	return map[string]any{"outcome": s.mcpOutcome(uid, o, full), "applied": true}, nil
}
