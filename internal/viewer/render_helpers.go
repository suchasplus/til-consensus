package viewer

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/suchasplus/til-consensus/consensus"
)

func normalizeFormat(value string) string {
	switch strings.TrimSpace(value) {
	case FormatMarkdown:
		return FormatMarkdown
	case FormatJSON:
		return FormatJSON
	default:
		return FormatText
	}
}

func normalizeSections(sections []string) []string {
	if len(sections) == 0 {
		return []string{SectionAll}
	}
	out := make([]string, 0, len(sections))
	seen := map[string]struct{}{}
	for _, section := range sections {
		trimmed := strings.TrimSpace(section)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return []string{SectionAll}
	}
	return out
}

func lineageParentRequestID(lineage *consensus.RunLineage) string {
	if lineage == nil {
		return ""
	}
	return lineage.ParentRequestID
}

func lineageParentSessionID(lineage *consensus.RunLineage) string {
	if lineage == nil {
		return ""
	}
	return lineage.ParentSessionID
}

func lineageParentCaseID(lineage *consensus.RunLineage) string {
	if lineage == nil {
		return ""
	}
	return lineage.ParentCaseID
}

func lineageTrigger(lineage *consensus.RunLineage) string {
	if lineage == nil {
		return ""
	}
	return lineage.Trigger
}

func lineageCaseID(result consensus.RunResult) string {
	if result.CaseManifest == nil {
		return ""
	}
	return result.CaseManifest.CaseID
}

func artifactRefPath(ref *consensus.ArtifactRef) string {
	if ref == nil {
		return ""
	}
	return ref.Path
}

func sectionSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}

func shouldRenderSection(doc Document, requested map[string]bool, section string) bool {
	if requested[section] {
		return true
	}
	if !requested[SectionAll] {
		return false
	}
	switch section {
	case SectionOverview, SectionArtifacts:
		return true
	case SectionClaims, SectionChallenges, SectionVerifications:
		return doc.Overview.Mode == string(consensus.WorkflowModeAdjudication)
	case SectionObservations:
		return len(doc.Observations) > 0
	case SectionFollowups:
		return len(doc.FollowUps) > 0
	case SectionDebug:
		return false
	case SectionRounds:
		return doc.Overview.Mode == string(consensus.WorkflowModeFreeDebate) || doc.Overview.Mode == string(consensus.WorkflowModeDelphi)
	case SectionVotes:
		return doc.Overview.Mode == string(consensus.WorkflowModeFreeDebate)
	case SectionStatements, SectionConvergence:
		return doc.Overview.Mode == string(consensus.WorkflowModeDelphi)
	default:
		return false
	}
}

func displayRunDir(path string) string {
	if path == "" {
		return "."
	}
	return filepath.Base(path)
}

func displayCompanionPath(runDir string, path string) string {
	if path == "" {
		return ""
	}
	rel, err := filepath.Rel(runDir, path)
	if err == nil && !strings.HasPrefix(rel, "..") {
		if rel == "." {
			return "./"
		}
		return "./" + filepath.ToSlash(rel)
	}
	return path
}

func writeTextHeading(b *strings.Builder, title string) {
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString(title)
	b.WriteString("\n")
}

func sumCounts(values map[string]int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func compactPayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := stringValue(payload, key)
		if value == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
	}
	return strings.Join(parts, " ")
}

func stringValue(payload map[string]any, key string) string {
	if len(payload) == 0 {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch value := payload[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func sanitizeFilename(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "\n", "-", "\t", "-")
	return replacer.Replace(trimmed)
}
