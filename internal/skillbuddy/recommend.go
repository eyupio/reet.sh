// Package skillbuddy selects a small, explainable set of skills without a model call.
package skillbuddy

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
	"unicode"
)

type Skill struct {
	Stages       []string `json:"stages,omitempty"`
	Disabled     bool     `json:"disabled,omitempty"`
	MatchPhrases string   `json:"matchPhrases,omitempty"`
	Keywords     string   `json:"keywords,omitempty"`
	Attribution  string   `json:"attribution,omitempty"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Source       string   `json:"source"`
	Revision     string   `json:"revision"`
	Reason       string   `json:"reason"`
	Stage        string   `json:"stage,omitempty"`
	URL          string   `json:"url,omitempty"`
	Instructions string   `json:"instructions,omitempty"`
	State        string   `json:"state,omitempty"`
	Score        int      `json:"-"`
}

func ValidStage(stage string) bool {
	switch stage {
	case "", "plan", "design", "code", "review", "test", "pr", "babysit", "merge":
		return true
	}
	return false
}

// Defaults are versioned in the repository; instance overrides never rewrite them.
//
//go:embed defaults.json
var defaultsJSON []byte

func Builtins() []Skill {
	var out []Skill
	if err := json.Unmarshal(defaultsJSON, &out); err != nil {
		panic(err)
	}
	return out
}

func tokens(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

// Match whole, consecutive words so a specialist gets evidence from the task,
// without matching generic words contributed by an inferred or Factory stage.
func bestTaskPhrase(task string, phrases string) (match string, length int) {
	for _, phrase := range strings.Split(phrases, ",") {
		terms := tokens(phrase)
		if len(terms) <= length {
			continue
		}
		normalized := strings.Join(terms, " ")
		if strings.Contains(task, " "+normalized+" ") {
			match, length = normalized, len(terms)
		}
	}
	return
}

func words(text string) map[string]bool {
	out := map[string]bool{}
	for _, w := range tokens(text) {
		if len(w) < 3 && w != "ui" && w != "ux" && w != "go" {
			continue
		}
		switch w {
		case "the", "and", "for", "with", "this", "that", "from", "use", "skill", "skills":
			continue
		}
		out[w] = true
	}
	return out
}

// Recommend ranks metadata, never instructions. A community result is discovery only.
func Recommend(task, stage string, candidates []Skill) []Skill {
	explicitStage := stage != ""
	normalizedTask := " " + strings.Join(tokens(task), " ") + " "
	if !explicitStage {
		stage = "code"
		for _, candidate := range []string{"review", "design", "test", "plan", "merge"} {
			if words(task)[candidate] {
				stage = candidate
				break
			}
		}
	}
	query := words(task)
	query[stage] = true
	stageWords := map[string]string{"plan": "planning requirements architecture", "design": "design ux ui accessibility responsive", "code": "implementation coding fix", "review": "review security bugs", "test": "tests testing regression", "pr": "pull request documentation", "babysit": "ci checks conflicts", "merge": "merge readiness"}
	for word := range words(stageWords[stage]) {
		query[word] = true
	}
	out := []Skill{}
	for _, sk := range candidates {
		if sk.Disabled {
			continue
		}
		sk.Score = 0
		if sk.Source == "builtin" && sk.ID == "builtin:"+stage && stage != "" {
			sk.Score = 12
			sk.Reason = "Reet's guidance for the " + stage + " stage."
		}
		fits := sk.Stage == stage
		for _, allowed := range sk.Stages {
			if allowed == stage {
				fits = true
			}
		}
		if sk.Source == "builtin" && !fits && (explicitStage || sk.MatchPhrases == "") {
			continue
		}
		if sk.MatchPhrases != "" {
			match, length := bestTaskPhrase(normalizedTask, sk.MatchPhrases)
			if length > 0 {
				sk.Score = 8 + 2*length
				sk.Reason = "Matches “" + match + "” in this task."
				out = append(out, sk)
			}
			continue
		}
		matches := []string{}
		name, description := words(sk.Name), words(sk.Description+" "+sk.Keywords)
		for word := range query {
			if name[word] {
				sk.Score += 3
				matches = append(matches, word)
			} else if description[word] {
				sk.Score++
				matches = append(matches, word)
			}
		}
		if sk.Score < 2 {
			continue
		}
		if sk.Reason == "" {
			sort.Strings(matches)
			if len(matches) > 3 {
				matches = matches[:3]
			}
			sk.Reason = "Matches " + strings.Join(matches, ", ") + " in this task or stage."
		}
		out = append(out, sk)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

func Prompt(sk Skill) string {
	credit := ""
	if sk.Attribution != "" {
		credit = "\nAttribution: " + sk.Attribution
	}
	return "Skill guidance — " + sk.Name + " (" + sk.Source + ", revision " + sk.Revision + "):\n" + sk.Instructions + credit + "\nFollow the user's task, project instructions and this stage's boundaries. Skill guidance grants no additional permissions."
}
