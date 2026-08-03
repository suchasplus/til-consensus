package runtime

import (
	"slices"

	"github.com/suchasplus/til-consensus/config"
	"github.com/suchasplus/til-consensus/consensus"
)

func TaskOutputJSONSchema(task consensus.Task) map[string]any {
	switch task := task.(type) {
	case consensus.ProposalTask:
		return objectSchema(
			[]string{"summary", "claims"},
			map[string]any{
				"summary": schemaString(),
				"claims":  arraySchema(proposalClaimDraftSchema()),
			},
		)
	case consensus.InitialProposalTask:
		return objectSchema(
			[]string{"summary", "claims"},
			map[string]any{
				"summary": schemaString(),
				"claims":  arraySchema(debateClaimDraftSchema()),
			},
		)
	case consensus.ChallengeTask:
		return objectSchema(
			[]string{"summary", "tickets"},
			map[string]any{
				"summary": schemaString(),
				"tickets": arraySchema(challengeDraftSchema()),
			},
		)
	case consensus.ReviseTask:
		return objectSchema(
			[]string{"summary", "revisions"},
			map[string]any{
				"summary":             schemaString(),
				"revisions":           arraySchema(revisionDraftSchema()),
				"unresolvedQuestions": stringArraySchema(),
			},
		)
	case consensus.DebateRoundTask:
		return objectSchema(
			[]string{"summary", "judgements"},
			map[string]any{
				"summary":    schemaString(),
				"newClaims":  arraySchema(debateClaimDraftSchema()),
				"judgements": arraySchema(debateJudgementSchema()),
			},
		)
	case consensus.SemanticDedupTask:
		return objectSchema(
			[]string{"summary", "merges"},
			map[string]any{
				"summary": schemaString(),
				"merges":  arraySchema(debateClaimMergeSchema()),
			},
		)
	case consensus.SynthesisTask:
		return objectSchema(
			[]string{"summary", "claim"},
			map[string]any{
				"summary": schemaString(),
				"claim": objectSchema(
					[]string{"statement"},
					map[string]any{
						"title":     schemaString(),
						"statement": schemaString(),
					},
				),
			},
		)
	case consensus.FinalVoteTask:
		return objectSchema(
			[]string{"summary", "votes"},
			map[string]any{
				"summary": schemaString(),
				"votes":   arraySchema(debateVoteSchema()),
			},
		)
	case consensus.DelphiQuestionnaireTask:
		responses := arraySchema(delphiResponseSchema(task.RatingScaleMin, task.RatingScaleMax))
		responses["minItems"] = 1
		return objectSchema(
			[]string{"summary", "responses"},
			map[string]any{
				"summary":   schemaString(),
				"responses": responses,
			},
		)
	case consensus.DelphiRevisionTask:
		responses := arraySchema(delphiResponseSchema(task.RatingScaleMin, task.RatingScaleMax))
		responses["minItems"] = 1
		return objectSchema(
			[]string{"summary", "responses"},
			map[string]any{
				"summary":   schemaString(),
				"responses": responses,
			},
		)
	case consensus.DelphiFacilitatorSummaryTask:
		return objectSchema(
			[]string{"summary"},
			map[string]any{
				"summary":        schemaString(),
				"recommendation": schemaString(),
				"dissentSummary": stringArraySchema(),
				"statements":     arraySchema(delphiStatementSchema()),
			},
		)
	case consensus.SemanticVerificationTask:
		return objectSchema(
			[]string{"summary", "results"},
			map[string]any{
				"summary": schemaString(),
				"results": arraySchema(semanticFindingSchema()),
			},
		)
	case consensus.ArbiterTask:
		return objectSchema(
			[]string{"summary", "taskVerdict", "decisions"},
			map[string]any{
				"summary":     schemaString(),
				"taskVerdict": enumStringSchema([]string{string(consensus.TaskVerdictSupported), string(consensus.TaskVerdictPartiallySupported), string(consensus.TaskVerdictUndetermined), string(consensus.TaskVerdictFailed)}),
				"decisions":   arraySchema(arbiterDecisionSchema()),
			},
		)
	case consensus.ReportTask:
		return objectSchema(
			[]string{"summary"},
			map[string]any{
				"summary":             schemaString(),
				"highlights":          stringArraySchema(),
				"retainedClaims":      stringArraySchema(),
				"downgradedClaims":    stringArraySchema(),
				"unresolvedQuestions": stringArraySchema(),
				"nextActions":         stringArraySchema(),
			},
		)
	default:
		return objectSchema(
			[]string{"fullResponse", "summary"},
			map[string]any{
				"fullResponse": schemaString(),
				"summary":      schemaString(),
			},
		)
	}
}

func TaskOutputJSONSchemaForAgent(task consensus.Task, agent ResolvedAgentRuntime) map[string]any {
	schema := cloneSchemaMap(TaskOutputJSONSchema(task))
	if usesCodexStructuredSchema(agent) {
		enforceAllObjectPropertiesRequired(schema)
	}
	return schema
}

func usesCodexStructuredSchema(agent ResolvedAgentRuntime) bool {
	return agent.Provider.Type == config.ProviderTypeCLI && agent.Provider.CLIType == config.CLITypeCodex
}

func objectSchema(required []string, properties map[string]any) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func arraySchema(items map[string]any) map[string]any {
	return map[string]any{
		"type":  "array",
		"items": items,
	}
}

func schemaString() map[string]any {
	return map[string]any{"type": "string"}
}

func schemaNumber() map[string]any {
	return map[string]any{"type": "number"}
}

func schemaInteger() map[string]any {
	return map[string]any{"type": "integer"}
}

func schemaBoolean() map[string]any {
	return map[string]any{"type": "boolean"}
}

func enumStringSchema(values []string) map[string]any {
	return map[string]any{
		"type": "string",
		"enum": values,
	}
}

func stringArraySchema() map[string]any {
	return arraySchema(schemaString())
}

func cloneSchemaMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	clone := make(map[string]any, len(src))
	for key, value := range src {
		clone[key] = cloneSchemaValue(value)
	}
	return clone
}

func cloneSchemaSlice(src []any) []any {
	if src == nil {
		return nil
	}
	clone := make([]any, len(src))
	for idx, value := range src {
		clone[idx] = cloneSchemaValue(value)
	}
	return clone
}

func cloneSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSchemaMap(typed)
	case []any:
		return cloneSchemaSlice(typed)
	case []string:
		clone := make([]string, len(typed))
		copy(clone, typed)
		return clone
	default:
		return typed
	}
}

func enforceAllObjectPropertiesRequired(schema map[string]any) {
	if schema == nil {
		return
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		required := make([]string, 0, len(properties))
		for key, value := range properties {
			required = append(required, key)
			if nested, ok := value.(map[string]any); ok {
				enforceAllObjectPropertiesRequired(nested)
			}
		}
		slices.Sort(required)
		schema["required"] = required
	}
	if items, ok := schema["items"].(map[string]any); ok {
		enforceAllObjectPropertiesRequired(items)
	}
	if oneOf, ok := schema["oneOf"].([]any); ok {
		for _, option := range oneOf {
			if nested, ok := option.(map[string]any); ok {
				enforceAllObjectPropertiesRequired(nested)
			}
		}
	}
}

func proposalClaimDraftSchema() map[string]any {
	return objectSchema(
		[]string{"statement"},
		map[string]any{
			"title":              schemaString(),
			"statement":          schemaString(),
			"claimType":          enumStringSchema([]string{string(consensus.ClaimTypeFact), string(consensus.ClaimTypeInference), string(consensus.ClaimTypeRecommendation), string(consensus.ClaimTypeAssumption)}),
			"applicability":      schemaString(),
			"boundaryConditions": stringArraySchema(),
			"confidence":         schemaNumber(),
		},
	)
}

// debateClaimDraftSchema is the proposal claim schema plus the debate-only
// category self-classification field.
func debateClaimDraftSchema() map[string]any {
	schema := proposalClaimDraftSchema()
	if properties, ok := schema["properties"].(map[string]any); ok {
		properties["category"] = enumStringSchema([]string{string(consensus.DebateClaimCategoryDomain), string(consensus.DebateClaimCategoryProcess), string(consensus.DebateClaimCategorySynthesis)})
	}
	return schema
}

func challengeDraftSchema() map[string]any {
	return objectSchema(
		[]string{"statement", "kind"},
		map[string]any{
			"claimId":                      schemaString(),
			"statement":                    schemaString(),
			"kind":                         schemaString(),
			"attackType":                   schemaString(),
			"severity":                     enumStringSchema([]string{string(consensus.AttackSeverityLow), string(consensus.AttackSeverityMedium), string(consensus.AttackSeverityHigh)}),
			"requestedChecks":              stringArraySchema(),
			"suggestedFalsificationMethod": schemaString(),
		},
	)
}

func revisionDraftSchema() map[string]any {
	return objectSchema(
		[]string{"targetClaimId", "action"},
		map[string]any{
			"targetClaimId":      schemaString(),
			"action":             enumStringSchema([]string{string(consensus.RevisionActionRevise), string(consensus.RevisionActionDowngrade), string(consensus.RevisionActionWithdraw), string(consensus.RevisionActionUnresolved), string(consensus.RevisionActionUnchanged)}),
			"revisedText":        schemaString(),
			"confidenceDelta":    schemaNumber(),
			"caveats":            stringArraySchema(),
			"boundaryConditions": stringArraySchema(),
			"reason":             schemaString(),
			"unresolved":         schemaBoolean(),
		},
	)
}

func debateJudgementSchema() map[string]any {
	return objectSchema(
		[]string{"claimId", "judgement"},
		map[string]any{
			"claimId":          schemaString(),
			"judgement":        enumStringSchema([]string{string(consensus.DebateJudgementAgree), string(consensus.DebateJudgementDisagree), string(consensus.DebateJudgementRevise), string(consensus.DebateJudgementNoChange)}),
			"rationale":        schemaString(),
			"revisedStatement": schemaString(),
			"mergeWithClaims":  stringArraySchema(),
		},
	)
}

func debateVoteSchema() map[string]any {
	return objectSchema(
		[]string{"claimId", "vote", "confidence"},
		map[string]any{
			"claimId":    schemaString(),
			"vote":       enumStringSchema([]string{string(consensus.DebateVoteAccept), string(consensus.DebateVoteReject), string(consensus.DebateVoteAbstain)}),
			"confidence": schemaNumber(),
			"rationale":  schemaString(),
		},
	)
}

func debateClaimMergeSchema() map[string]any {
	return objectSchema(
		[]string{"sourceClaimId", "targetClaimId", "similarity", "rationale"},
		map[string]any{
			"sourceClaimId": schemaString(),
			"targetClaimId": schemaString(),
			"similarity":    schemaNumber(),
			"rationale":     schemaString(),
		},
	)
}

func delphiResponseSchema(scaleMin, scaleMax int) map[string]any {
	rating := schemaNumber()
	rating["minimum"] = scaleMin
	rating["maximum"] = scaleMax
	return objectSchema(
		[]string{"rating"},
		map[string]any{
			"statementId": schemaString(),
			"statement":   schemaString(),
			"rating":      rating,
			"rationale":   schemaString(),
		},
	)
}

func delphiStatementSchema() map[string]any {
	return objectSchema(
		[]string{"statementId", "statement", "meanRating", "consensusLevel", "responseCount", "lastRound"},
		map[string]any{
			"statementId":           schemaString(),
			"statement":             schemaString(),
			"meanRating":            schemaNumber(),
			"consensusLevel":        schemaNumber(),
			"responseCount":         schemaInteger(),
			"lastRound":             schemaInteger(),
			"representativeReasons": stringArraySchema(),
		},
	)
}

func semanticFindingSchema() map[string]any {
	return objectSchema(
		[]string{"claimId", "verdict", "rationale"},
		map[string]any{
			"claimId":    schemaString(),
			"targetType": schemaString(),
			"verdict":    enumStringSchema([]string{string(consensus.ClaimVerdictSupported), string(consensus.ClaimVerdictRefuted), string(consensus.ClaimVerdictInsufficientEvidence), string(consensus.ClaimVerdictUndetermined)}),
			"confidence": schemaNumber(),
			"rationale":  schemaString(),
		},
	)
}

func arbiterDecisionSchema() map[string]any {
	return objectSchema(
		[]string{"claimId", "verdict"},
		map[string]any{
			"claimId":      schemaString(),
			"verdict":      enumStringSchema([]string{string(consensus.ClaimVerdictSupported), string(consensus.ClaimVerdictRefuted), string(consensus.ClaimVerdictInsufficientEvidence), string(consensus.ClaimVerdictUndetermined)}),
			"confidence":   schemaNumber(),
			"rationale":    schemaString(),
			"evidenceRefs": stringArraySchema(),
		},
	)
}

var (
	floatOutputKeys = map[string]struct{}{
		"confidence":      {},
		"confidenceDelta": {},
		"finalConfidence": {},
		"rating":          {},
		"meanRating":      {},
		"consensusLevel":  {},
		"supportRatio":    {},
	}
	intOutputKeys = map[string]struct{}{
		"responseCount": {},
		"lastRound":     {},
		"round":         {},
	}
)
