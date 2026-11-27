package api

import (
	"errors"

	"github.com/eyupio/reet.sh/internal/skillbuddy"
)

func mcpBuddyTool(name string) bool {
	return name == "reet_recommend_skills" || name == "reet_load_skill"
}
func (s *server) executeMCPBuddy(access mcpAccess, name string, args map[string]any) (map[string]any, error) {
	if access.User.Role == "viewer" {
		return nil, errors.New("Skill recommendations need a writer account.")
	}
	str := func(key string) string { v, _ := args[key].(string); return v }
	if name == "reet_load_skill" {
		sk, err := s.buddyLoad(access.User.ID, str("id"))
		if err != nil {
			_, problem := buddyLoadProblem(err)
			return nil, errors.New(problem)
		}
		return map[string]any{"skill": sk, "guidance": skillbuddy.Prompt(sk), "limits": "Instructions only. Supporting scripts and plugin hooks are not installed. Loading does not prove use or grant permissions."}, nil
	}
	task, stage := str("task"), str("stage")
	if len(task) == 0 || len(task) > 4000 || !skillbuddy.ValidStage(stage) {
		return nil, errors.New("Give a task of up to 4,000 bytes and a valid stage.")
	}
	candidates, err := s.buddyCatalogue(access.User.ID)
	if err != nil {
		return nil, err
	}
	sk := skillbuddy.Recommend(task, stage, candidates)
	for i := range sk {
		sk[i].Instructions = ""
	}
	return map[string]any{"skills": sk, "next": "Load a relevant builtin or personal skill with reet_load_skill. Community entries are suggestions to inspect and add to your library first. Only load what this task needs."}, nil
}
