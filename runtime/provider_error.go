package runtime

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"strings"

	"github.com/suchasplus/til-consensus/config"
	clirunner "github.com/suchasplus/til-consensus/runtime/cli"
)

type ProviderErrorClass string

const (
	ProviderErrorUnknown         ProviderErrorClass = "unknown"
	ProviderErrorTimeout         ProviderErrorClass = "timeout"
	ProviderErrorCanceled        ProviderErrorClass = "canceled"
	ProviderErrorAuth            ProviderErrorClass = "auth"
	ProviderErrorRateLimited     ProviderErrorClass = "rate_limited"
	ProviderErrorUnavailable     ProviderErrorClass = "unavailable"
	ProviderErrorNetwork         ProviderErrorClass = "network"
	ProviderErrorCommandNotFound ProviderErrorClass = "command_not_found"
	ProviderErrorCommandExit     ProviderErrorClass = "command_exit"
	ProviderErrorParse           ProviderErrorClass = "parse"
)

type ProviderError struct {
	Class      ProviderErrorClass `json:"class"`
	Provider   string             `json:"provider,omitempty"`
	Operation  string             `json:"operation,omitempty"`
	StatusCode int                `json:"statusCode,omitempty"`
	ExitCode   int                `json:"exitCode,omitempty"`
	Message    string             `json:"message,omitempty"`
	Err        error              `json:"-"`
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "provider error"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "provider error"
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func classifyProviderError(err error) *ProviderError {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &ProviderError{Class: ProviderErrorTimeout, Message: err.Error(), Err: err}
	case errors.Is(err, context.Canceled):
		return &ProviderError{Class: ProviderErrorCanceled, Message: err.Error(), Err: err}
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr
	}
	var exitErr *exec.Error
	if errors.As(err, &exitErr) && errors.Is(exitErr, exec.ErrNotFound) {
		return &ProviderError{Class: ProviderErrorCommandNotFound, Message: err.Error(), Err: err}
	}
	var commandErr *clirunner.CommandError
	if errors.As(err, &commandErr) {
		return &ProviderError{
			Class:    classifyCLICommandStderr(commandErr.CLIType, commandErr.Stderr),
			ExitCode: commandErr.ExitCode,
			Message:  err.Error(),
			Err:      err,
		}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return &ProviderError{Class: ProviderErrorNetwork, Message: err.Error(), Err: err}
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "429"):
		return &ProviderError{Class: ProviderErrorRateLimited, Message: err.Error(), Err: err}
	case strings.Contains(message, "401"), strings.Contains(message, "403"), strings.Contains(message, "unauthorized"), strings.Contains(message, "forbidden"):
		return &ProviderError{Class: ProviderErrorAuth, Message: err.Error(), Err: err}
	case strings.Contains(message, "503"), strings.Contains(message, "502"), strings.Contains(message, "504"), strings.Contains(message, "connection refused"):
		return &ProviderError{Class: ProviderErrorUnavailable, Message: err.Error(), Err: err}
	case strings.Contains(message, "stderr="), strings.Contains(message, "stdout="):
		return &ProviderError{Class: ProviderErrorCommandExit, Message: err.Error(), Err: err}
	default:
		return &ProviderError{Class: ProviderErrorUnknown, Message: err.Error(), Err: err}
	}
}

func classifyCLICommandStderr(cliType string, stderr string) ProviderErrorClass {
	switch cliType {
	case config.CLITypeCodex,
		config.CLITypeClaude,
		config.CLITypeGemini,
		config.CLITypeAntigravity,
		config.CLITypeOpenCode,
		config.CLITypePi:
	default:
		return ProviderErrorCommandExit
	}
	message := strings.ToLower(strings.TrimSpace(stderr))
	if message == "" {
		return ProviderErrorCommandExit
	}
	switch {
	case containsAny(message, "rate limit", "rate-limit", "too many requests", "resource_exhausted", "quota exceeded", "status 429", "status=429", "http 429"):
		return ProviderErrorRateLimited
	case containsAny(message, "unauthorized", "authentication failed", "invalid api key", "invalid_api_key", "forbidden", "status 401", "status=401", "http 401", "status 403", "status=403", "http 403"):
		return ProviderErrorAuth
	case containsAny(message, "service unavailable", "connection refused", "bad gateway", "gateway timeout", "status 502", "status=502", "http 502", "status 503", "status=503", "http 503", "status 504", "status=504", "http 504"):
		return ProviderErrorUnavailable
	default:
		return ProviderErrorCommandExit
	}
}

func containsAny(value string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}
