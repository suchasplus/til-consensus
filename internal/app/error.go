package app

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ExitSuccess              = 0
	ExitInternalError        = 1
	ExitUsageError           = 2
	ExitConfigNotFound       = 3
	ExitConfigInvalid        = 4
	ExitInputInvalid         = 5
	ExitProviderNotReady     = 6
	ExitProviderAuthFailed   = 7
	ExitProviderRateLimited  = 8
	ExitProviderTimeout      = 9
	ExitProviderSchemaFailed = 10
	ExitRunFailed            = 11
	ExitRunCancelled         = 12
	ExitArtifactNotFound     = 13
	ExitArtifactInvalid      = 14
)

type AppError struct {
	Code    int
	Message string
	Hint    string
	Cause   error
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "til-consensus error"
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func appError(code int, message string, hint string, cause error) error {
	return &AppError{Code: code, Message: message, Hint: hint, Cause: cause}
}

func ExitCodeForError(err error) int {
	if err == nil {
		return ExitSuccess
	}
	var appErr *AppError
	if errors.As(err, &appErr) && appErr.Code != 0 {
		return appErr.Code
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "config file not found") || strings.Contains(text, "cannot find config file"):
		return ExitConfigNotFound
	case strings.Contains(text, "decode yaml config") || strings.Contains(text, "config is invalid") || strings.Contains(text, "unknown provider") || strings.Contains(text, "unknown agent") || strings.Contains(text, "references unknown agent"):
		return ExitConfigInvalid
	case strings.Contains(text, "read run input") || strings.Contains(text, "decode yaml input") || strings.Contains(text, "decode json input") || strings.Contains(text, "missing task") || strings.Contains(text, "task file"):
		return ExitInputInvalid
	case strings.Contains(text, "env ") && strings.Contains(text, " is not set"):
		return ExitProviderAuthFailed
	case strings.Contains(text, "status=401") || strings.Contains(text, "status=403") || strings.Contains(text, "auth"):
		return ExitProviderAuthFailed
	case strings.Contains(text, "status=429") || strings.Contains(text, "rate"):
		return ExitProviderRateLimited
	case strings.Contains(text, "timed out") || strings.Contains(text, "timeout") || strings.Contains(text, "deadline exceeded"):
		return ExitProviderTimeout
	case strings.Contains(text, "schema") || strings.Contains(text, "decode proposal output") || strings.Contains(text, "decode semantic verification output") || strings.Contains(text, "decode arbiter output"):
		return ExitProviderSchemaFailed
	case strings.Contains(text, "artifact") && (strings.Contains(text, "not found") || strings.Contains(text, "no such file")):
		return ExitArtifactNotFound
	case strings.Contains(text, "artifact"):
		return ExitArtifactInvalid
	default:
		return ExitInternalError
	}
}

func FormatError(err error, debug bool) string {
	if err == nil {
		return ""
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		var b strings.Builder
		if appErr.Message != "" {
			b.WriteString(appErr.Message)
		} else if appErr.Cause != nil {
			b.WriteString(appErr.Cause.Error())
		}
		if appErr.Hint != "" {
			b.WriteString("\n")
			b.WriteString("hint: ")
			b.WriteString(appErr.Hint)
		}
		if debug && appErr.Cause != nil && appErr.Cause.Error() != b.String() {
			b.WriteString("\n")
			b.WriteString("cause: ")
			_, _ = fmt.Fprintf(&b, "%+v", appErr.Cause)
		}
		return b.String()
	}
	return err.Error()
}
