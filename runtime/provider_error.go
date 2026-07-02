package runtime

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"strings"
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
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &ProviderError{Class: ProviderErrorTimeout, Message: err.Error(), Err: err}
	case errors.Is(err, context.Canceled):
		return &ProviderError{Class: ProviderErrorCanceled, Message: err.Error(), Err: err}
	}
	var exitErr *exec.Error
	if errors.As(err, &exitErr) && errors.Is(exitErr, exec.ErrNotFound) {
		return &ProviderError{Class: ProviderErrorCommandNotFound, Message: err.Error(), Err: err}
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
