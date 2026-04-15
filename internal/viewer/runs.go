package viewer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/suchasplus/til-consensus/internal/artifact"
)

type CompletedRun struct {
	RequestID  string
	ResultPath string
}

func ListCompletedRuns(resultTemplate string) ([]CompletedRun, error) {
	const token = "{requestId}"
	index := strings.Index(resultTemplate, token)
	if index == -1 {
		return nil, nil
	}
	prefixSlash := strings.LastIndex(resultTemplate[:index], string(filepath.Separator))
	if prefixSlash == -1 {
		return nil, nil
	}
	scanDir := resultTemplate[:prefixSlash]
	entries, err := os.ReadDir(scanDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]CompletedRun, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !artifact.RequestIDPattern.MatchString(entry.Name()) {
			continue
		}
		resultPath := strings.ReplaceAll(resultTemplate, token, entry.Name())
		if _, err := os.Stat(resultPath); err != nil {
			continue
		}
		out = append(out, CompletedRun{
			RequestID:  entry.Name(),
			ResultPath: resultPath,
		})
	}
	slices.SortFunc(out, func(left, right CompletedRun) int {
		switch {
		case left.RequestID < right.RequestID:
			return -1
		case left.RequestID > right.RequestID:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

func ResolveLatestRun(resultTemplate string) (*CompletedRun, error) {
	runs, err := ListCompletedRuns(resultTemplate)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	latest := runs[len(runs)-1]
	return &latest, nil
}

func OpenBrowser(url string) error {
	var command string
	var args []string
	switch {
	case isDarwin():
		command = "open"
		args = []string{url}
	case isLinux():
		command = "xdg-open"
		args = []string{url}
	case isWindows():
		command = "cmd"
		args = []string{"/c", "start", "", url}
	default:
		return fmt.Errorf("unsupported platform for browser launch")
	}
	if err := exec.Command(command, args...).Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}

var (
	isDarwin  = func() bool { return runtimeGOOS() == "darwin" }
	isLinux   = func() bool { return runtimeGOOS() == "linux" }
	isWindows = func() bool { return runtimeGOOS() == "windows" }
)

var runtimeGOOS = func() string {
	return runtime.GOOS
}
