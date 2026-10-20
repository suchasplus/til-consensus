package delegate

import (
	"encoding/json"
	"strings"

	clirunner "github.com/suchasplus/til-consensus/runtime/cli"
)

type answerStream struct{ raw, published string }

func (s *answerStream) consume(event clirunner.CLIEvent) string {
	if event.Text == "" {
		return ""
	}
	switch event.Type {
	case "text_delta":
		s.raw += event.Text
	case "message":
		s.raw = event.Text
	case "terminal":
		if s.raw == "" {
			s.raw = event.Text
		}
	default:
		return ""
	}
	preview := summaryPreview(s.raw)
	if !strings.HasPrefix(preview, s.published) {
		return ""
	}
	delta := strings.TrimPrefix(preview, s.published)
	s.published = preview
	return delta
}

// summaryPreview is a prefix parser, not a JSON repair step. Only a value at
// coreOutput.summary leaves this function. The final complete response still
// passes the full schema and invocation validator before becoming a message.
func summaryPreview(raw string) string {
	start := -1
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		value, end, complete := prefixString(raw, i)
		if !complete {
			return ""
		}
		next := skipSpace(raw, end)
		if value == "coreOutput" && next < len(raw) && raw[next] == ':' {
			next = skipSpace(raw, next+1)
			if next < len(raw) && raw[next] == '{' {
				start = next + 1
				break
			}
		}
		i = end - 1
	}
	if start < 0 {
		return ""
	}
	depth := 1
	for i := start; i < len(raw); i++ {
		switch raw[i] {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth <= 0 {
				return ""
			}
		case '"':
			value, end, complete := prefixString(raw, i)
			if !complete {
				return ""
			}
			next := skipSpace(raw, end)
			if depth == 1 && value == "summary" && next < len(raw) && raw[next] == ':' {
				next = skipSpace(raw, next+1)
				if next < len(raw) && raw[next] == '"' {
					text, _, _ := prefixString(raw, next)
					return text
				}
				return ""
			}
			i = end - 1
		}
	}
	return ""
}

func skipSpace(raw string, index int) int {
	for index < len(raw) && (raw[index] == ' ' || raw[index] == '\n' || raw[index] == '\r' || raw[index] == '\t') {
		index++
	}
	return index
}

func prefixString(raw string, start int) (string, int, bool) {
	escaped := false
	for i := start + 1; i < len(raw); i++ {
		if escaped {
			escaped = false
			continue
		}
		if raw[i] == '\\' {
			escaped = true
			continue
		}
		if raw[i] == '"' {
			var value string
			if json.Unmarshal([]byte(raw[start:i+1]), &value) != nil {
				return "", i + 1, false
			}
			return value, i + 1, true
		}
	}
	// At most one unfinished JSON escape or UTF-8 character can straddle a
	// chunk. Drop that suffix until the next chunk completes it.
	for end := len(raw); end > start && end >= len(raw)-12; end-- {
		var value string
		if json.Unmarshal([]byte(raw[start:end]+"\""), &value) == nil {
			return value, len(raw), false
		}
	}
	return "", len(raw), false
}
