package workflow

import (
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
)

var anonymousNames = regexp.MustCompile(`(?i)\b(?:Claude(?:\s+Code)?|Grok(?:\s+Build)?|Codex|AGY|Antigravity)\b`)
var anonymousModels = regexp.MustCompile(`(?i)\b(?:gpt-[a-z0-9][a-z0-9.-]*|claude-(?:opus|sonnet|haiku)[a-z0-9.-]*|grok-[0-9][a-z0-9.-]*|gemini-[0-9][a-z0-9.-]*)\b`)
var anonymousPaths = regexp.MustCompile(`(?:/(?:Users|home|tmp|private|var)/[^\s"'<>,;:()\[\]，。；：、（）【】《》]*)`)
var anonymousSessions = regexp.MustCompile(`(?i)\b(?:session[_ -]?id|conversation[_ -]?id|thread[_ -]?id|invocation[_ -]?id)\s*[:=]\s*[^\s,;"'<>，。；：、（）【】《》]+`)

// AnonymousPeerText treats previous model prose as untrusted source material.
// It removes known CLI/model/path/session identifiers, including self-reports.
// This is a deterministic projection, not a guarantee against every possible
// semantic self-identification. The owner's original audit text is untouched.
func AnonymousPeerText(text string) string {
	text = anonymousPaths.ReplaceAllString(text, "[匿名路径]")
	text = anonymousSessions.ReplaceAllString(text, "[匿名会话]")
	text = anonymousModels.ReplaceAllString(text, "匿名模型")
	return anonymousNames.ReplaceAllString(text, "匿名参与者")
}

func anonymousReference(value string) string {
	if value == "" {
		return ""
	}
	if suffix := strings.TrimPrefix(value, "anonymous-ref-"); suffix != value && len(suffix) == 16 {
		if _, err := hex.DecodeString(suffix); err == nil {
			return value
		}
	}
	return "anonymous-ref-" + Hash(value)[:16]
}

func anonymousValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			switch strings.ToLower(key) {
			case "raisedby", "agentid", "ownerid", "proposedby", "proposerid", "author", "authorid", "invocationid", "artifactref", "artifactpath", "sessionid", "conversationid", "threadid", "model", "actualmodel", "requestedmodel", "reportedmodel", "modelbindings", "actualmetadata", "cliversion", "binarypath", "command":
				continue
			case "id", "claimid", "statementid", "parentrunid", "requestid", "proposalid":
				if text, ok := child.(string); ok {
					out[key] = anonymousReference(text)
					continue
				}
			case "resolutionrefs", "exclusivewith", "evidencerefs":
				if refs, ok := child.([]any); ok {
					mapped := make([]any, 0, len(refs))
					for _, ref := range refs {
						if text, ok := ref.(string); ok {
							mapped = append(mapped, anonymousReference(text))
						}
					}
					out[key] = mapped
					continue
				}
			}
			out[key] = anonymousValue(child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			out[i] = anonymousValue(child)
		}
		return out
	case string:
		return AnonymousPeerText(typed)
	default:
		return value
	}
}

func anonymousMaterial(material consensus.MaterialRef) consensus.MaterialRef {
	material.Path = ""
	allowed := map[string]any{}
	for _, key := range []string{"source", "sourceSha256", "rawSha256", "byteLength", "normalization", "snapshotFormat"} {
		if value, ok := material.Metadata[key]; ok {
			if key == "source" {
				if text, yes := value.(string); yes {
					value = AnonymousPeerText(text)
				}
			}
			allowed[key] = value
		}
	}
	material.Metadata = allowed
	if AnonymousPeerText(material.ID) != material.ID {
		material.ID = anonymousReference(material.ID)
	}
	material.Title = "匿名参考候选与材料"
	var value any
	if material.Metadata["snapshotFormat"] == "tcview-material-v1" && material.Kind == "source_material" {
		// User supplied facts remain byte-exact inside the snapshot. Only the
		// provenance title/source is projected; peer generated prose uses the
		// separate projection below.
		var snapshot map[string]any
		if json.Unmarshal([]byte(material.Content), &snapshot) == nil {
			snapshot["title"] = "匿名用户材料"
			if id, ok := snapshot["id"].(string); ok && AnonymousPeerText(id) != id {
				snapshot["id"] = anonymousReference(id)
			}
			if source, ok := snapshot["source"].(string); ok {
				snapshot["source"] = AnonymousPeerText(source)
			}
			if body, err := json.Marshal(snapshot); err == nil {
				material.Content = string(body)
				material.Hash = Hash(material.Content)
				return material
			}
		}
	}
	if json.Unmarshal([]byte(material.Content), &value) == nil {
		if projected, err := json.Marshal(anonymousValue(value)); err == nil {
			material.Content = string(projected)
		}
	} else {
		material.Content = AnonymousPeerText(material.Content)
	}
	material.Hash = Hash(material.Content)
	return material
}

func anonymousTaskSpec(spec consensus.TaskSpec) consensus.TaskSpec {
	materials := make([]consensus.MaterialRef, len(spec.Materials))
	for i, material := range spec.Materials {
		materials[i] = anonymousMaterial(material)
	}
	spec.Materials = materials
	// The user's own goal remains intact: a task comparing CLI products may
	// legitimately name them. Only generated peer material/source metadata is
	// projected. Models must not infer authorship from the preserved subject.
	return spec
}

func anonymousStatements(statements []consensus.DelphiStatement) []consensus.DelphiStatement {
	out := append([]consensus.DelphiStatement(nil), statements...)
	for i := range out {
		out[i].Statement = AnonymousPeerText(out[i].Statement)
		out[i].RepresentativeReasons = append([]string(nil), out[i].RepresentativeReasons...)
		for j := range out[i].RepresentativeReasons {
			out[i].RepresentativeReasons[j] = AnonymousPeerText(out[i].RepresentativeReasons[j])
		}
	}
	return out
}

// AnonymousDelphiTask projects every peer-controlled input, including prior
// summaries and aggregate reasons. TaskMeta.AgentID and statement IDs remain
// stable because they are required to validate this participant's own response.
func AnonymousDelphiTask(task consensus.Task) consensus.Task {
	switch typed := task.(type) {
	case consensus.DelphiQuestionnaireTask:
		typed.TaskSpec = anonymousTaskSpec(typed.TaskSpec)
		typed.PreviousStatements = anonymousStatements(typed.PreviousStatements)
		typed.PreviousSummary = AnonymousPeerText(typed.PreviousSummary)
		return typed
	case consensus.DelphiRevisionTask:
		typed.TaskSpec = anonymousTaskSpec(typed.TaskSpec)
		typed.StatementSummaries = anonymousStatements(typed.StatementSummaries)
		typed.PreviousSummary = AnonymousPeerText(typed.PreviousSummary)
		return typed
	case consensus.DelphiFacilitatorSummaryTask:
		typed.TaskSpec = anonymousTaskSpec(typed.TaskSpec)
		typed.StatementSummaries = anonymousStatements(typed.StatementSummaries)
		return typed
	}
	return task
}
