package gaffer

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/eyupio/reet.sh/internal/skillbuddy"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A template is a successful plan saved to start again. It keeps what made the
// plan: its pipelines' briefs, stage layout, criteria and the skill revisions
// that were approved for its stages. It keeps nothing that ties it to a run:
// the machine, the agents and the sign-ins are chosen again at each launch.

const maxTemplateDescriptionBytes = 1000

// SkillLoader loads a skill for the account as it is approved now.
type SkillLoader func(id string) (skillbuddy.Skill, error)

type TemplateSaveInput struct {
	RequestID   string `json:"requestId"`
	OutcomeID   string `json:"outcomeId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// TemplateID and BaseVersion save the outcome as the next version of a
	// template the account has, on the version the person was shown.
	TemplateID  string `json:"templateId"`
	BaseVersion int    `json:"baseVersion"`
}

type TemplateEditInput struct {
	RequestID   string                      `json:"requestId"`
	BaseVersion int                         `json:"baseVersion"`
	Content     store.GafferTemplateContent `json:"content"`
}

type TemplateLaunchInput struct {
	RequestID    string `json:"requestId"`
	Version      int    `json:"version"`
	ProjectID    string `json:"projectId"`
	Tool         string `json:"tool"`
	Model        string `json:"model"`
	Provider     string `json:"provider"`
	Trust        bool   `json:"trust"`
	Yolo         bool   `json:"yolo"`
	PullLatest   string `json:"pullLatest"`
	AutoReport   bool   `json:"autoReport"`
	Automation   bool   `json:"automation"`
	BalanceBelow int    `json:"balanceBelow"`
	// SkippedSkills are the gaps the person was shown and agreed to start
	// without, each as TemplateSkillGap.Key.
	SkippedSkills []string `json:"skippedSkills"`
	Confirmed     bool     `json:"confirmed"`
}

// TemplateSkillGap is a skill a template names that is no longer approved for
// the account at the revision the template holds: "changed" when another
// revision is approved now, "unavailable" when the skill cannot be used.
type TemplateSkillGap struct {
	Pipeline int    `json:"pipeline"`
	Stage    int    `json:"stage"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Revision string `json:"revision"`
	Status   string `json:"status"`
	Key      string `json:"key"`
}

type TemplateView struct {
	store.GafferTemplate
	Latest          int                `json:"latest"`
	Versions        []int              `json:"versions"`
	LastUsedVersion int                `json:"lastUsedVersion"`
	SkillGaps       []TemplateSkillGap `json:"skillGaps"`
}

type TemplateSummary struct {
	ID              string `json:"id"`
	Version         int    `json:"version"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Pipelines       int    `json:"pipelines"`
	CreatedAt       int64  `json:"createdAt"`
	UpdatedAt       int64  `json:"updatedAt"`
	LastUsedVersion int    `json:"lastUsedVersion"`
}

func templateConflict(code, message string) error { return &DeliveryConflict{code, message} }

// successfulPipeline is a pipeline whose work was finished: every stage was
// handed over, or the person accepted its delivery.
func successfulPipeline(p store.GafferOutcomeSession) bool {
	return len(p.Stages) > 0 && (p.State == "done" || p.DeliveryAccepted)
}

// templateFromOutcome copies what is reusable from a successful outcome.
func templateFromOutcome(o store.GafferOutcome) (store.GafferTemplateContent, error) {
	c := store.GafferTemplateContent{Objective: o.Objective, MaxReviews: o.MaxReviews, Hours: o.Hours}
	ok := o.State != "starting" && len(o.Sessions) > 0
	for _, p := range o.Sessions {
		ok = ok && successfulPipeline(p)
	}
	if !ok {
		return c, templateConflict("template_not_successful", "Only a plan whose every pipeline finished, or had its delivery accepted, can be saved as a template.")
	}
	if len(o.Sessions) > MaxOutcomeSessions {
		return c, invalid("outcomeId", "A template starts at most "+strconv.Itoa(MaxOutcomeSessions)+" pipelines, and this outcome has "+strconv.Itoa(len(o.Sessions))+".")
	}
	for _, p := range o.Sessions {
		pipeline := store.GafferTemplatePipeline{Title: p.Title, Prompt: p.Prompt, AutoMerge: p.AutoMerge}
		for _, st := range p.Stages {
			stage := store.GafferTemplateStage{Kind: st.Kind, Part: st.Part, Brief: st.Brief, Tier: st.Tier, TierWhy: st.TierWhy}
			for _, sk := range st.SkillGuidance {
				stage.Skills = append(stage.Skills, store.GafferTemplateSkill{ID: sk.ID, Name: sk.Name, Source: sk.Source, Revision: sk.Revision})
			}
			pipeline.Stages = append(pipeline.Stages, stage)
		}
		for _, cr := range p.Criteria {
			pipeline.Criteria = append(pipeline.Criteria, store.GafferTemplateCriterion{ID: cr.ID, Text: cr.Text})
		}
		c.Pipelines = append(c.Pipelines, pipeline)
	}
	return c, nil
}

// templateSessions is the plan a template holds, as an approval would carry it.
// No stage names an agent, so each takes its tier's from the crew at launch.
func templateSessions(c store.GafferTemplateContent) []OutcomeSession {
	out := make([]OutcomeSession, 0, len(c.Pipelines))
	for _, p := range c.Pipelines {
		auto := p.AutoMerge
		item := OutcomeSession{Title: p.Title, Prompt: p.Prompt, AutoMerge: &auto}
		for _, st := range p.Stages {
			item.Stages = append(item.Stages, OutcomeStageInput{Kind: st.Kind, Part: st.Part, Brief: st.Brief, Tier: st.Tier, TierWhy: st.TierWhy})
		}
		for _, cr := range p.Criteria {
			item.Criteria, item.CriteriaIDs = append(item.Criteria, cr.Text), append(item.CriteriaIDs, cr.ID)
		}
		out = append(out, item)
	}
	return out
}

func validSkillRef(sk store.GafferTemplateSkill) bool {
	return sk.ID != "" && len(sk.ID) <= 200 && terminalproto.ValidPlainText(sk.ID, 200) && len(sk.Revision) <= 200 && terminalproto.ValidPlainText(sk.Revision, 200) &&
		terminalproto.ValidPlainText(sk.Name, 200) && terminalproto.ValidPlainText(sk.Source, 40)
}

// normalTemplate checks a template's content and gives its criteria their
// ids: one the template had keeps it, a new one gets its own.
func normalTemplate(c *store.GafferTemplateContent, known []store.GafferTemplateCriterion) error {
	c.Name, c.Description, c.Objective = strings.TrimSpace(c.Name), strings.TrimSpace(c.Description), strings.TrimSpace(c.Objective)
	if !terminalproto.ValidTitle(c.Name) {
		return invalid("name", "Give the template a name of 1 to 80 plain characters.")
	}
	if len(c.Description) > maxTemplateDescriptionBytes || c.Description != "" && !store.ValidWorkerText(c.Description) {
		return invalid("description", "Write the description in up to 1,000 bytes of plain text.")
	}
	if c.Objective == "" || len(c.Objective) > MaxOutcomeObjectiveBytes || !store.ValidWorkerText(c.Objective) {
		return invalid("objective", "Write the objective in up to 4,000 UTF-8 bytes of plain text.")
	}
	if c.MaxReviews < 1 || c.MaxReviews > 48 || c.Hours < 1 || c.Hours > 168 {
		return invalid("goal", "Choose 1 to 48 reviews over 1 to 168 hours for each session.")
	}
	if len(c.Pipelines) < 1 || len(c.Pipelines) > MaxOutcomeSessions {
		return invalid("pipelines", "A template starts between 1 and 5 pipelines.")
	}
	old := make([]store.GafferCriterion, 0, len(known))
	for _, cr := range known {
		old = append(old, store.GafferCriterion{ID: cr.ID, Text: cr.Text})
	}
	for i := range c.Pipelines {
		p := &c.Pipelines[i]
		if len(p.Stages) == 0 {
			return invalid("pipelines", "Give each pipeline of a template its stages.")
		}
		for j := range p.Stages {
			st := &p.Stages[j]
			if st.Tier == "" {
				st.Tier = DefaultStageTier(st.Kind)
			}
			if len(st.Skills) > 2 {
				return invalid("skills", "A stage carries at most two skills.")
			}
			for _, sk := range st.Skills {
				if !validSkillRef(sk) {
					return invalid("skills", "One of the template's skills could not be read.")
				}
			}
		}
		items := make([]CriterionInput, 0, len(p.Criteria))
		for _, cr := range p.Criteria {
			items = append(items, CriterionInput{ID: cr.ID, Text: cr.Text})
		}
		criteria, err := newCriteria(old, items, 1)
		if err != nil {
			return err
		}
		p.Criteria = nil
		for _, cr := range criteria {
			p.Criteria = append(p.Criteria, store.GafferTemplateCriterion{ID: cr.ID, Text: cr.Text})
		}
	}
	// What a launch will be given must be a plan an approval accepts. Any
	// reviewed agent stands in for the one chosen at launch.
	_, err := outcomePieces(templateSessions(*c), "opencode", "", "", store.GafferCrew{}, false, map[string]bool{}, 0)
	return err
}

func templateCriteria(c store.GafferTemplateContent) []store.GafferTemplateCriterion {
	var out []store.GafferTemplateCriterion
	for _, p := range c.Pipelines {
		out = append(out, p.Criteria...)
	}
	return out
}

// SaveTemplate saves one of the account's successful outcomes as a new
// template, or as the next version of one it has.
func (s *Service) SaveTemplate(uid int64, in TemplateSaveInput) (store.GafferTemplate, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.GafferTemplate{}, err
	}
	if !store.ValidGafferOutcomeID(in.RequestID) {
		return store.GafferTemplate{}, invalid("requestId", "A request ID is 32 lowercase hexadecimal characters.")
	}
	if in.TemplateID != "" && (!store.ValidGafferOutcomeID(in.TemplateID) || in.BaseVersion < 1) || in.TemplateID == "" && in.BaseVersion != 0 {
		return store.GafferTemplate{}, invalid("templateId", "Name the template and the version you were shown, or neither for a new template.")
	}
	o, err := s.store.GafferOutcomeFor(uid, in.OutcomeID)
	if err != nil {
		return store.GafferTemplate{}, err
	}
	content, err := templateFromOutcome(o)
	if err != nil {
		return store.GafferTemplate{}, err
	}
	content.Name, content.Description = in.Name, in.Description
	var known []store.GafferTemplateCriterion
	if in.TemplateID != "" {
		base, err := s.store.GafferTemplateFor(uid, in.TemplateID, 0)
		if err != nil {
			return store.GafferTemplate{}, err
		}
		known = templateCriteria(base.Content)
	}
	// The outcome's criteria ids are carried over as the template's own.
	known = append(known, templateCriteria(content)...)
	if err = normalTemplate(&content, known); err != nil {
		return store.GafferTemplate{}, err
	}
	id := in.TemplateID
	if id == "" {
		if id, err = newChatID(); err != nil {
			return store.GafferTemplate{}, err
		}
	}
	saved, err := s.store.SaveGafferTemplateFor(uid, store.GafferTemplate{ID: id, RequestID: in.RequestID, OutcomeID: o.ID, Content: content}, in.BaseVersion)
	if errors.Is(err, store.ErrGafferTemplateRepeat) {
		if saved.OutcomeID == o.ID && (in.TemplateID == "" && saved.Version == 1 || saved.ID == in.TemplateID && saved.Version == in.BaseVersion+1) {
			return saved, nil
		}
		return store.GafferTemplate{}, templateConflict("template_request_reused", "That request ID was already used for something else. Try again.")
	}
	return saved, err
}

// EditTemplate saves changed content as the next version of a template.
func (s *Service) EditTemplate(uid int64, id string, in TemplateEditInput) (store.GafferTemplate, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.GafferTemplate{}, err
	}
	if !store.ValidGafferOutcomeID(in.RequestID) {
		return store.GafferTemplate{}, invalid("requestId", "A request ID is 32 lowercase hexadecimal characters.")
	}
	if in.BaseVersion < 1 {
		return store.GafferTemplate{}, invalid("baseVersion", "Reload this template before changing it.")
	}
	base, err := s.store.GafferTemplateFor(uid, id, 0)
	if err != nil {
		return store.GafferTemplate{}, err
	}
	content := in.Content
	if err = normalTemplate(&content, templateCriteria(base.Content)); err != nil {
		return store.GafferTemplate{}, err
	}
	saved, err := s.store.SaveGafferTemplateFor(uid, store.GafferTemplate{ID: id, RequestID: in.RequestID, Content: content}, in.BaseVersion)
	if errors.Is(err, store.ErrGafferTemplateRepeat) {
		if saved.ID == id && saved.OutcomeID == "" && saved.Version == in.BaseVersion+1 {
			return saved, nil
		}
		return store.GafferTemplate{}, templateConflict("template_request_reused", "That request ID was already used for something else. Try again.")
	}
	return saved, err
}

// lastUsedVersions is the version of each template the account's newest
// outcome started from it used, from the outcomes kept.
func (s *Service) lastUsedVersions(uid int64) (map[string]int, error) {
	rows, err := s.store.GafferOutcomesFor(uid)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, o := range rows {
		if _, seen := out[o.TemplateID]; o.TemplateID != "" && !seen {
			out[o.TemplateID] = o.TemplateVersion
		}
	}
	return out, nil
}

func (s *Service) Templates(uid int64) ([]TemplateSummary, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return nil, err
	}
	rows, err := s.store.GafferTemplatesFor(uid)
	if err != nil {
		return nil, err
	}
	used, err := s.lastUsedVersions(uid)
	if err != nil {
		return nil, err
	}
	out := make([]TemplateSummary, 0, len(rows))
	for _, t := range rows {
		out = append(out, TemplateSummary{ID: t.ID, Version: t.Version, Name: t.Content.Name, Description: t.Content.Description, Pipelines: len(t.Content.Pipelines), CreatedAt: t.FirstCreatedAt, UpdatedAt: t.CreatedAt, LastUsedVersion: used[t.ID]})
	}
	return out, nil
}

// templateSkills loads each skill a template names, as approved now. A skill
// is used only at the revision the template holds; any other is a gap.
func templateSkills(c store.GafferTemplateContent, load SkillLoader) (map[string]skillbuddy.Skill, []TemplateSkillGap) {
	loaded, gaps := map[string]skillbuddy.Skill{}, []TemplateSkillGap{}
	for i, p := range c.Pipelines {
		for j, st := range p.Stages {
			for _, ref := range st.Skills {
				key := strconv.Itoa(i) + ":" + strconv.Itoa(j) + ":" + ref.ID
				sk, err := load(ref.ID)
				switch {
				case err != nil:
					gaps = append(gaps, TemplateSkillGap{Pipeline: i, Stage: j, ID: ref.ID, Name: ref.Name, Revision: ref.Revision, Status: "unavailable", Key: key})
				case sk.Revision != ref.Revision:
					gaps = append(gaps, TemplateSkillGap{Pipeline: i, Stage: j, ID: ref.ID, Name: ref.Name, Revision: ref.Revision, Status: "changed", Key: key})
				default:
					loaded[key] = sk
				}
			}
		}
	}
	return loaded, gaps
}

// Template reads one version of a template, version 0 being the latest, with
// the skills it names that are no longer approved at their revision.
func (s *Service) Template(uid int64, id string, version int, load SkillLoader) (TemplateView, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return TemplateView{}, err
	}
	t, err := s.store.GafferTemplateFor(uid, id, version)
	if err != nil {
		return TemplateView{}, err
	}
	versions, err := s.store.GafferTemplateVersionsFor(uid, id)
	if err != nil {
		return TemplateView{}, err
	}
	used, err := s.lastUsedVersions(uid)
	if err != nil {
		return TemplateView{}, err
	}
	_, gaps := templateSkills(t.Content, load)
	return TemplateView{GafferTemplate: t, Latest: versions[len(versions)-1], Versions: versions, LastUsedVersion: used[id], SkillGaps: gaps}, nil
}

// DeleteTemplate removes a template and its versions. Outcomes started from
// it keep everything they copied.
func (s *Service) DeleteTemplate(uid int64, id string, version int) error {
	if _, err := s.chatPrincipal(uid); err != nil {
		return err
	}
	return s.store.DeleteGafferTemplateFor(uid, id, version)
}

// LaunchTemplate starts an outcome from a template version, for the project
// and agents the person chooses now. The plan is the version's; each stage's
// agent comes from the crew as it is saved at this moment.
func (s *Service) LaunchTemplate(uid int64, id string, in TemplateLaunchInput, load SkillLoader) (store.GafferOutcome, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return store.GafferOutcome{}, err
	}
	if !store.ValidGafferOutcomeID(in.RequestID) {
		return store.GafferOutcome{}, invalid("requestId", "A request ID is 32 lowercase hexadecimal characters.")
	}
	if in.Version < 1 {
		return store.GafferOutcome{}, invalid("version", "Choose the template version to start.")
	}
	// Asked for before under this request: that outcome is the answer.
	if existing, err := s.store.GafferOutcomeFor(uid, in.RequestID); err == nil {
		if existing.TemplateID != id || existing.TemplateVersion != in.Version {
			return store.GafferOutcome{}, templateConflict("template_request_reused", "That request ID was already used for something else. Try again.")
		}
		return outcomeView(existing), nil
	}
	t, err := s.store.GafferTemplateFor(uid, id, in.Version)
	if err != nil {
		return store.GafferOutcome{}, err
	}
	loaded, gaps := templateSkills(t.Content, load)
	have := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		have = append(have, gap.Key)
	}
	skipped := append([]string(nil), in.SkippedSkills...)
	sort.Strings(have)
	sort.Strings(skipped)
	if strings.Join(have, "\n") != strings.Join(skipped, "\n") {
		return store.GafferOutcome{}, templateConflict("template_skills_changed", "The skills this template uses have changed since you looked. Review them before starting.")
	}
	sessions := templateSessions(t.Content)
	for i := range sessions {
		for j := range sessions[i].Stages {
			size := 0
			for _, ref := range t.Content.Pipelines[i].Stages[j].Skills {
				if sk, ok := loaded[strconv.Itoa(i)+":"+strconv.Itoa(j)+":"+ref.ID]; ok {
					sk.State = "selected"
					size += len(skillbuddy.Prompt(sk))
					sessions[i].Stages[j].Skills = append(sessions[i].Stages[j].Skills, sk)
				}
			}
			if size > 2000 {
				return store.GafferOutcome{}, invalid("skills", "A stage's skills must fit in 2,000 bytes. Edit the template's skills.")
			}
		}
	}
	return s.CreateOutcome(uid, OutcomeInput{Objective: t.Content.Objective, ProjectID: in.ProjectID, Tool: in.Tool, Model: in.Model, Provider: in.Provider, Trust: in.Trust, Yolo: in.Yolo, PullLatest: in.PullLatest,
		Sessions: sessions, MaxReviews: t.Content.MaxReviews, Hours: t.Content.Hours, Confirmed: in.Confirmed, AutoReport: in.AutoReport, Automation: in.Automation, BalanceBelow: in.BalanceBelow,
		RequestID: in.RequestID, Template: &store.GafferTemplateRef{ID: t.ID, Version: t.Version}})
}

type TemplateCriteriaDiff struct {
	Added    []string                `json:"added"`
	Removed  []string                `json:"removed"`
	Reworded []TemplateCriterionRedo `json:"reworded"`
}

type TemplateCriterionRedo struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

type TemplateSkillDiff struct {
	Stage string `json:"stage"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	From  string `json:"from"`
	To    string `json:"to"`
}

type TemplatePipelineDiff struct {
	Title            string               `json:"title"`
	Change           string               `json:"change"`
	PromptChanged    bool                 `json:"promptChanged"`
	AutoMergeChanged bool                 `json:"autoMergeChanged"`
	StagesChanged    bool                 `json:"stagesChanged"`
	StagesBefore     []string             `json:"stagesBefore"`
	StagesAfter      []string             `json:"stagesAfter"`
	Criteria         TemplateCriteriaDiff `json:"criteria"`
	Skills           []TemplateSkillDiff  `json:"skills"`
}

type TemplateDiff struct {
	From               int                    `json:"from"`
	To                 int                    `json:"to"`
	Changed            bool                   `json:"changed"`
	NameChanged        bool                   `json:"nameChanged"`
	DescriptionChanged bool                   `json:"descriptionChanged"`
	ObjectiveChanged   bool                   `json:"objectiveChanged"`
	LimitsChanged      bool                   `json:"limitsChanged"`
	Pipelines          []TemplatePipelineDiff `json:"pipelines"`
}

func stageLabel(st store.GafferTemplateStage) string {
	if st.Part != "" {
		return st.Kind + ": " + st.Part
	}
	return st.Kind
}

// diffTemplates says what changed from a to b. Pipelines are matched by
// title, stages by kind and part, criteria by id.
func diffTemplates(a, b store.GafferTemplateContent) TemplateDiff {
	d := TemplateDiff{NameChanged: a.Name != b.Name, DescriptionChanged: a.Description != b.Description, ObjectiveChanged: a.Objective != b.Objective,
		LimitsChanged: a.MaxReviews != b.MaxReviews || a.Hours != b.Hours, Pipelines: []TemplatePipelineDiff{}}
	before := map[string]store.GafferTemplatePipeline{}
	for _, p := range a.Pipelines {
		before[p.Title] = p
	}
	after := map[string]bool{}
	for _, p := range b.Pipelines {
		after[p.Title] = true
		old, ok := before[p.Title]
		if !ok {
			old = store.GafferTemplatePipeline{}
		}
		item := diffPipeline(old, p)
		if !ok {
			item.Change = "added"
		}
		d.Pipelines = append(d.Pipelines, item)
	}
	for _, p := range a.Pipelines {
		if !after[p.Title] {
			item := diffPipeline(p, store.GafferTemplatePipeline{})
			item.Title, item.Change = p.Title, "removed"
			d.Pipelines = append(d.Pipelines, item)
		}
	}
	d.Changed = d.NameChanged || d.DescriptionChanged || d.ObjectiveChanged || d.LimitsChanged
	for _, p := range d.Pipelines {
		d.Changed = d.Changed || p.Change != "same"
	}
	return d
}

func diffPipeline(a, b store.GafferTemplatePipeline) TemplatePipelineDiff {
	d := TemplatePipelineDiff{Title: b.Title, PromptChanged: a.Prompt != b.Prompt, AutoMergeChanged: a.AutoMerge != b.AutoMerge, StagesBefore: []string{}, StagesAfter: []string{},
		Criteria: TemplateCriteriaDiff{Added: []string{}, Removed: []string{}, Reworded: []TemplateCriterionRedo{}}, Skills: []TemplateSkillDiff{}}
	if len(a.Stages) != len(b.Stages) {
		d.StagesChanged = true
	}
	stagesA := map[string]store.GafferTemplateStage{}
	for i, st := range a.Stages {
		d.StagesBefore = append(d.StagesBefore, stageLabel(st))
		stagesA[stageLabel(st)] = st
		if i < len(b.Stages) {
			other := b.Stages[i]
			d.StagesChanged = d.StagesChanged || st.Kind != other.Kind || st.Part != other.Part || st.Brief != other.Brief || st.Tier != other.Tier
		}
	}
	stagesB := map[string]store.GafferTemplateStage{}
	for _, st := range b.Stages {
		d.StagesAfter = append(d.StagesAfter, stageLabel(st))
		stagesB[stageLabel(st)] = st
	}
	skillDiff := func(label string, from, to []store.GafferTemplateSkill) {
		revs := map[string]string{}
		for _, sk := range from {
			revs[sk.ID] = sk.Revision
		}
		seen := map[string]bool{}
		for _, sk := range to {
			seen[sk.ID] = true
			if revs[sk.ID] != sk.Revision {
				d.Skills = append(d.Skills, TemplateSkillDiff{Stage: label, ID: sk.ID, Name: sk.Name, From: revs[sk.ID], To: sk.Revision})
			}
		}
		for _, sk := range from {
			if !seen[sk.ID] {
				d.Skills = append(d.Skills, TemplateSkillDiff{Stage: label, ID: sk.ID, Name: sk.Name, From: sk.Revision})
			}
		}
	}
	for _, st := range b.Stages {
		skillDiff(stageLabel(st), stagesA[stageLabel(st)].Skills, st.Skills)
	}
	for _, st := range a.Stages {
		if _, ok := stagesB[stageLabel(st)]; !ok {
			skillDiff(stageLabel(st), st.Skills, nil)
		}
	}
	texts := map[string]string{}
	for _, cr := range a.Criteria {
		texts[cr.ID] = cr.Text
	}
	kept := map[string]bool{}
	for _, cr := range b.Criteria {
		old, ok := texts[cr.ID]
		kept[cr.ID] = true
		switch {
		case !ok:
			d.Criteria.Added = append(d.Criteria.Added, cr.Text)
		case old != cr.Text:
			d.Criteria.Reworded = append(d.Criteria.Reworded, TemplateCriterionRedo{ID: cr.ID, From: old, To: cr.Text})
		}
	}
	for _, cr := range a.Criteria {
		if !kept[cr.ID] {
			d.Criteria.Removed = append(d.Criteria.Removed, cr.Text)
		}
	}
	d.Change = "same"
	if d.PromptChanged || d.AutoMergeChanged || d.StagesChanged || len(d.Skills) > 0 || len(d.Criteria.Added)+len(d.Criteria.Removed)+len(d.Criteria.Reworded) > 0 {
		d.Change = "changed"
	}
	return d
}

// TemplateDiff compares two kept versions of a template. to 0 is the latest;
// from 0 is the version last used, else the one before to.
func (s *Service) TemplateDiff(uid int64, id string, from, to int) (TemplateDiff, error) {
	if _, err := s.chatPrincipal(uid); err != nil {
		return TemplateDiff{}, err
	}
	b, err := s.store.GafferTemplateFor(uid, id, to)
	if err != nil {
		return TemplateDiff{}, err
	}
	if from == 0 {
		used, err := s.lastUsedVersions(uid)
		if err != nil {
			return TemplateDiff{}, err
		}
		if from = used[id]; from == 0 {
			from = max(b.Version-1, 1)
		}
	}
	a, err := s.store.GafferTemplateFor(uid, id, from)
	if err != nil {
		return TemplateDiff{}, err
	}
	d := diffTemplates(a.Content, b.Content)
	d.From, d.To = a.Version, b.Version
	return d, nil
}
