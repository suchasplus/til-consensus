package cli

import (
	"strings"
	"testing"
)

func TestReducePiJSONLOutput(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{
			name: "joins text deltas",
			raw: strings.Join([]string{
				`{"type":"session","id":"session-1"}`,
				`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"{\"ok\":"}}`,
				`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"true}"}}`,
				`{"type":"message_end","message":{"role":"assistant","stopReason":"stop"}}`,
			}, "\n"),
			want: `{"ok":true}`,
		},
		{
			name: "ignores thinking and tool events",
			raw: strings.Join([]string{
				`{"type":"message_update","assistantMessageEvent":{"type":"thinking_delta","delta":"private reasoning"}}`,
				`{"type":"message_update","assistantMessageEvent":{"type":"toolcall_end","delta":"tool log"}}`,
				`{"type":"message_end","message":{"role":"toolResult","content":[]}}`,
				`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"答案"}}`,
			}, "\n"),
			want: "答案",
		},
		{
			name:    "assistant error",
			raw:     `{"type":"message_end","message":{"role":"assistant","errorMessage":"model unavailable"}}`,
			wantErr: "model unavailable",
		},
		{
			name: "assistant error wins after text",
			raw: strings.Join([]string{
				`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"partial"}}`,
				`{"type":"message_end","message":{"role":"assistant","errorMessage":"quota exceeded"}}`,
			}, "\n"),
			wantErr: "quota exceeded",
		},
		{name: "empty stream", wantErr: "no assistant text"},
		{name: "only blank lines", raw: "\n  \n", wantErr: "no assistant text"},
		{name: "only thinking", raw: `{"type":"message_update","assistantMessageEvent":{"type":"thinking_delta","delta":"thinking"}}`, wantErr: "no assistant text"},
		{name: "empty delta", raw: `{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":""}}`, wantErr: "no assistant text"},
		{name: "malformed line", raw: "not-json", wantErr: "protocol error at line 1"},
		{name: "malformed line after text", raw: "{\"type\":\"message_update\",\"assistantMessageEvent\":{\"type\":\"text_delta\",\"delta\":\"partial\"}}\nnot-json", wantErr: "protocol error at line 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reducePiJSONLOutput(tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				if got != "" {
					t.Fatalf("expected empty output on error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("reducePiJSONLOutput failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("output: got=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestReducePiJSONLOutputRejectsOversizedLine(t *testing.T) {
	raw := `{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"` + strings.Repeat("x", maxPiJSONLLineBytes) + `"}}`
	_, err := reducePiJSONLOutput(raw)
	if err == nil || !strings.Contains(err.Error(), "protocol error") {
		t.Fatalf("expected oversized-line protocol error, got %v", err)
	}
}
