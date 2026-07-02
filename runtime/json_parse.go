package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type JSONParseError struct {
	Message            string
	RawText            string
	ExtractedCandidate string
}

func (e *JSONParseError) Error() string {
	return e.Message
}

// StripCodeFences removes one outer markdown code fence when present. It does
// not inspect task schema or field names.
func StripCodeFences(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```") && strings.HasSuffix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 2 {
			return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		}
	}
	return trimmed
}

// ExtractJSONObject returns the first balanced JSON object candidate from text.
// It is intentionally syntax-oriented only: wrapper text is trimmed, but task
// schema enforcement happens later during typed decode/validation.
func ExtractJSONObject(text string) string {
	cleaned := StripCodeFences(text)
	if cleaned == "" {
		return ""
	}
	if strings.HasPrefix(cleaned, "{") && strings.HasSuffix(cleaned, "}") {
		return cleaned
	}
	start := strings.Index(cleaned, "{")
	if start == -1 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for idx := start; idx < len(cleaned); idx++ {
		ch := cleaned[idx]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == '{' {
			depth++
		}
		if ch == '}' {
			depth--
			if depth == 0 {
				return cleaned[start : idx+1]
			}
		}
	}
	return ""
}

// StrictJSONObjectBytes accepts exactly one JSON object with no wrapper text and
// no trailing data. This is the "strict schema first" path used by strict
// compliance telemetry before any syntax recovery is attempted.
func StrictJSONObjectBytes(text string) ([]byte, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("strict JSON object required: output is empty")
	}
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return nil, fmt.Errorf("strict JSON object required: output must contain exactly one JSON object and no wrapper text")
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("strict JSON decode failed: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != nil && err != io.EOF {
		return nil, fmt.Errorf("strict JSON object required: unexpected trailing data: %w", err)
	}
	return json.Marshal(value)
}

// ParseJSONObject performs syntax-only recovery for cooperative-but-imperfect
// model output. It may strip code fences, extract the first object, remove
// trailing commas, or escape stray quotes. It never applies task-specific alias
// or enum mappings; schema enforcement stays in typed decode/validation.
func ParseJSONObject(text string) (any, error) {
	candidate := ExtractJSONObject(text)
	if candidate == "" {
		return nil, &JSONParseError{Message: "no JSON object found in output", RawText: text}
	}
	value, err := decodeJSONValue(candidate)
	if err == nil {
		return value, nil
	}
	if repaired := removeTrailingCommas(candidate); repaired != candidate {
		if value, err := decodeJSONValue(repaired); err == nil {
			return value, nil
		}
	}
	if repaired := repairInnerQuotes(candidate); repaired != candidate {
		if value, err := decodeJSONValue(repaired); err == nil {
			return value, nil
		}
	}
	if repaired, ok := escapeStrayQuotes(candidate); ok {
		if value, err := decodeJSONValue(repaired); err == nil {
			return value, nil
		}
	}
	return nil, &JSONParseError{
		Message:            "invalid JSON output",
		RawText:            text,
		ExtractedCandidate: candidate,
	}
}

func decodeJSONValue(text string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != nil && err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing data: %w", err)
	}
	return value, nil
}

func removeTrailingCommas(input string) string {
	replacer := strings.NewReplacer(",}", "}", ",]", "]")
	out := input
	for {
		next := replacer.Replace(out)
		if next == out {
			return out
		}
		out = next
	}
}

func repairInnerQuotes(candidate string) string {
	var b strings.Builder
	inString := false
	escaped := false
	for i := 0; i < len(candidate); i++ {
		ch := candidate[i]
		if escaped {
			b.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			b.WriteByte(ch)
			escaped = true
			continue
		}
		if ch == '"' && inString {
			j := i + 1
			for j < len(candidate) && (candidate[j] == ' ' || candidate[j] == '\n' || candidate[j] == '\r' || candidate[j] == '\t') {
				j++
			}
			if j < len(candidate) && !strings.ContainsRune(`,}][:]`, rune(candidate[j])) {
				b.WriteString(`\"`)
				continue
			}
		}
		if ch == '"' {
			inString = !inString
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func escapeStrayQuotes(candidate string) (string, bool) {
	text := candidate
	lastPos := -1
	for i := 0; i < 32; i++ {
		var tmp any
		err := json.Unmarshal([]byte(text), &tmp)
		if err == nil {
			return text, true
		}
		syntaxErr, ok := err.(*json.SyntaxError)
		if !ok {
			return "", false
		}
		pos := int(syntaxErr.Offset) - 1
		if pos <= lastPos {
			return "", false
		}
		lastPos = pos
		quotePos := -1
		for j := pos; j >= 0; j-- {
			if text[j] != '"' {
				continue
			}
			backslashes := 0
			for k := j - 1; k >= 0 && text[k] == '\\'; k-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				quotePos = j
				break
			}
		}
		if quotePos == -1 {
			return "", false
		}
		text = text[:quotePos] + `\` + text[quotePos:]
	}
	return "", false
}
