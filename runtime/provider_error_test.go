package runtime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"testing"

	"github.com/suchasplus/til-consensus/config"
	clirunner "github.com/suchasplus/til-consensus/runtime/cli"
)

func TestClassifyProviderErrorUsesOnlyCLIStderr(t *testing.T) {
	tests := []struct {
		name    string
		cliType string
		stderr  string
		stdout  string
		want    ProviderErrorClass
	}{
		{name: "stdout status ignored", cliType: config.CLITypeCodex, stderr: "ordinary failure", stdout: "the task discusses HTTP 429", want: ProviderErrorCommandExit},
		{name: "rate limit", cliType: config.CLITypePi, stderr: "request failed: status=429 rate limit exceeded", want: ProviderErrorRateLimited},
		{name: "auth", cliType: config.CLITypeClaude, stderr: "authentication failed: invalid API key", want: ProviderErrorAuth},
		{name: "grok actual unsigned", cliType: config.CLITypeGrok, stderr: "Not signed in. To authenticate without a browser, run:\n  grok login --device-code", want: ProviderErrorAuth},
		{name: "grok rate limit", cliType: config.CLITypeGrok, stderr: "HTTP 429 rate limit", want: ProviderErrorRateLimited},
		{name: "grok stdout auth ignored", cliType: config.CLITypeGrok, stderr: "ordinary failure", stdout: "Not signed in", want: ProviderErrorCommandExit},
		{name: "unavailable", cliType: config.CLITypeGemini, stderr: "HTTP 503 service unavailable", want: ProviderErrorUnavailable},
		{name: "unknown exit", cliType: config.CLITypeOpenCode, stderr: "process failed", want: ProviderErrorCommandExit},
		{name: "generic never refined", cliType: config.CLITypeGeneric, stderr: "HTTP 429 rate limit", want: ProviderErrorCommandExit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &clirunner.CommandError{CLIType: tt.cliType, Command: []string{"cli"}, ExitCode: 7, Stderr: tt.stderr, Stdout: tt.stdout, Err: errors.New("exit")}
			got := classifyProviderError(err)
			if got.Class != tt.want || got.ExitCode != 7 {
				t.Fatalf("classification: got=%#v want=%s", got, tt.want)
			}
		})
	}
}

func TestClassifyProviderErrorPriority(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ProviderErrorClass
	}{
		{name: "deadline", err: context.DeadlineExceeded, want: ProviderErrorTimeout},
		{name: "cancel", err: context.Canceled, want: ProviderErrorCanceled},
		{name: "claude exit zero aborted envelope", err: &clirunner.ClaudeResultError{Subtype: "error_during_execution", TerminalReason: "aborted_streaming"}, want: ProviderErrorCanceled},
		{name: "wrapped provider deadline", err: &ProviderError{Class: ProviderErrorUnknown, Err: context.DeadlineExceeded}, want: ProviderErrorTimeout},
		{name: "command not found", err: fmt.Errorf("wrapped: %w", &exec.Error{Name: "missing", Err: exec.ErrNotFound}), want: ProviderErrorCommandNotFound},
		{name: "dns", err: &net.DNSError{Err: "no such host", Name: "example.invalid"}, want: ProviderErrorNetwork},
		{name: "api rate limit remains", err: errors.New("provider status=429"), want: ProviderErrorRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyProviderError(tt.err)
			if got.Class != tt.want {
				t.Fatalf("classification: got=%#v want=%s", got, tt.want)
			}
		})
	}
}
