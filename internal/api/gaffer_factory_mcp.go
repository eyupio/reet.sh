package api

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// The Factory over MCP. Two tools read the outcomes and their pipelines; the
// rest are the person's own controls, offered only to a connection with
// management scope on a writer account. They are the same calls the Factory
// page makes, so the same rules hold: nothing here starts an outcome, answers
// an approval, runs a command or merges anything itself.

var mcpFactoryReads = map[string]bool{"reet_list_factory": true, "reet_get_factory_outcome": true, "reet_list_factory_projects": true, "reet_list_pathways": true}
var mcpFactoryWrites = map[string]bool{
	"reet_move_pipeline_on": true, "reet_retry_pipeline": true, "reet_set_pipeline_merge": true,
	"reet_approve_pipeline_merge": true, "reet_stop_outcome": true, "reet_report_outcome": true,
	"reet_start_outcome": true, "reet_add_pipelines": true, "reet_tell_pipeline_agent": true, "reet_send_pipeline_back": true,
}

// mcpPipelines reads the pipelines an MCP client asks to start: a title, a
// self-contained brief, optional stage kinds and, for any stage, how
// demanding it is. A client names no agent for a stage: its tier's agent and
// backup come from the person's saved crew. It never chooses YOLO, folder
// trust, a merge that happens by itself or who takes the decisions; those
// stay the person's. A pipeline started on a pathway takes the pathway's
// stages, so it gives none of its own and is not given the usual ones.
func mcpPipelines(raw any, onPathway bool) ([]gaffer.OutcomeSession, error) {
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
		if onPathway {
			if item["stages"] != nil || item["tiers"] != nil || item["parts"] != nil {
				return nil, errors.New("A pipeline takes a pathway or its own stages, not both. Nothing was started.")
			}
			out = append(out, piece)
			continue
		}
		tiers, _ := item["tiers"].(map[string]any)
		for kind, tier := range tiers {
			name, _ := tier.(string)
			if name != "light" && name != "standard" && name != "heavy" {
				return nil, errors.New("A stage’s tier is light, standard or heavy. \"" + kind + "\" in “" + title + "” has another.")
			}
		}
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
		// A stage can be split into named parts, each with a brief and, if it
		// differs, a tier. The parts replace the stage, in the order given.
		if list, ok := item["parts"].([]any); ok && len(list) > 0 {
			split, order := map[string][]gaffer.OutcomeStageInput{}, []string{}
			for _, entry := range list {
				part, _ := entry.(map[string]any)
				kind, _ := part["stage"].(string)
				name, _ := part["title"].(string)
				brief, _ := part["brief"].(string)
				tier, _ := part["tier"].(string)
				if len(split[kind]) == 0 {
					order = append(order, kind)
				}
				split[kind] = append(split[kind], gaffer.OutcomeStageInput{Kind: kind, Part: name, Brief: brief, Tier: tier})
			}
			for _, kind := range order {
				at := -1
				for i := range piece.Stages {
					if piece.Stages[i].Kind == kind {
						at = i
					}
				}
				if at < 0 || len(split[kind]) < 2 {
					return nil, errors.New("“" + title + "” splits a \"" + kind + "\" stage it does not have, or gives it only one part. A split is two to four parts of a stage the pipeline runs.")
				}
				for i := range split[kind] {
					if split[kind][i].Tier == "" {
						if tier, ok := tiers[kind].(string); ok {
							split[kind][i].Tier = tier
						}
					}
				}
				piece.Stages = append(piece.Stages[:at], append(split[kind], piece.Stages[at+1:]...)...)
			}
		}
		named := map[string]bool{}
		for i := range piece.Stages {
			named[piece.Stages[i].Kind] = true
			if tier, ok := tiers[piece.Stages[i].Kind].(string); ok && piece.Stages[i].Part == "" {
				piece.Stages[i].Tier = tier
			}
		}
		for kind := range tiers {
			if !named[kind] {
				return nil, errors.New("“" + title + "” gives a tier for a \"" + kind + "\" stage it does not have.")
			}
		}
		out = append(out, piece)
	}
	return out, nil
}

func mcpFactoryTool(name string) bool { return mcpFactoryReads[name] || mcpFactoryWrites[name] }

// mcpHandoverBytes is the most of a stage's handover note a read carries: the
// notes can be 4,000 bytes each, and a reply is capped at 25,000 characters.
const mcpHandoverBytes = 600

// mcpOutcomeMore is said with every outcome in short, so a client knows where the rest is.
const mcpOutcomeMore = "This is the outcome in short. reet_get_factory_outcome has every stage."

// mcpOutcomeRow is an outcome in short, as a list row and as the answer to a
// control: where it stands and which stage each pipeline is at, in a few
// hundred bytes however long the pipelines are. It carries no session text.
func mcpOutcomeRow(o store.GafferOutcome) map[string]any {
	pipelines := []map[string]any{}
	for _, piece := range o.Sessions {
		row := map[string]any{"key": piece.Key, "title": piece.Title, "state": piece.State, "updated_at": piece.UpdatedAt}
		if piece.Stage < len(piece.Stages) && piece.Stages[piece.Stage].State != "done" {
			row["stage_in_hand"] = piece.Stages[piece.Stage].Label()
		}
		pipelines = append(pipelines, row)
	}
	row := map[string]any{"id": o.ID, "state": o.State, "objective": truncate(o.Objective, 200), "project": o.ProjectName, "updated_at": o.UpdatedAt, "pipelines": pipelines}
	if o.ArchivedAt > 0 {
		row["archived_at"] = o.ArchivedAt
	}
	return row
}

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
		if piece.PathwayName != "" {
			row["pathway"] = piece.PathwayName
		}
		if piece.SessionID != "" {
			row["session_id"] = piece.SessionID
		}
		// A pipeline with no stages says on itself whether the outcome's agent and model were applied.
		if piece.Applied != nil {
			row["choice"] = piece.Applied.Check
			if piece.Applied.Model != "" {
				row["applied_model"] = piece.Applied.Model
			}
			if piece.Applied.Effort != "" {
				row["applied_effort"] = piece.Applied.Effort
			}
			if piece.Applied.Note != "" {
				row["choice_note"] = piece.Applied.Note
			}
		}
		stages := []map[string]any{}
		for i, stage := range piece.Stages {
			item := map[string]any{"kind": stage.Kind, "name": stage.Label(), "state": stage.State, "agent": stage.Tool}
			if stage.Model != "" {
				item["model"] = stage.Model
			}
			if stage.Effort != "" {
				item["effort"] = stage.Effort
			}
			// One part of a stage that was split: parts of a kind sit together and run in order.
			if stage.Part != "" {
				item["part"] = stage.Part
			}
			// How demanding the stage was judged to be, which chose its agent from the person's crew.
			if stage.Tier != "" {
				item["tier"] = stage.Tier
			}
			// A stage that started on its backup because its first choice's plan
			// was running low says so, with what it would otherwise have run on.
			if stage.Backup != nil {
				item["backup_agent"] = stage.Backup.Tool
			}
			if stage.Balanced != "" {
				item["balanced"] = stage.Balanced
				if stage.First != nil {
					item["first_choice_agent"] = stage.First.Tool
					if stage.First.Model != "" {
						item["first_choice_model"] = stage.First.Model
					}
				}
			}
			// agent, model and effort are what was chosen. choice says whether the
			// stage's session was read back to be on them: applied, mismatch or
			// unchecked, with what it is on and, for a mismatch, what differs.
			if stage.ChoiceCheck != "" {
				item["choice"] = stage.ChoiceCheck
				if stage.AppliedModel != "" {
					item["applied_model"] = stage.AppliedModel
				}
				if stage.AppliedEffort != "" {
					item["applied_effort"] = stage.AppliedEffort
				}
				if stage.ChoiceNote != "" {
					item["choice_note"] = stage.ChoiceNote
				}
			}
			// The effort chosen for the stage, when its agent did not offer it
			// for the model on that machine: the stage then started on the
			// agent's default effort, and effort above is empty.
			if stage.EffortNotOffered != "" {
				item["effort_not_offered"] = stage.EffortNotOffered
			}
			if i == piece.Stage && stage.State != "done" {
				item["in_hand"] = true
				row["stage_in_hand"] = stage.Label()
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
				item["handover"] = truncate(stage.Summary, mcpHandoverBytes)
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
	// Who takes the decisions when a stage stops: the person, or Gaffer for
	// them. What Gaffer decided is listed; its reasons are words about
	// sessions, so they go only to a connection that may control the Factory.
	out["balance_below"] = gaffer.DefaultBalanceBelow
	if o.BalanceBelow > 0 {
		out["balance_below"] = o.BalanceBelow
	}
	out["decisions_by"] = "you"
	if o.Automation {
		out["decisions_by"] = "gaffer"
	}
	if len(o.Decisions) > 0 {
		taken := []map[string]any{}
		for _, d := range o.Decisions {
			item := map[string]any{"at": d.At, "pipeline": d.Key, "stage": d.Stage, "about": d.About, "choice": d.Choice, "failed": d.Failed}
			if full {
				item["why"] = d.Why
			}
			taken = append(taken, item)
		}
		out["gaffer_decisions"] = taken
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
	// A stage agent uses the same connection as the person's own client on
	// its machine, so a connection approves a merge only when the person has
	// allowed it to. Gaffer's own tools and the signed-in browser are the
	// person's and are not connections.
	inApp := access.Connection.ID == 0 && (access.Connection.Kind == "gaffer" || access.Connection.Kind == "browser")
	if name == "reet_approve_pipeline_merge" && !inApp && !access.Connection.ApproveMerges {
		return nil, errors.New("This connection may not approve merges. Approve it in reet, or allow merge approval for this connection under Account → MCP access. Nothing was approved.")
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
	// Handover notes and reports are session text. Gaffer's own call returns
	// them only when the provider that will read the result is allowed it.
	uid, full := access.User.ID, access.canWrite() && (access.Connection.Kind != "gaffer" || gaffer.SessionTextAllowed(ctx))
	str := func(key string) string { v, _ := args[key].(string); return v }
	if name == "reet_list_factory" {
		rows, err := s.gafferProviders.Outcomes(uid)
		if err != nil {
			return nil, err
		}
		out := []any{}
		for _, o := range rows {
			if o.ArchivedAt == 0 || args["include_archived"] == true {
				out = append(out, mcpOutcomeRow(o))
			}
		}
		// The builds answer "is this controller, or that machine, new enough
		// for a feature": a client otherwise has to guess from side effects.
		machines := []map[string]any{}
		if agents, err := s.st.SyncAgentsFor(uid); err == nil {
			for _, a := range agents {
				machines = append(machines, map[string]any{"id": a.ID, "name": a.Host, "version": a.Version, "commit": a.Commit, "last_seen": a.LastSeen})
			}
		}
		return mcpPage(out, args, map[string]any{"controller": map[string]any{"version": s.version, "commit": s.commit}, "machines": machines})
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
	// A pathway's stages are listed by name; their prompts are not.
	if name == "reet_list_pathways" {
		rows, err := s.gafferProviders.Pathways(uid)
		if err != nil {
			return nil, err
		}
		out := []any{}
		for _, p := range rows {
			if !p.Enabled {
				continue
			}
			stages := []string{}
			for _, st := range p.Stages {
				stages = append(stages, store.GafferOutcomeStage{Kind: st.Kind, Name: st.Name}.Label())
			}
			problems := s.gafferProviders.PathwayProblems(p, s.templateSkills(uid))
			if problems == nil {
				problems = []string{}
			}
			out = append(out, map[string]any{"id": p.ID, "name": p.Name, "description": p.Description, "stages": stages, "problems": problems})
		}
		// Fifty pathways of twelve named stages do not fit one answer, so they are paged.
		return mcpPage(out, args, nil)
	}
	id, key := str("outcome_id"), str("pipeline")
	if name == "reet_get_factory_outcome" {
		o, err := s.gafferProviders.Outcome(uid, id)
		if err != nil {
			return nil, err
		}
		// An outcome of many pipelines can be too long to read whole, so one can be read alone.
		if key != "" {
			o.Sessions = slices.DeleteFunc(o.Sessions, func(piece store.GafferOutcomeSession) bool { return piece.Key != key })
			if len(o.Sessions) == 0 {
				return nil, errors.New("That outcome has no pipeline \"" + key + "\". reet_list_factory has the keys of its pipelines.")
			}
		}
		return map[string]any{"outcome": s.mcpOutcome(uid, o, full)}, nil
	}
	confirmed := args["confirm"] == true
	needsConfirm := name == "reet_tell_pipeline_agent" || name == "reet_start_outcome" || name == "reet_add_pipelines" || name == "reet_send_pipeline_back" || name == "reet_move_pipeline_on" || name == "reet_approve_pipeline_merge" || name == "reet_stop_outcome"
	if needsConfirm && !confirmed {
		return nil, errors.New("Pass confirm=true only when the user has asked for exactly this. Nothing was changed.")
	}
	// An outcome on YOLO has agents that act without approvals. Whatever sets
	// one going, moves it on or lets it merge is decided on the Factory page
	// by the person: what its agents are told, what joins them, a stage sent
	// back, a pipeline tried again or moved on, and a merge approved. A client
	// can still read it, make a pipeline wait for approval, ask for a report
	// and stop it.
	if name == "reet_tell_pipeline_agent" || name == "reet_add_pipelines" || name == "reet_send_pipeline_back" ||
		name == "reet_retry_pipeline" || name == "reet_move_pipeline_on" || name == "reet_approve_pipeline_merge" {
		cur, err := s.gafferProviders.Outcome(uid, id)
		if err != nil {
			return nil, err
		}
		if cur.Yolo {
			return nil, errors.New("This outcome runs on YOLO, so its agents act without approvals. Telling them, adding pipelines, sending work back, trying a pipeline again, moving it on and approving its merge are done on the Factory page. Nothing was changed.")
		}
		if name == "reet_send_pipeline_back" || name == "reet_move_pipeline_on" {
			if err := mcpStageNamed(cur, key, str("stage")); err != nil {
				return nil, err
			}
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
		if errors.Is(err, store.ErrNotFound) {
			// The outcome was read above, so what is missing is the session's chat.
			return nil, errors.New("That pipeline’s session can no longer be told anything: its chat is not available to Gaffer. No instruction was queued.")
		}
		if err != nil {
			return nil, err
		}
		s.event(uid, "mcp", "mcp-factory-control", "", map[string]any{"connection": access.Connection.Name, "tool": name, "outcome": id, "pipeline": key, "message": m.ID, "state": m.State})
		m.Text = ""
		// A receipt is not a delivery: an instruction the agent could not take
		// (it is waiting on an approval, say) was not queued, and is not
		// reported as applied. The same request_id reads that receipt again, so
		// trying once more takes a new one.
		out := map[string]any{"message": m, "applied": m.State != "blocked" && m.State != "failed"}
		if out["applied"] == false {
			out["note"] = "No instruction was queued. To try again, use a new request_id: this one keeps this receipt."
		}
		return out, nil
	}
	var o store.GafferOutcome
	var err error
	// applied says whether the call changed anything; a control that found
	// nothing to do says so instead of reporting a change.
	applied := true
	switch name {
	case "reet_start_outcome":
		pipelines, perr := mcpPipelines(args["pipelines"], str("pathway") != "")
		if perr != nil {
			return nil, perr
		}
		// The browser offers only the agents the project's machine can run as a
		// chat. A client names one blind, so it is checked here: an outcome whose
		// every pipeline would fail at launch is refused before it is saved.
		// A machine that has not said what it can run is not taken to be able:
		// the check refuses what it cannot confirm.
		if project, perr := s.st.FolderProjectFor(uid, str("project_id")); perr == nil {
			caps, cerr := s.st.RemoteCapabilitiesFor(uid, project.Host)
			if cerr != nil || !caps.Capabilities.ACP {
				return nil, errors.New("This project’s machine has not reported that it can run agent chats: it is offline, has never connected, or is too old. Nothing was started.")
			}
			if _, ok := caps.Capabilities.ChatAgent(str("agent")); !ok && terminalproto.ReviewedChatTool(str("agent")) {
				return nil, errors.New("That agent cannot run a Gaffer chat on this project's machine right now: it is not installed there, is out of date, or its chat is switched off for this account. Nothing was started.")
			}
		}
		// A client that did not hear back asks again under the same request_id
		// and is answered with the outcome that request already started.
		if rid := str("request_id"); rid != "" {
			if existing, eerr := s.gafferProviders.Outcome(uid, rid); eerr == nil {
				return map[string]any{"outcome": mcpOutcomeRow(existing), "applied": false, "note": "This request_id already started this outcome. Nothing more was started. " + mcpOutcomeMore}, nil
			}
		}
		// The person's usual limits; no YOLO, no folder trust and no merge by itself can be asked for here.
		// When a backup takes over is the client's to say, within the person's bounds; who decides is not.
		balanceBelow := 0
		if n, ok := args["balance_below"].(float64); ok {
			balanceBelow = int(n)
			if float64(balanceBelow) != n || balanceBelow < gaffer.MinBalanceBelow || balanceBelow > gaffer.MaxBalanceBelow {
				return nil, errors.New("balance_below is a whole number from 5 to 90.")
			}
		}
		pathway, perr := s.mcpUsePathway(uid, str("pathway"), pipelines)
		if perr != nil {
			return nil, perr
		}
		o, err = s.gafferProviders.CreateOutcome(uid, gaffer.OutcomeInput{Objective: str("objective"), ProjectID: str("project_id"), Tool: str("agent"), Model: str("model"), Sessions: pipelines, BalanceBelow: balanceBelow, MaxReviews: 12, Hours: 24, Confirmed: true, AutoReport: args["reports_by_itself"] == true, RequestID: str("request_id")})
		id = o.ID
		if err == nil {
			s.recordPathwaySkills(uid, pathway, o, len(pipelines))
		}
	case "reet_add_pipelines":
		pipelines, perr := mcpPipelines(args["pipelines"], str("pathway") != "")
		if perr != nil {
			return nil, perr
		}
		pathway, perr := s.mcpUsePathway(uid, str("pathway"), pipelines)
		if perr != nil {
			return nil, perr
		}
		o, err = s.gafferProviders.AddPipelines(uid, id, gaffer.AddPipelinesInput{Confirmed: true, Sessions: pipelines})
		if err == nil {
			s.recordPathwaySkills(uid, pathway, o, len(pipelines))
		}
	case "reet_send_pipeline_back":
		o, err = s.gafferProviders.SendPipelineBack(uid, id, key, str("stage"), str("reason"))
	case "reet_move_pipeline_on":
		// The stage is what makes this the move the caller read: without it a pipeline that has moved on since would be moved again.
		if str("stage") == "" {
			return nil, errors.New("Pass stage, the stage you read as in hand, so a pipeline that has moved on since is not moved again. Nothing was changed.")
		}
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
		// An approval is of the pipeline as the caller read it. One that has
		// changed since (sent back and done again, say) is different work, so
		// an approval made earlier, or one Gaffer asked for and the person
		// answered later, does not carry over to it.
		seen, _ := args["seen_updated_at"].(float64)
		stale := errors.New("That pipeline has changed since it was read, or seen_updated_at (the pipeline’s updated_at from that read) was not given. Nothing was approved. Read the outcome again and approve what is there now.")
		if seen < 1 {
			return nil, stale
		}
		o, err = latest(func(revision int64) (store.GafferOutcome, error) {
			cur, err := s.gafferProviders.Outcome(uid, id)
			if err != nil {
				return cur, err
			}
			for _, piece := range cur.Sessions {
				if piece.Key == key && piece.UpdatedAt != int64(seen) {
					return cur, stale
				}
			}
			return s.gafferProviders.ApproveMerge(uid, id, key, cur.Revision)
		})
	case "reet_stop_outcome":
		// Stopping is everything the person means by it: nothing more starts,
		// and nothing still moving is looked after.
		var before int64
		o, err = latest(func(revision int64) (store.GafferOutcome, error) {
			before = revision
			return s.gafferProviders.StopFollowingOutcome(uid, id, revision)
		})
		// An outcome that had already finished is left as it is.
		applied = err == nil && o.Revision != before
	case "reet_report_outcome":
		o, err = s.gafferProviders.ReviewOutcome(ctx, gaffer.Principal{UserID: uid, CanUseLocal: access.User.Role == "owner" || access.User.Role == "admin"}, id, 0)
	}
	if err != nil {
		return nil, err
	}
	if !applied {
		return map[string]any{"outcome": mcpOutcomeRow(o), "applied": false, "note": "Nothing was changed: this outcome had already finished. " + mcpOutcomeMore}, nil
	}
	s.event(uid, "mcp", "mcp-factory-control", "", map[string]any{"connection": access.Connection.Name, "tool": name, "outcome": id, "pipeline": key})
	return map[string]any{"outcome": mcpOutcomeRow(o), "applied": true, "note": mcpOutcomeMore}, nil
}

// mcpUsePathway gives the pipelines the stages of the enabled pathway a client
// named by its id or its name in any case, and returns its id. No name, no
// pathway: the pipelines are left as they are.
func (s *server) mcpUsePathway(uid int64, named string, pipelines []gaffer.OutcomeSession) (string, error) {
	if named == "" {
		return "", nil
	}
	rows, err := s.gafferProviders.Pathways(uid)
	if err != nil {
		return "", err
	}
	// An id is matched before any name, so a pathway named with another's id
	// is not taken for it.
	id, on := "", []string{}
	for _, p := range rows {
		if p.Enabled && p.ID == named {
			id = p.ID
		}
	}
	for _, p := range rows {
		if !p.Enabled {
			continue
		}
		if id == "" && strings.EqualFold(p.Name, named) {
			id = p.ID
		}
		on = append(on, p.Name)
	}
	if id == "" && len(on) == 0 {
		return "", errors.New("No pathway called “" + named + "” is switched on, and no other is either. Nothing was started.")
	}
	if id == "" {
		return "", errors.New("No pathway called “" + named + "” is switched on. The ones that are: " + strings.Join(on, ", ") + ". Nothing was started.")
	}
	if err := s.usePathway(uid, id, pipelines); err != nil {
		var refused *gaffer.Invalid
		if errors.As(err, &refused) {
			return "", errors.New(err.Error() + " Nothing was started.")
		}
		return "", err
	}
	return id, nil
}

// mcpStageNamed refuses a stage the pipeline does not have, naming those it
// has. A pipeline that is not there is left for the control to refuse.
func mcpStageNamed(o store.GafferOutcome, key, stage string) error {
	for _, piece := range o.Sessions {
		if piece.Key != key || stage == "" {
			continue
		}
		labels := []string{}
		for _, st := range piece.Stages {
			if st.Label() == stage {
				return nil
			}
			if !slices.Contains(labels, st.Label()) {
				labels = append(labels, st.Label())
			}
		}
		return errors.New("That pipeline has no “" + stage + "” stage. Its stages are: " + strings.Join(labels, ", ") + ". Nothing was changed.")
	}
	return nil
}
