package consensus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type externalSourceResult struct {
	Source       ExternalCommandSource
	Summary      string
	Excerpt      string
	Artifact     *ArtifactRef
	MatchedOK    bool
	Contradicted bool
	ExecFailed   bool
	FailureClass string
	Notes        []string
	Metadata     map[string]any
}

type structuredParseResult struct {
	Summary    string
	Excerpt    string
	Notes      []string
	Metadata   map[string]any
	Success    bool
	HasSuccess bool
	Failure    bool
	HasFailure bool
}

func runExternalCommandSource(ctx context.Context, deps EngineDeps, clock Clock, ids IDFactory, request StartRequest, sessionID string, source ExternalCommandSource, stage string, metadata map[string]string) (externalSourceResult, error) {
	result := externalSourceResult{
		Source: source,
	}
	workdir := source.Workdir
	if workdir == "" && request.TaskSpec.WorkspaceSnapshot != nil {
		workdir = request.TaskSpec.WorkspaceSnapshot.Root
	}
	if workdir == "" {
		workdir = "."
	}
	cmd := exec.CommandContext(ctx, source.Command, source.Args...)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), renderExternalSourceEnv(request, sessionID, stage, metadata, source.Env)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	artifact, artifactErr := writeExternalSourceArtifact(artifactDir(deps.ArtifactDir), ids, source.Name, stage, stdout.Bytes(), stderr.Bytes())
	if artifactErr == nil {
		result.Artifact = artifact
	}
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = strings.TrimSpace(stderr.String())
	}
	result.Excerpt = truncateExcerpt(output)
	if err != nil {
		result.ExecFailed = true
		result.Contradicted = true
		result.FailureClass = "command_exec_failed"
		result.Summary = fmt.Sprintf("external source %s 执行失败: %v", source.Name, err)
		result.Notes = append(result.Notes, classifyCommandFailure(err))
		return result, nil
	}
	parsed, parseErr := parseExternalSourceOutput(source.Parsing, output)
	if parseErr != nil {
		result.FailureClass = "structured_parse_failed"
		result.Notes = append(result.Notes, "structured_parse_failed:"+parseErr.Error())
	} else {
		result.Metadata = cloneAnyMap(parsed.Metadata)
		result.Notes = append(result.Notes, parsed.Notes...)
		if strings.TrimSpace(parsed.Summary) != "" {
			result.Summary = parsed.Summary
		}
		if strings.TrimSpace(parsed.Excerpt) != "" {
			result.Excerpt = truncateExcerpt(parsed.Excerpt)
		}
		if parsed.HasFailure && parsed.Failure {
			result.Contradicted = true
			result.FailureClass = "structured_failure"
			if strings.TrimSpace(result.Summary) == "" {
				result.Summary = fmt.Sprintf("external source %s 命中结构化 failure 条件", source.Name)
			}
			return result, nil
		}
		if parsed.HasSuccess {
			result.MatchedOK = parsed.Success
			if strings.TrimSpace(result.Summary) == "" {
				if parsed.Success {
					result.Summary = fmt.Sprintf("external source %s 命中结构化 success 条件", source.Name)
				} else {
					result.Summary = fmt.Sprintf("external source %s 结构化结果未命中 success 条件", source.Name)
				}
			}
			if source.SuccessPattern == "" && source.FailurePattern == "" {
				return result, nil
			}
		}
	}
	if source.FailurePattern != "" {
		matched, matchErr := matchesPattern(source.FailurePattern, stdout.String()+"\n"+stderr.String())
		if matchErr != nil {
			result.Notes = append(result.Notes, "failure_pattern_invalid:"+matchErr.Error())
		} else if matched {
			result.Contradicted = true
			result.FailureClass = "failure_pattern_matched"
			result.Summary = fmt.Sprintf("external source %s 命中 failure_pattern", source.Name)
			return result, nil
		}
	}
	if source.SuccessPattern != "" {
		matched, matchErr := matchesPattern(source.SuccessPattern, stdout.String()+"\n"+stderr.String())
		if matchErr != nil {
			result.Notes = append(result.Notes, "success_pattern_invalid:"+matchErr.Error())
			result.FailureClass = "success_pattern_invalid"
			result.Summary = fmt.Sprintf("external source %s 执行成功，但 success_pattern 无法解析", source.Name)
			return result, nil
		}
		result.MatchedOK = matched
		if matched {
			result.Summary = fmt.Sprintf("external source %s 命中 success_pattern", source.Name)
		} else {
			result.Summary = fmt.Sprintf("external source %s 执行成功，但未命中 success_pattern", source.Name)
		}
		return result, nil
	}
	result.MatchedOK = true
	if strings.TrimSpace(result.Summary) == "" {
		result.Summary = fmt.Sprintf("external source %s 执行成功", source.Name)
	}
	return result, nil
}

func renderExternalSourceEnv(request StartRequest, sessionID string, stage string, metadata map[string]string, values map[string]string) []string {
	out := []string{
		"TIL_CONSENSUS_REQUEST_ID=" + request.RequestID,
		"TIL_CONSENSUS_SESSION_ID=" + sessionID,
		"TIL_CONSENSUS_STAGE=" + stage,
	}
	if request.TaskSpec.WorkspaceSnapshot != nil && strings.TrimSpace(request.TaskSpec.WorkspaceSnapshot.Root) != "" {
		out = append(out, "TIL_CONSENSUS_WORKSPACE_ROOT="+request.TaskSpec.WorkspaceSnapshot.Root)
	}
	for key, value := range metadata {
		out = append(out, key+"="+value)
	}
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}

func artifactDir(dir string) string {
	return strings.TrimSpace(dir)
}

func writeExternalSourceArtifact(dir string, ids IDFactory, sourceName string, stage string, stdout []byte, stderr []byte) (*ArtifactRef, error) {
	if dir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	body := bytes.NewBuffer(nil)
	_, _ = body.WriteString("# stdout\n")
	_, _ = body.Write(stdout)
	_, _ = body.WriteString("\n# stderr\n")
	_, _ = body.Write(stderr)
	prefix := "external"
	if ids != nil {
		prefix = ids.NewEntityID("external")
	}
	name := filepath.Join(dir, sanitizeFilename(prefix+"-"+stage+"-"+sourceName)+".log")
	if err := os.WriteFile(name, body.Bytes(), 0o644); err != nil {
		return nil, err
	}
	hash := sha256.Sum256(body.Bytes())
	return &ArtifactRef{
		Path:      name,
		Hash:      hex.EncodeToString(hash[:]),
		MediaType: "text/plain",
	}, nil
}

func truncateExcerpt(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 240 {
		return value
	}
	return value[:240] + "..."
}

func matchesPattern(pattern string, body string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(body), nil
}

func parseExternalSourceOutput(parsing ExternalCommandParsing, body string) (structuredParseResult, error) {
	if parsing.Mode == "" || parsing.Mode == ExternalCommandParseModeText {
		return structuredParseResult{}, nil
	}
	switch parsing.Mode {
	case ExternalCommandParseModeJSON:
		return parseStructuredExternalOutput(parsing, body, parseJSONPayload)
	case ExternalCommandParseModeYAML:
		return parseStructuredExternalOutput(parsing, body, parseYAMLPayload)
	case ExternalCommandParseModeXML:
		return parseStructuredExternalOutput(parsing, body, parseXMLPayload)
	default:
		return structuredParseResult{}, fmt.Errorf("unsupported parse mode: %s", parsing.Mode)
	}
}

func parseStructuredExternalOutput(parsing ExternalCommandParsing, body string, parser func(string) (any, error)) (structuredParseResult, error) {
	payload, err := parser(body)
	if err != nil {
		return structuredParseResult{}, err
	}
	result := structuredParseResult{
		Metadata: map[string]any{},
	}
	for _, path := range parsing.RequiredPaths {
		if _, ok := jsonPathLookup(payload, path); !ok {
			return structuredParseResult{}, fmt.Errorf("required path missing: %s", path)
		}
	}
	if value, ok := jsonPathLookup(payload, parsing.SummaryPath); ok {
		result.Summary = coerceStructuredString(value)
	}
	if value, ok := jsonPathLookup(payload, parsing.ExcerptPath); ok {
		result.Excerpt = coerceStructuredString(value)
	}
	if value, ok := jsonPathLookup(payload, parsing.NotesPath); ok {
		result.Notes = coerceStructuredStrings(value)
	}
	if value, ok := jsonPathLookup(payload, parsing.SuccessPath); ok {
		if parsed, ok := coerceStructuredBool(value); ok {
			result.Success = parsed
			result.HasSuccess = true
		}
	}
	if value, ok := jsonPathLookup(payload, parsing.FailurePath); ok {
		if parsed, ok := coerceStructuredBool(value); ok {
			result.Failure = parsed
			result.HasFailure = true
		}
	}
	for key, path := range parsing.MetadataPaths {
		if value, ok := jsonPathLookup(payload, path); ok {
			result.Metadata[key] = value
		}
	}
	if len(result.Metadata) == 0 {
		result.Metadata = nil
	}
	return result, nil
}

func parseJSONPayload(body string) (any, error) {
	var payload any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func parseYAMLPayload(body string) (any, error) {
	var payload any
	if err := yaml.Unmarshal([]byte(body), &payload); err != nil {
		return nil, err
	}
	return normalizeStructuredValue(payload), nil
}

func parseXMLPayload(body string) (any, error) {
	decoder := xml.NewDecoder(strings.NewReader(body))
	var root *xmlNode
	var stack []*xmlNode
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		switch current := token.(type) {
		case xml.StartElement:
			node := &xmlNode{Name: current.Name.Local}
			stack = append(stack, node)
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			stack[len(stack)-1].Text += string(current)
		case xml.EndElement:
			if len(stack) == 0 {
				continue
			}
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				root = node
				continue
			}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, node)
		}
	}
	if root == nil {
		return nil, fmt.Errorf("empty xml payload")
	}
	return map[string]any{root.Name: xmlNodeValue(root)}, nil
}

type xmlNode struct {
	Name     string
	Text     string
	Children []*xmlNode
}

func xmlNodeValue(node *xmlNode) any {
	if node == nil {
		return nil
	}
	if len(node.Children) == 0 {
		return strings.TrimSpace(node.Text)
	}
	value := map[string]any{}
	if text := strings.TrimSpace(node.Text); text != "" {
		value["_text"] = text
	}
	grouped := map[string][]any{}
	for _, child := range node.Children {
		grouped[child.Name] = append(grouped[child.Name], xmlNodeValue(child))
	}
	for key, items := range grouped {
		if len(items) == 1 {
			value[key] = items[0]
			continue
		}
		value[key] = items
	}
	return value
}

func jsonPathLookup(root any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	current := []any{root}
	for _, raw := range strings.Split(path, ".") {
		part := strings.TrimSpace(raw)
		if part == "" {
			return nil, false
		}
		name, index, hasIndex, wildcard, ok := splitJSONPathPart(part)
		if !ok {
			return nil, false
		}
		next := make([]any, 0)
		for _, candidate := range current {
			resolved, ok := stepStructuredPath(candidate, name, hasIndex, index, wildcard)
			if !ok {
				continue
			}
			next = append(next, resolved...)
		}
		if len(next) == 0 {
			return nil, false
		}
		current = next
	}
	if len(current) == 1 {
		return current[0], true
	}
	return current, true
}

func stepStructuredPath(current any, name string, hasIndex bool, index int, wildcard bool) ([]any, bool) {
	value := current
	if name != "" {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		var found bool
		value, found = obj[name]
		if !found {
			return nil, false
		}
	}
	if wildcard {
		list, ok := value.([]any)
		if !ok || len(list) == 0 {
			return nil, false
		}
		return list, true
	}
	if hasIndex {
		list, ok := value.([]any)
		if !ok || index < 0 || index >= len(list) {
			return nil, false
		}
		return []any{list[index]}, true
	}
	return []any{value}, true
}

func splitJSONPathPart(part string) (string, int, bool, bool, bool) {
	if !strings.Contains(part, "[") {
		return part, 0, false, false, true
	}
	open := strings.Index(part, "[")
	close := strings.Index(part, "]")
	if open < 0 || close <= open+1 || close != len(part)-1 {
		return "", 0, false, false, false
	}
	indexToken := part[open+1 : close]
	if indexToken == "*" {
		return part[:open], 0, false, true, true
	}
	index, err := strconv.Atoi(indexToken)
	if err != nil {
		return "", 0, false, false, false
	}
	return part[:open], index, true, false, true
}

func coerceStructuredString(value any) string {
	switch item := value.(type) {
	case string:
		return strings.TrimSpace(item)
	case json.Number:
		return item.String()
	case float64:
		return strconv.FormatFloat(item, 'f', -1, 64)
	case bool:
		if item {
			return "true"
		}
		return "false"
	default:
		body, err := json.Marshal(item)
		if err != nil {
			return ""
		}
		return string(body)
	}
}

func coerceStructuredStrings(value any) []string {
	switch item := value.(type) {
	case []any:
		out := make([]string, 0, len(item))
		for _, part := range item {
			if text := strings.TrimSpace(coerceStructuredString(part)); text != "" {
				out = append(out, text)
			}
		}
		return dedupeStrings(out)
	case []string:
		return dedupeStrings(item)
	default:
		if text := strings.TrimSpace(coerceStructuredString(item)); text != "" {
			return []string{text}
		}
		return nil
	}
}

func coerceStructuredBool(value any) (bool, bool) {
	switch item := value.(type) {
	case bool:
		return item, true
	case string:
		switch strings.ToLower(strings.TrimSpace(item)) {
		case "true", "yes", "1", "pass", "passed", "ok":
			return true, true
		case "false", "no", "0", "fail", "failed":
			return false, true
		default:
			return false, false
		}
	case float64:
		return item != 0, true
	case json.Number:
		number, err := item.Float64()
		if err != nil {
			return false, false
		}
		return number != 0, true
	default:
		return false, false
	}
}

func normalizeStructuredValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeStructuredValue(item)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[coerceStructuredString(key)] = normalizeStructuredValue(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, normalizeStructuredValue(item))
		}
		return out
	default:
		return value
	}
}
