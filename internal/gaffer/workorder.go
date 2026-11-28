package gaffer

import (
	"errors"
	"strconv"
	"strings"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// MaxWorkOrderBytes is the most text a work order may be.
const MaxWorkOrderBytes = 64 << 10

// DefaultWorkOrderStages are the stages a pipeline runs when it names none,
// here and in reet_start_outcome.
var DefaultWorkOrderStages = []string{"code", "review", "test", "pr", "babysit", "merge"}

const workOrderPrefix = "reet: work-order/"

type WorkOrderMember struct {
	Tool   string `json:"tool"`
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

type WorkOrderProblem struct {
	Line int    `json:"line"` // 1-based; 0 when it is about the whole text
	Text string `json:"text"`
}

type WorkOrder struct {
	Objective string                     `json:"objective"`
	Project   string                     `json:"project,omitempty"`
	Pathway   string                     `json:"pathway,omitempty"`
	Continues string                     `json:"continues,omitempty"`
	Crew      map[string]WorkOrderMember `json:"crew,omitempty"` // key: light|standard|heavy
	Sessions  []OutcomeSession           `json:"sessions"`
}

// WorkOrderLimits are the figures a work order is held to, read from the
// constants and validators that enforce them.
func WorkOrderLimits() map[string]int {
	return map[string]int{
		"pipelines":        MaxOutcomeSessions,
		"objective_bytes":  MaxOutcomeObjectiveBytes,
		"brief_bytes":      MaxOutcomePromptBytes,
		"parts":            MaxStageParts,
		"part_brief_bytes": maxPartBriefBytes,
		"criteria":         store.MaxGafferCriteria,
		"criterion_bytes":  store.MaxGafferCriterionText,
		"title_runes":      terminalproto.MaxTitleRunes,
	}
}

// workOrderBlock finds the work order in a paste: the line of its opening
// ---, the lines from there to the end of the order, and the first line of
// stray text before it (-1 for none; a fence around the order is not stray).
func workOrderBlock(text string) (block []string, offset, stray int, found bool) {
	text = strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(text, "\ufeff"), "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(text, "\n")
	for i := 0; i+1 < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "---" || !strings.HasPrefix(strings.TrimSpace(lines[i+1]), workOrderPrefix) {
			continue
		}
		if i > 0 && strings.HasPrefix(strings.TrimSpace(lines[i-1]), "```") {
			// The wrapper closes at its first closing line while no fence inside the order is open.
			wrap, wrapLen := fenceRun(strings.TrimSpace(lines[i-1]))
			var inner byte
			innerLen := 0
			for j := i + 1; j < len(lines); j++ {
				t := strings.TrimSpace(lines[j])
				c, run := fenceRun(t)
				switch {
				case c == 0:
				case inner == 0 && c == wrap && run >= wrapLen && run == len(t):
					for _, rest := range lines[j+1:] {
						if workOrderSection(rest) {
							return lines[i:j], i, j, true // the order goes on after its fence closed
						}
					}
					return lines[i:j], i, -1, true
				case inner == 0:
					inner, innerLen = c, run
				case c == inner && run >= innerLen && run == len(t):
					inner = 0
				}
			}
			return lines[i:], i, -1, true
		}
		for k := 0; k < i; k++ {
			if strings.TrimSpace(lines[k]) != "" {
				return lines[i:], i, k, true
			}
		}
		return lines[i:], i, -1, true
	}
	return nil, 0, -1, false
}

// IsWorkOrder is the cheap test both routes use to decide the text is meant as one.
func IsWorkOrder(text string) bool {
	_, _, _, found := workOrderBlock(text)
	return found
}

type woLine struct {
	n int
	s string
}

type woPart struct {
	line, stage int
	kind, title string
	body        []woLine
}

type woPipe struct {
	line     int
	title    string
	lead     []woLine
	parts    []*woPart
	crit     []woLine
	critLine int
}

// ParseWorkOrder never returns a usable order together with problems: any problem means start nothing.
func ParseWorkOrder(text string) (WorkOrder, []WorkOrderProblem) {
	var problems []WorkOrderProblem
	if len(text) > MaxWorkOrderBytes {
		return WorkOrder{}, []WorkOrderProblem{{0, "The work order is over " + strconv.Itoa(MaxWorkOrderBytes>>10) + " KB. Shorten the briefs."}}
	}
	block, offset, stray, found := workOrderBlock(text)
	if !found {
		return WorkOrder{}, []WorkOrderProblem{{0, "This is not a work order. It starts with a --- line, then reet: work-order/1."}}
	}
	bad := func(i int, msg string) { problems = append(problems, WorkOrderProblem{offset + i + 1, msg}) }
	whole := func(msg string) { problems = append(problems, WorkOrderProblem{0, msg}) }
	if stray > offset {
		problems = append(problems, WorkOrderProblem{stray + 1, "The code fence around the work order closes here, but the work order goes on after it. Wrap the work order in a longer fence (````) than any fence inside it."})
	} else if stray >= 0 {
		problems = append(problems, WorkOrderProblem{stray + 1, "Send the work order on its own, or inside one code fence. Remove the text before it."})
	}

	// The header: flat key: value lines up to the next ---.
	var order WorkOrder
	seen, at := map[string]bool{}, map[string]int{}
	end := -1
	for i := 1; i < len(block) && end < 0; i++ {
		line := strings.TrimSpace(block[i])
		switch {
		case line == "---":
			end = i
		case line == "":
		default:
			key, value, ok := strings.Cut(line, ":")
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if !ok {
				bad(i, "Write each header line as key: value.")
				continue
			}
			if seen[key] {
				bad(i, "The header gives “"+key+"” twice. Keep one.")
				continue
			}
			seen[key], at[key] = true, i
			if value == "" && (key == "project" || key == "pathway" || key == "continues") {
				bad(i, "Give “"+key+"” a value, or remove the line.")
				continue
			}
			if (key == "project" || key == "pathway") && !terminalproto.ValidTitle(value) {
				bad(i, "“"+key+"” is a name of 1 to "+strconv.Itoa(terminalproto.MaxTitleRunes)+" plain characters. Shorten it.")
				continue
			}
			switch key {
			case "reet":
				if value != "work-order/1" {
					bad(i, "This controller reads work-order/1 only. Write reet: work-order/1.")
				}
			case "objective":
				order.Objective = value
			case "project":
				order.Project = value
			case "pathway":
				order.Pathway = value
			case "continues":
				order.Continues = value
			case "crew":
			default:
				bad(i, "The header has no “"+key+"” key. Use reet, objective, project, crew, pathway or continues.")
			}
		}
	}
	if end < 0 {
		bad(0, "The header needs a closing --- line.")
		return WorkOrder{}, problems
	}
	if order.Continues != "" {
		if !store.ValidGafferOutcomeID(order.Continues) {
			bad(at["continues"], "continues takes an outcome's id, as written in its work report.")
		}
		order.Objective = "" // ignored when pipelines are added to an outcome
		if i, ok := at["crew"]; ok {
			bad(i, "Remove crew: an outcome's crew was chosen when it started.")
		}
	} else if !seen["objective"] {
		bad(1, "Add an objective: line to the header.")
	} else if o := order.Objective; o == "" || len(o) > MaxOutcomeObjectiveBytes || !store.ValidWorkerText(o) {
		bad(at["objective"], "Write the objective as plain text of up to "+strconv.Itoa(MaxOutcomeObjectiveBytes)+" bytes.")
	}
	if i, ok := at["crew"]; ok {
		if order.Pathway != "" {
			bad(i, "Remove crew: a pathway names its own agents.")
		} else if order.Continues == "" {
			_, raw, _ := strings.Cut(block[i], ":")
			order.Crew = workOrderCrew(strings.TrimSpace(raw), func(msg string) { bad(i, msg) })
		}
	}

	// The body: sections, with fenced code left alone.
	var pipes []*woPipe
	var cur *[]woLine
	started := false
	var fenceChar byte // the character and length of the open code fence; 0 when none
	fenceLen, fenceAt := 0, 0
	for i := end + 1; i < len(block); i++ {
		line := strings.TrimRight(block[i], " \t")
		trimmed := strings.TrimSpace(line)
		inFence := fenceChar != 0
		if c, run := fenceRun(trimmed); c != 0 {
			switch {
			case !inFence:
				fenceChar, fenceLen, fenceAt = c, run, i
			case c == fenceChar && run >= fenceLen && run == len(trimmed):
				fenceChar = 0
			}
		}
		var last *woPipe
		if len(pipes) > 0 {
			last = pipes[len(pipes)-1]
		}
		if !inFence && trimmed == "---" {
			if i+1 < len(block) && strings.HasPrefix(strings.TrimSpace(block[i+1]), workOrderPrefix) {
				bad(i, "Send one work order per paste. Remove everything from this line on.")
				return WorkOrder{}, problems
			}
			bad(i, "A brief cannot hold a --- line. Remove it.")
			continue
		}
		if heading := headingLevel(line); !inFence && heading > 0 && heading <= 3 {
			title := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			switch {
			case heading == 2 && strings.HasPrefix(title, "Pipeline:"):
				last = &woPipe{line: i, title: strings.TrimSpace(strings.TrimPrefix(title, "Pipeline:"))}
				pipes = append(pipes, last)
				cur = &last.lead
			case heading == 3 && last != nil && strings.HasPrefix(title, "Part:"):
				kind, name, _ := strings.Cut(strings.TrimPrefix(title, "Part:"), "/")
				part := &woPart{line: i, kind: strings.TrimSpace(kind), title: strings.TrimSpace(name)}
				part.stage = stageIndex(part.kind)
				last.parts = append(last.parts, part)
				cur = &part.body
			case heading == 3 && last != nil && title == "Criteria":
				if last.critLine != 0 {
					bad(i, "A pipeline has one ### Criteria list. Merge them.")
				}
				last.critLine = i + 1
				cur = &last.crit
			default:
				bad(i, "“"+trimmed+"” is not a section of a work order. Use ## Pipeline: <title>, ### Part: <stage> / <title> or ### Criteria; inside a brief use headings of level 4 or deeper.")
				cur = nil
			}
			started = true
			continue
		}
		if cur == nil {
			if trimmed != "" && !started {
				bad(i, "Start each pipeline with ## Pipeline: <title>.")
				started = true
			}
			continue
		}
		*cur = append(*cur, woLine{i, line})
	}
	if fenceChar != 0 {
		bad(fenceAt, "This code block is never closed. Close it with a line of the same fence characters, so the headings after it are read as headings.")
	}
	if len(pipes) == 0 && len(problems) == 0 {
		whole("Add at least one ## Pipeline: <title> section.")
	}

	titles := map[string]bool{}
	for k, p := range pipes {
		if k == MaxOutcomeSessions {
			bad(p.line, "A work order has at most "+strconv.Itoa(MaxOutcomeSessions)+" pipelines. Remove this one, or send it in another work order.")
			break
		}
		if !terminalproto.ValidTitle(p.title) {
			bad(p.line, "Give the pipeline a title of 1 to "+strconv.Itoa(terminalproto.MaxTitleRunes)+" plain characters.")
		} else if titles[p.title] {
			bad(p.line, "Give each pipeline its own title; “"+p.title+"” is used twice.")
		}
		titles[p.title] = true
		order.Sessions = append(order.Sessions, workOrderSession(p, order.Pathway != "", bad))
	}
	if len(problems) > 0 {
		return WorkOrder{}, problems
	}

	// Everything the session rules cover is judged by the code that enforces
	// them. The agent here only stands in: a work order does not choose it.
	check := order.Sessions
	if order.Pathway != "" {
		// A pathway brings the stages that criteria need; judge those alone.
		check = make([]OutcomeSession, len(order.Sessions))
		for i, s := range order.Sessions {
			if len(s.Criteria) > 0 {
				if _, err := planCriteria(s.Criteria); err != nil {
					whole(workOrderWhy(err))
				}
			}
			s.Criteria = nil
			check[i] = s
		}
	}
	if _, err := outcomePieces(check, "opencode", "", "", store.GafferCrew{}, false, map[string]bool{}, 0); err != nil {
		whole(workOrderWhy(err))
	}
	if len(problems) > 0 {
		return WorkOrder{}, problems
	}
	return order, nil
}

func workOrderWhy(err error) string {
	var invalid *Invalid
	if errors.As(err, &invalid) {
		return invalid.Message
	}
	return err.Error()
}

// headingLevel is the number of # opening a Markdown heading line, else 0.
// Up to three leading spaces still make a heading.
func headingLevel(line string) int {
	for k := 0; k < 3 && strings.HasPrefix(line, " "); k++ {
		line = line[1:]
	}
	n := len(line) - len(strings.TrimLeft(line, "#"))
	if n == 0 || n > 6 || len(line) == n || line[n] != ' ' {
		return 0
	}
	return n
}

// workOrderSection reports a line that opens one of a work order's sections.
func workOrderSection(line string) bool {
	title := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
	switch headingLevel(line) {
	case 2:
		return strings.HasPrefix(title, "Pipeline:")
	case 3:
		return strings.HasPrefix(title, "Part:") || title == "Criteria"
	}
	return false
}

// fenceRun is the character and length of the code fence a trimmed line
// starts with, or 0 when it starts none.
func fenceRun(t string) (byte, int) {
	if t == "" || t[0] != '`' && t[0] != '~' {
		return 0, 0
	}
	if run := len(t) - len(strings.TrimLeft(t, t[:1])); run >= 3 {
		return t[0], run
	}
	return 0, 0
}

// woSettings reads the setting lines directly under a heading. forms gives,
// for each of stage, stages, tier and tiers, the setting to write here; a
// line written as one of them (before : or =) in any other way is refused
// with that form. It returns the lines after the settings.
func woSettings(lines []woLine, forms map[string]string, place string, bad func(int, string)) (map[string]string, map[string]int, []woLine) {
	settings, at := map[string]string{}, map[string]int{}
	for len(lines) > 0 {
		t := strings.TrimSpace(lines[0].s)
		if t == "" {
			lines = lines[1:]
			continue
		}
		cut := strings.IndexAny(t, ":=")
		if cut < 0 {
			break
		}
		raw := strings.TrimSpace(t[:cut])
		key := strings.ToLower(raw)
		form, ok := forms[key]
		if !ok {
			break
		}
		switch {
		case raw != key || t[cut] != ':' || !strings.HasPrefix(form, key+":"):
			bad(lines[0].n, "Write this setting as "+form+", in lower case with a colon.")
		case at[key] != 0: // a body line is never line 0
			bad(lines[0].n, "“"+key+":” is given twice for this "+place+". Keep one.")
		default:
			settings[key], at[key] = strings.TrimSpace(t[cut+1:]), lines[0].n
		}
		lines = lines[1:]
	}
	return settings, at, lines
}

const (
	stagesForm = "stages: <stage>, <stage>"
	tiersForm  = "tiers: <stage>=<tier>, <stage>=<tier>"
	tierForm   = "tier: light, standard or heavy"
)

var (
	pipelineSettings = map[string]string{"stage": stagesForm, "stages": stagesForm, "tier": tiersForm, "tiers": tiersForm}
	partSettings     = map[string]string{"stage": tierForm, "stages": tierForm, "tier": tierForm, "tiers": tierForm}
)

// workOrderCrew reads <tier>=<agent>[/<model>[/<effort>]] entries.
func workOrderCrew(raw string, bad func(string)) map[string]WorkOrderMember {
	crew := map[string]WorkOrderMember{}
	for _, entry := range strings.Split(raw, ",") {
		tier, who, ok := strings.Cut(strings.TrimSpace(entry), "=")
		tier = strings.TrimSpace(tier)
		if !ok || !validStageTier(tier) {
			bad("Write crew as <tier>=<agent>, with light, standard or heavy for the tier; “" + strings.TrimSpace(entry) + "” is not that.")
			continue
		}
		if _, dup := crew[tier]; dup {
			bad("crew names the " + tier + " tier twice. Keep one.")
			continue
		}
		parts := strings.Split(strings.TrimSpace(who), "/")
		member := WorkOrderMember{Tool: strings.TrimSpace(parts[0])}
		if len(parts) > 1 {
			member.Model = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			member.Effort = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 || !terminalproto.ReviewedChatTool(member.Tool) || member.Model != "" && !terminalproto.ValidModel(member.Model) || member.Effort != "" && !terminalproto.ValidModel(member.Effort) {
			bad("crew cannot use “" + strings.TrimSpace(who) + "”. Write an agent reet can run as <agent>/<model>/<effort>, with model and effort optional.")
			continue
		}
		crew[tier] = member
	}
	if len(crew) == 0 {
		return nil
	}
	return crew
}

// workOrderSession turns one pipeline section into a session.
func workOrderSession(p *woPipe, onPathway bool, bad func(int, string)) OutcomeSession {
	settings, settingAt, rest := woSettings(p.lead, pipelineSettings, "pipeline", bad)
	session := OutcomeSession{Title: p.title, Prompt: workOrderText(rest)}
	written := false
	for _, c := range p.crit {
		if strings.TrimSpace(c.s) == "" {
			continue
		}
		written = true
		text := strings.TrimSpace(c.s)
		if !strings.HasPrefix(text, "- ") && !strings.HasPrefix(text, "* ") {
			bad(c.n, "Write each criterion as a list item starting with “- ”.")
			continue
		}
		session.Criteria = append(session.Criteria, strings.TrimSpace(text[2:]))
	}
	if p.critLine != 0 && !written {
		bad(p.critLine-1, "This ### Criteria list is empty. Add “- ” items, or remove the heading.")
	}
	if onPathway {
		for _, key := range []string{"stages", "tiers"} {
			if n, ok := settingAt[key]; ok {
				bad(n, "A pipeline on a pathway takes no "+key+". Remove the line, or remove pathway: from the header.")
			}
		}
		if len(p.parts) > 0 {
			bad(p.parts[0].line, "A pipeline on a pathway is not split into parts. Remove the part, or remove pathway: from the header.")
		}
		return session
	}

	kinds := DefaultWorkOrderStages
	if raw, ok := settings["stages"]; ok {
		kinds = nil
		last := -1
		for _, name := range strings.Split(raw, ",") {
			name = strings.TrimSpace(name)
			switch at := stageIndex(name); {
			case at < 0:
				bad(settingAt["stages"], "“"+name+"” is not a stage. The stages are "+strings.Join(StageKinds, ", ")+".")
			case at <= last:
				bad(settingAt["stages"], "Put the stages in pipeline order, each once: "+strings.Join(StageKinds, ", ")+".")
			default:
				last = at
				kinds = append(kinds, name)
			}
		}
	}
	has := func(kind string) bool {
		for _, k := range kinds {
			if k == kind {
				return true
			}
		}
		return false
	}
	tiers := map[string]string{}
	if raw, ok := settings["tiers"]; ok {
		for _, entry := range strings.Split(raw, ",") {
			kind, tier, _ := strings.Cut(strings.TrimSpace(entry), "=")
			kind, tier = strings.TrimSpace(kind), strings.TrimSpace(tier)
			switch {
			case !has(kind):
				bad(settingAt["tiers"], "tiers names “"+kind+"”, which is not one of this pipeline's stages.")
			case !validStageTier(tier):
				bad(settingAt["tiers"], "The tier of "+kind+" is light, standard or heavy, not “"+tier+"”.")
			case tiers[kind] != "":
				bad(settingAt["tiers"], "tiers gives "+kind+" a tier twice. Keep one.")
			default:
				tiers[kind] = tier
			}
		}
	}
	split, firstPart := map[string][]OutcomeStageInput{}, map[string]int{}
	for _, part := range p.parts {
		switch {
		case part.title == "" || part.stage < 0:
			bad(part.line, "Write a part as ### Part: <stage> / <title>.")
			continue
		case !stageTakesParts(part.kind):
			bad(part.line, "The "+part.kind+" stage is not split into parts. Split plan, design, code, review or test.")
			continue
		case !has(part.kind):
			bad(part.line, "This pipeline has no "+part.kind+" stage to split. Add it to stages:.")
			continue
		}
		stage := OutcomeStageInput{Kind: part.kind, Part: part.title, Tier: tiers[part.kind]}
		own, ownAt, body := woSettings(part.body, partSettings, "part", bad)
		if tier, ok := own["tier"]; ok {
			stage.Tier = tier
			if !validStageTier(tier) {
				bad(ownAt["tier"], "A part's tier is light, standard or heavy, not “"+tier+"”.")
			}
		}
		stage.Brief = workOrderText(body)
		if len(split[part.kind]) == 0 {
			firstPart[part.kind] = part.line
		}
		split[part.kind] = append(split[part.kind], stage)
	}
	for _, kind := range kinds {
		if len(split[kind]) == 1 {
			bad(firstPart[kind], "Split a stage into 2 to "+strconv.Itoa(MaxStageParts)+" parts, or give “"+kind+"” no parts.")
		}
		if parts := split[kind]; len(parts) > 0 {
			session.Stages = append(session.Stages, parts...)
		} else {
			session.Stages = append(session.Stages, OutcomeStageInput{Kind: kind, Tier: tiers[kind]})
		}
	}
	return session
}

func workOrderText(lines []woLine) string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.s
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
