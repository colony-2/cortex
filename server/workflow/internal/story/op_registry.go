package story

import (
	"strings"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
)

func nextTaskFromRegistry(opID string, currentTask string) (string, bool) {
	opID = strings.TrimSpace(opID)
	if opID == "" {
		return "", false
	}
	def, ok := coreops.Get(opID)
	if !ok || def == nil {
		return "", false
	}
	stepID := strings.TrimSpace(stepIDFromTaskType(opID, currentTask))
	if stepID == "" {
		return "", false
	}
	chain := def.TaskChain()
	for i := range chain {
		st := chain[i]
		if strings.TrimSpace(st.Name) != stepID {
			continue
		}
		next := strings.TrimSpace(st.NextStepTask)
		if next == "" {
			return "", false
		}
		return next, true
	}
	return "", false
}
