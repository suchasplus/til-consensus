package viewer

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"strings"
)

func BuildURL(viewerURL string, reportJSON []byte) (string, error) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(reportJSON); err != nil {
		return "", fmt.Errorf("gzip report: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close gzip writer: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(compressed.Bytes())
	base := viewerURL
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base + "#v=1&d=" + encoded, nil
}
