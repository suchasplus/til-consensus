package viewer

import (
	"encoding/json"
	"fmt"
)

func RenderDocument(doc Document, options RenderOptions) (string, error) {
	format := normalizeFormat(options.Format)
	switch format {
	case FormatMarkdown:
		return renderMarkdown(doc, options.Verbose), nil
	case FormatJSON:
		body, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal view json: %w", err)
		}
		return string(body) + "\n", nil
	default:
		return renderText(doc, options.Verbose), nil
	}
}
