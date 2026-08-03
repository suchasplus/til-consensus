package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/suchasplus/til-consensus/config"
)

const (
	maxCLIPromptBytes = 8 * 1024 * 1024

	promptDirPlaceholder  = "<prompt-dir>"
	promptFilePlaceholder = "<prompt-file>"
)

var removePromptDir = os.RemoveAll

type promptTransport struct {
	Args    []string
	Stdin   string
	Cleanup func() error
}

// promptFileProvisioner supplies the directory and file path that the argv will
// reference. Real execution writes an actual private file; preview substitutes
// placeholders without touching the filesystem. Routing both through
// buildPromptTransport is what guarantees a preview can never drift from the
// command that actually runs.
type promptFileProvisioner func(prompt string) (dir string, path string, cleanup func() error, err error)

func preparePromptTransport(cliType string, providerModel string, prompt string, reasoning string) (promptTransport, error) {
	return buildPromptTransport(cliType, providerModel, prompt, reasoning, writePrivatePromptFile)
}

func previewPromptTransport(cliType string, providerModel string, prompt string, reasoning string) (promptTransport, error) {
	return buildPromptTransport(cliType, providerModel, prompt, reasoning, placeholderPromptFile)
}

func buildPromptTransport(cliType string, providerModel string, prompt string, reasoning string, provision promptFileProvisioner) (promptTransport, error) {
	if err := validatePromptSize(prompt); err != nil {
		return promptTransport{}, err
	}
	if cliType != config.CLITypeAntigravity {
		return promptTransport{
			Args:    buildBaseArgs(cliType, providerModel, reasoning),
			Stdin:   prompt,
			Cleanup: noopPromptCleanup,
		}, nil
	}
	dir, path, cleanup, err := provision(prompt)
	if err != nil {
		return promptTransport{}, err
	}
	return promptTransport{
		Args:    agyPromptArgs(providerModel, dir, path),
		Cleanup: cleanup,
	}, nil
}

// agyPromptArgs is the single source of truth for antigravity's argv shape.
func agyPromptArgs(providerModel string, dir string, path string) []string {
	return []string{
		"--add-dir", dir,
		"--dangerously-skip-permissions",
		"--model", providerModel,
		"-p", agyPromptBootstrap(path),
	}
}

func placeholderPromptFile(string) (string, string, func() error, error) {
	return promptDirPlaceholder,
		filepath.Join(promptDirPlaceholder, promptFilePlaceholder),
		noopPromptCleanup,
		nil
}

func noopPromptCleanup() error { return nil }

func validatePromptSize(prompt string) error {
	if len([]byte(prompt)) > maxCLIPromptBytes {
		return fmt.Errorf("cli prompt exceeds %d byte safety limit", maxCLIPromptBytes)
	}
	return nil
}

func writePrivatePromptFile(prompt string) (dir string, path string, cleanup func() error, err error) {
	dir, err = os.MkdirTemp("", "til-consensus-agy-prompt-*")
	if err != nil {
		return "", "", nil, fmt.Errorf("create agy prompt directory: %w", err)
	}
	cleanup = func() error { return removePromptDir(dir) }
	failed := true
	defer func() {
		if failed {
			_ = cleanup()
		}
	}()
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", "", nil, fmt.Errorf("secure agy prompt directory: %w", err)
	}
	path = filepath.Join(dir, "prompt.txt")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", "", nil, fmt.Errorf("create agy prompt file: %w", err)
	}
	if _, err := file.WriteString(prompt); err != nil {
		_ = file.Close()
		return "", "", nil, fmt.Errorf("write agy prompt file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", "", nil, fmt.Errorf("close agy prompt file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", "", nil, fmt.Errorf("secure agy prompt file: %w", err)
	}
	failed = false
	return dir, path, cleanup, nil
}

func agyPromptBootstrap(path string) string {
	return fmt.Sprintf("Read the complete task instructions from the absolute file %q. Treat the entire file contents as the full task instruction, follow them exactly, and return only the requested final output.", path)
}
