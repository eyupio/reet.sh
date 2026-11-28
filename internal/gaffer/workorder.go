package gaffer

import (
	"slices"
	"strconv"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A work order is the public, versioned contract an outside agent (or the
// person pasting its output) hands the Factory: plain Markdown with a flat
// key:value header, parsed strictly so a nearly-right order never starts as
// something subtly different. Everything in it is untrusted text, shown to
// the person and never obeyed beyond the fields below; this file only reads
// it into the same shapes the plan review already edits, reusing the
// session, stage, part and criteria rules outcomePieces already enforces
// rather than restating any of them.

// MaxWorkOrderBytes is the most a pasted or submitted work order may be.
const MaxWorkOrderBytes = 64 << 10

// WorkOrderMember is one tier's crew suggestion from a work order's header:
// an agent and, optionally, the model and effort to run it on.
type WorkOrderMember struct {
	Tool   string `json:"tool"`
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

// WorkOrderProblem is one reason a work order was refused: a line (1-based;
// 0 when it is about the whole text) and what is wrong, in words that say
// what to change.
type WorkOrderProblem struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

func problem(line int, text string) *WorkOrderProblem {
	return &WorkOrderProblem{Line: line, Text: text}
}

// WorkOrder is a work order's header hints and the pipelines it asks for, in
// the shape the plan review already edits.
type WorkOrder struct {
	Objective string                     `json:"objective"`
	Project   string                     `json:"project,omitempty"`
	Pathway   string                     `json:"pathway,omitempty"`
	Continues string                     `json:"continues,omitempty"`
	Crew      map[string]WorkOrderMember `json:"crew,omitempty"`
	Sessions  []OutcomeSession           `json:"sessions"`
}

// workOrderHeaderKeys are the only keys a header line may use. A key reet
// does not know is a refusal, never ignored: a work order has no field for
// anything that could switch on YOLO, folder trust, auto-merge or who takes
// decisions.
var workOrderHeaderKeys = map[string]bool{"reet": true, "objective": true, "project": true, "crew": true, "pathway": true, "continues": true}

// IsWorkOrder is the cheap test both the paste and MCP routes use to decide
// text is meant as a work order: a header, possibly the whole of one fenced
// code block, whose first key is reet: work-order/. It says nothing about
// whether that work order is otherwise sound.
func IsWorkOrder(text string) bool {
	lines, _, ok, _ := locateWorkOrderBlock(text)
	if !ok {
		return false
	}
	closeAt := closingHeaderLine(lines)
	return closeAt > 0 && workOrderVersion(lines[1:closeAt]) != ""
}

// workOrderVersion returns the version a header's first non-blank line
// names, when that line is "reet: work-order/<version>", or "" otherwise.
func workOrderVersion(headerLines []string) string {
	for _, line := range headerLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok || strings.TrimSpace(key) != "reet" {
			return ""
		}
		value = strings.TrimSpace(value)
		if !strings.HasPrefix(value, "work-order/") {
			return ""
		}
		return strings.TrimPrefix(value, "work-order/")
	}
	return ""
}

// closingHeaderLine returns the index, within lines, of the line that closes
// the header lines opens, or -1 when there is none. lines[0] is assumed to
// already be the opening "---".
func closingHeaderLine(lines []string) int {
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return i
		}
	}
	return -1
}

// locateWorkOrderBlock finds the lines that make up a work order: the whole
// paste, or, when the whole paste sits in one fenced code block, the lines
// inside that fence. offset is the 1-based line, in text, of the returned
// lines' first entry. Blank lines around the block are trimmed, keeping the
// line numbers of what is left.
//
// A fence only wraps the whole work order when its own content starts with
// the header's opening "---": a pipeline's brief is free Markdown and may
// have a fenced code sample of its own, and that inner fence must never be
// mistaken for the one wrapping the whole paste. So the wrapping fence's
// start is the first "```" line whose own content begins with "---". Its
// end is found by stepping through what follows one fence deep at a time:
// a "```" line that carries a language tag (the usual way to open a code
// sample, "```go" and so on) opens a nested fence to step over, matched by
// the next bare "```"; a bare "```" met while not inside one of those is a
// candidate for the wrapping fence's own close.
//
// Telling that candidate apart from a plain, untagged fence nested in the
// body (also a bare "```", with nothing to mark it as not the close) is not
// possible from local syntax alone: a nested sample must carry a language
// tag to be recognised as one. So, rather than silently truncating the work
// order at the first bare line that is not really the close (missing
// whatever came after it with no error at all), every bare "```" met while
// not inside a tagged fence is counted across the whole text; exactly one
// is the ordinary, unambiguous case, and that one is the close. More than
// one means a plain fence sits somewhere in the body, and ambiguity is
// reported rather than guessed at.
func locateWorkOrderBlock(text string) (lines []string, offset int, ok bool, ambiguous *WorkOrderProblem) {
	all := strings.Split(strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(text), "\n")
	fenceStart, fenceEnd := -1, -1
	for i, line := range all {
		if !strings.HasPrefix(strings.TrimSpace(line), "```") {
			continue
		}
		if j := nextNonBlankLine(all, i+1); j >= 0 && strings.TrimSpace(all[j]) == "---" {
			fenceStart = i
			break
		}
	}
	if fenceStart >= 0 {
		nested, closes, first := false, 0, -1
		for i := fenceStart + 1; i < len(all); i++ {
			trimmed := strings.TrimSpace(all[i])
			if !strings.HasPrefix(trimmed, "```") {
				continue
			}
			switch {
			case nested:
				nested = trimmed != "```"
			case trimmed == "```":
				closes++
				if first < 0 {
					first = i
				}
			default:
				nested = true
			}
		}
		switch {
		case closes == 1:
			fenceEnd = first
		case closes > 1:
			return nil, 0, false, problem(first+1, "A work order wrapped in one fence must not contain a plain \"```\" fence inside it. Give the inner fence a language tag (for example \"```text\"), or send the work order unwrapped.")
		}
	}
	if fenceStart >= 0 && fenceEnd > fenceStart {
		lines, offset = all[fenceStart+1:fenceEnd], fenceStart+2
	} else {
		lines, offset = all, 1
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines, offset = lines[1:], offset+1
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, 0, false, nil
	}
	return lines, offset, true, nil
}

// ParseWorkOrder reads text as a work order. It never returns a usable order
// together with problems: any problem means start nothing. Session, stage,
// part and criteria rules the plan review already enforces are checked once,
// by calling outcomePieces, after the parser's own line-numbered checks pass.
func ParseWorkOrder(text string) (WorkOrder, []WorkOrderProblem) {
	if n := len(text); n > MaxWorkOrderBytes {
		return WorkOrder{}, []WorkOrderProblem{*problem(0, "A work order is at most "+strconv.Itoa(MaxWorkOrderBytes)+" bytes; this one is "+strconv.Itoa(n)+".")}
	}
	lines, offset, ok, ambiguous := locateWorkOrderBlock(text)
	if ambiguous != nil {
		return WorkOrder{}, []WorkOrderProblem{*ambiguous}
	}
	if !ok {
		return WorkOrder{}, []WorkOrderProblem{*problem(0, "This is not a work order: it must start with a --- header whose first line is reet: work-order/1, as the whole paste or inside one fenced code block.")}
	}
	closeAt := closingHeaderLine(lines)
	if closeAt < 0 {
		return WorkOrder{}, []WorkOrderProblem{*problem(offset, "The header's opening --- is never closed with one of its own.")}
	}
	order, bodyPathway, perr := parseWorkOrderHeader(lines[1:closeAt], offset+1, offset)
	if perr != nil {
		return WorkOrder{}, []WorkOrderProblem{*perr}
	}
	bodyLines, bodyOffset := lines[closeAt+1:], offset+closeAt+1
	sessions, perr := parseWorkOrderBody(bodyLines, bodyOffset, bodyPathway)
	if perr != nil {
		return WorkOrder{}, []WorkOrderProblem{*perr}
	}
	order.Sessions = sessions
	if _, err := outcomePieces(order.Sessions, "opencode", "", "", store.GafferCrew{}, false, map[string]bool{}, 0); err != nil {
		return WorkOrder{}, []WorkOrderProblem{*problem(0, err.Error())}
	}
	return order, nil
}

// parseWorkOrderHeader reads the flat key: value lines between the header's
// two "---" lines. headerOffset is the 1-based line of headerLines[0];
// openLine is the header's opening "---", used for a problem that names no
// line of its own (an objective that is missing entirely).
func parseWorkOrderHeader(headerLines []string, headerOffset, openLine int) (order WorkOrder, pathwaySet bool, _ *WorkOrderProblem) {
	seen := map[string]bool{}
	first := true
	var objectiveLine, crewLine int
	for i, raw := range headerLines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		line := headerOffset + i
		key, value, ok := strings.Cut(trimmed, ":")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok {
			return order, false, problem(line, "A header line is \"key: value\". Found: "+trimmed)
		}
		if first {
			first = false
			if key != "reet" {
				return order, false, problem(line, "A work order's header starts with reet: work-order/1.")
			}
			if value != "work-order/1" {
				return order, false, problem(line, "This controller reads work-order/1. Found "+value+".")
			}
			seen[key] = true
			continue
		}
		if !workOrderHeaderKeys[key] {
			return order, false, problem(line, "Unknown header key \""+key+"\". Keep to reet, objective, project, crew, pathway and continues.")
		}
		if seen[key] {
			return order, false, problem(line, "The header's \""+key+"\" key is given twice.")
		}
		seen[key] = true
		switch key {
		case "objective":
			order.Objective, objectiveLine = value, line
		case "project":
			order.Project = value
		case "pathway":
			order.Pathway = value
		case "continues":
			order.Continues = value
		case "crew":
			crew, perr := parseWorkOrderCrew(value, line)
			if perr != nil {
				return order, false, perr
			}
			order.Crew, crewLine = crew, line
		}
	}
	if order.Continues == "" {
		if order.Objective == "" {
			return order, false, problem(openLine, "A work order needs an objective, unless it continues an existing outcome.")
		}
		if len(order.Objective) > MaxOutcomeObjectiveBytes {
			return order, false, problem(objectiveLine, "Write the objective in up to "+strconv.Itoa(MaxOutcomeObjectiveBytes)+" UTF-8 bytes.")
		}
	}
	if order.Pathway != "" && len(order.Crew) > 0 {
		return order, false, problem(crewLine, "pathway and crew cannot both be given: a pathway already decides who does the work.")
	}
	if order.Continues != "" && len(order.Crew) > 0 {
		return order, false, problem(crewLine, "continues and crew cannot both be given: the outcome's crew was chosen when it started.")
	}
	return order, order.Pathway != "", nil
}

// parseWorkOrderCrew reads "<tier>=<agent>[/<model>[/<effort>]], ..." from a
// crew header line, each tier at most once.
func parseWorkOrderCrew(value string, line int) (map[string]WorkOrderMember, *WorkOrderProblem) {
	crew := map[string]WorkOrderMember{}
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		tier, rest, ok := strings.Cut(entry, "=")
		tier = strings.TrimSpace(tier)
		if !ok || !validStageTier(tier) {
			return nil, problem(line, "crew gives a tier of light, standard or heavy. Found \""+tier+"\".")
		}
		if _, dup := crew[tier]; dup {
			return nil, problem(line, "crew names the \""+tier+"\" tier twice.")
		}
		agent, rest, _ := strings.Cut(rest, "/")
		model, effort, _ := strings.Cut(rest, "/")
		agent = strings.TrimSpace(agent)
		if !terminalproto.ReviewedChatTool(agent) {
			return nil, problem(line, "crew names an agent reet does not run: \""+agent+"\".")
		}
		crew[tier] = WorkOrderMember{Tool: agent, Model: strings.TrimSpace(model), Effort: strings.TrimSpace(effort)}
	}
	return crew, nil
}

// parseWorkOrderBody walks the "## Pipeline:" sections after the header.
func parseWorkOrderBody(lines []string, offset int, pathwaySet bool) ([]OutcomeSession, *WorkOrderProblem) {
	var sessions []OutcomeSession
	titles := map[string]bool{}
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		line := offset + i
		switch {
		case trimmed == "":
			i++
		case trimmed == "---":
			if j := nextNonBlankLine(lines, i+1); j >= 0 {
				if key, value, ok := strings.Cut(strings.TrimSpace(lines[j]), ":"); ok && strings.TrimSpace(key) == "reet" && strings.HasPrefix(strings.TrimSpace(value), "work-order/") {
					return nil, problem(line, "This paste holds more than one work order. Send one at a time.")
				}
			}
			return nil, problem(line, "A --- line is only used to open and close the header.")
		case strings.HasPrefix(trimmed, "## "):
			title, ok := strings.CutPrefix(strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")), "Pipeline:")
			if !ok {
				return nil, problem(line, "Unknown section \""+trimmed+"\". A top-level heading is \"## Pipeline: <title>\".")
			}
			title = strings.TrimSpace(title)
			if !terminalproto.ValidTitle(title) {
				return nil, problem(line, "Give the pipeline a title of 1 to 80 plain characters.")
			}
			if titles[title] {
				return nil, problem(line, "Two pipelines are both called \""+title+"\". Give each its own title.")
			}
			titles[title] = true
			session, consumed, perr := parseWorkOrderPipeline(lines[i:], line, title, pathwaySet)
			if perr != nil {
				return nil, perr
			}
			sessions = append(sessions, session)
			if len(sessions) > MaxOutcomeSessions {
				return nil, problem(line, "A work order has at most "+strconv.Itoa(MaxOutcomeSessions)+" pipelines.")
			}
			i += consumed
		default:
			return nil, problem(line, "Found text outside any \"## Pipeline:\" section: "+trimmed)
		}
	}
	if len(sessions) == 0 {
		return nil, problem(offset, "A work order needs at least one \"## Pipeline:\" section.")
	}
	return sessions, nil
}

func nextNonBlankLine(lines []string, start int) int {
	for i := start; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" {
			return i
		}
	}
	return -1
}

// parseWorkOrderPipeline reads one "## Pipeline:" section: lines[0] is its
// heading, at line headingLine. It returns how many of lines belong to this
// pipeline, so the caller can carry on with whatever follows.
func parseWorkOrderPipeline(lines []string, headingLine int, title string, pathwaySet bool) (OutcomeSession, int, *WorkOrderProblem) {
	offset := headingLine
	i := 1
	var kinds []string
	tierOf := map[string]string{}
	var stagesLine, tiersLine int
	hadStages, hadTiers := false, false
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			i++
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		key = strings.TrimSpace(key)
		if ok && key == "stages" && !hadStages {
			hadStages, stagesLine = true, offset+i
			list, perr := parseWorkOrderStageList(strings.TrimSpace(value), stagesLine)
			if perr != nil {
				return OutcomeSession{}, 0, perr
			}
			kinds = list
			i++
			continue
		}
		if ok && key == "tiers" && !hadTiers {
			hadTiers, tiersLine = true, offset+i
			raw, perr := parseWorkOrderTierList(strings.TrimSpace(value), tiersLine)
			if perr != nil {
				return OutcomeSession{}, 0, perr
			}
			tierOf = raw
			i++
			continue
		}
		break
	}
	for kind := range tierOf {
		if !slices.Contains(kinds, kind) {
			return OutcomeSession{}, 0, problem(tiersLine, "tiers names a stage not in this pipeline's stages: \""+kind+"\".")
		}
	}
	if pathwaySet && (hadStages || hadTiers) {
		at := stagesLine
		if !hadStages {
			at = tiersLine
		}
		return OutcomeSession{}, 0, problem(at, "A pipeline takes a pathway or its own stages, not both.")
	}
	brief, i := collectWorkOrderBrief(lines, i)

	parts := map[string][]OutcomeStageInput{}
	var partsOrder []string
	partsFirstLine := map[string]int{}
	var criteria []string
scan:
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		line := offset + i
		switch {
		case trimmed == "":
			i++
		case trimmed == "---", strings.HasPrefix(trimmed, "## "):
			break scan
		case strings.HasPrefix(trimmed, "### Part:"):
			kind, partTitle, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(trimmed, "### Part:")), "/")
			kind, partTitle = strings.TrimSpace(kind), strings.TrimSpace(partTitle)
			if !ok {
				return OutcomeSession{}, 0, problem(line, "A part heading is \"### Part: <stage> / <title>\".")
			}
			if !stageTakesParts(kind) {
				return OutcomeSession{}, 0, problem(line, "The \""+kind+"\" stage cannot be split into parts.")
			}
			if !slices.Contains(kinds, kind) {
				return OutcomeSession{}, 0, problem(line, "A part names a stage not in this pipeline's stages: \""+kind+"\".")
			}
			if !terminalproto.ValidTitle(partTitle) {
				return OutcomeSession{}, 0, problem(line, "Give the part a title of 1 to 80 plain characters.")
			}
			for _, have := range parts[kind] {
				if have.Part == partTitle {
					return OutcomeSession{}, 0, problem(line, "Two parts of the \""+kind+"\" stage are both called \""+partTitle+"\".")
				}
			}
			if len(parts[kind]) >= MaxStageParts {
				return OutcomeSession{}, 0, problem(line, "The \""+kind+"\" stage has more than "+strconv.Itoa(MaxStageParts)+" parts.")
			}
			if len(parts[kind]) == 0 {
				partsOrder = append(partsOrder, kind)
				partsFirstLine[kind] = line
			}
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
				i++
			}
			partTier := ""
			if i < len(lines) {
				if tk, tv, ok2 := strings.Cut(strings.TrimSpace(lines[i]), ":"); ok2 && strings.TrimSpace(tk) == "tier" {
					tv = strings.TrimSpace(tv)
					if !validStageTier(tv) {
						return OutcomeSession{}, 0, problem(offset+i, "Choose light, standard or heavy for the \""+partTitle+"\" part.")
					}
					partTier = tv
					i++
				}
			}
			var partBrief string
			partBrief, i = collectWorkOrderBrief(lines, i)
			parts[kind] = append(parts[kind], OutcomeStageInput{Kind: kind, Part: partTitle, Brief: pipelineText.Replace(partBrief), Tier: partTier})
		case trimmed == "### Criteria":
			i++
			for i < len(lines) {
				ctrim := strings.TrimSpace(lines[i])
				if ctrim == "" {
					i++
					continue
				}
				if strings.HasPrefix(ctrim, "## ") || strings.HasPrefix(ctrim, "### ") || ctrim == "---" {
					break
				}
				text, ok := strings.CutPrefix(ctrim, "- ")
				if !ok {
					return OutcomeSession{}, 0, problem(offset+i, "A criterion is a Markdown list item: \"- text\".")
				}
				if len(criteria) >= store.MaxGafferCriteria {
					return OutcomeSession{}, 0, problem(offset+i, "A pipeline has at most "+strconv.Itoa(store.MaxGafferCriteria)+" acceptance criteria.")
				}
				criteria = append(criteria, strings.TrimSpace(text))
				i++
			}
		default:
			return OutcomeSession{}, 0, problem(line, "Unknown section \""+trimmed+"\". Use \"### Part: <stage> / <title>\" or \"### Criteria\".")
		}
	}
	for _, kind := range partsOrder {
		if len(parts[kind]) < 2 {
			return OutcomeSession{}, 0, problem(partsFirstLine[kind], "A split needs at least two \"### Part:\" sections for the \""+kind+"\" stage.")
		}
	}
	var stages []OutcomeStageInput
	for _, kind := range kinds {
		if list := parts[kind]; len(list) > 0 {
			for idx := range list {
				if list[idx].Tier == "" {
					list[idx].Tier = tierOf[kind]
				}
			}
			stages = append(stages, list...)
		} else {
			stages = append(stages, OutcomeStageInput{Kind: kind, Tier: tierOf[kind]})
		}
	}
	session := OutcomeSession{Title: title, Prompt: pipelineText.Replace(brief), Stages: stages, Criteria: criteria}
	return session, i, nil
}

// collectWorkOrderBrief gathers prose from lines[i] up to (not including) the
// next "## ", "### " or "---" boundary line, or the end of lines, trimming
// blank lines at either end of what it collected.
func collectWorkOrderBrief(lines []string, i int) (string, int) {
	start := i
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") || trimmed == "---" {
			break
		}
		i++
	}
	raw := lines[start:i]
	for len(raw) > 0 && strings.TrimSpace(raw[0]) == "" {
		raw = raw[1:]
	}
	for len(raw) > 0 && strings.TrimSpace(raw[len(raw)-1]) == "" {
		raw = raw[:len(raw)-1]
	}
	return strings.TrimSpace(strings.Join(raw, "\n")), i
}

// parseWorkOrderStageList reads "plan, code, review" from a stages: line, in
// pipeline order, each stage reet has and at most once.
func parseWorkOrderStageList(value string, line int) ([]string, *WorkOrderProblem) {
	var kinds []string
	last := -1
	for _, part := range strings.Split(value, ",") {
		kind := strings.TrimSpace(part)
		if kind == "" {
			continue
		}
		at := stageIndex(kind)
		if at < 0 {
			return nil, problem(line, "stages names a stage reet does not have: \""+kind+"\".")
		}
		if at <= last {
			return nil, problem(line, "Keep stages in pipeline order: "+strings.Join(StageKinds, ", ")+".")
		}
		last = at
		kinds = append(kinds, kind)
	}
	if len(kinds) == 0 {
		return nil, problem(line, "stages needs at least one stage.")
	}
	return kinds, nil
}

// parseWorkOrderTierList reads "code=heavy, review=standard" from a tiers:
// line. Whether each stage it names is one of the pipeline's own is checked
// by the caller, once both lines are read.
func parseWorkOrderTierList(value string, line int) (map[string]string, *WorkOrderProblem) {
	out := map[string]string{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kind, tier, ok := strings.Cut(part, "=")
		kind, tier = strings.TrimSpace(kind), strings.TrimSpace(tier)
		if !ok || !validStageTier(tier) {
			return nil, problem(line, "tiers gives light, standard or heavy for each stage. Found \""+tier+"\" for \""+kind+"\".")
		}
		if _, dup := out[kind]; dup {
			return nil, problem(line, "tiers names the \""+kind+"\" stage twice.")
		}
		out[kind] = tier
	}
	return out, nil
}
