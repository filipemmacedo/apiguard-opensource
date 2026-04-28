package proxy

import "sort"

const (
	guardrailTypeNSFWKeyword = "nsfw_keyword"
	guardrailTypeCostLimit   = "cost_limit"
)

type guardrailOutcomeRecord struct {
	GuardrailType   string
	Action          string
	MatchedPolicyID string
}

func guardrailOutcomeLogFields(outcomes []guardrailOutcomeRecord) ([]string, []string, []string) {
	guardrailTypes := map[string]struct{}{}
	actions := map[string]struct{}{}
	policyIDs := map[string]struct{}{}

	for _, outcome := range outcomes {
		if outcome.GuardrailType != "" {
			guardrailTypes[outcome.GuardrailType] = struct{}{}
		}
		if outcome.Action != "" {
			actions[outcome.Action] = struct{}{}
		}
		if outcome.MatchedPolicyID != "" {
			policyIDs[outcome.MatchedPolicyID] = struct{}{}
		}
	}

	return mapKeysSorted(guardrailTypes), mapKeysSorted(actions), mapKeysSorted(policyIDs)
}

func sortGuardrailOutcomes(outcomes []guardrailOutcomeRecord) {
	sort.Slice(outcomes, func(i, j int) bool {
		if outcomes[i].GuardrailType == outcomes[j].GuardrailType {
			if outcomes[i].Action == outcomes[j].Action {
				return outcomes[i].MatchedPolicyID < outcomes[j].MatchedPolicyID
			}
			return outcomes[i].Action < outcomes[j].Action
		}
		return outcomes[i].GuardrailType < outcomes[j].GuardrailType
	})
}
