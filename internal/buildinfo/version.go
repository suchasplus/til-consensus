package buildinfo

import (
	"fmt"
	"runtime"
	"strings"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
	Dirty     = "unknown"
)

func Short() string {
	if value := strings.TrimSpace(Version); value != "" {
		return value
	}
	return "dev"
}

func CommitID() string {
	if value := strings.TrimSpace(Commit); value != "" {
		return value
	}
	return "unknown"
}

func BuiltAt() string {
	if value := strings.TrimSpace(BuildTime); value != "" {
		return value
	}
	return "unknown"
}

func IsDirty() string {
	if value := strings.TrimSpace(Dirty); value != "" {
		return value
	}
	return "unknown"
}

func Info() map[string]string {
	return map[string]string{
		"version":   Short(),
		"commit":    CommitID(),
		"buildTime": BuiltAt(),
		"dirty":     IsDirty(),
		"goVersion": runtime.Version(),
		"goos":      runtime.GOOS,
		"goarch":    runtime.GOARCH,
	}
}

func Format() string {
	return fmt.Sprintf(
		"version: %s\ncommit: %s\nbuild time: %s\ndirty: %s\ngo version: %s\ngoos: %s\ngoarch: %s\n",
		Short(),
		CommitID(),
		BuiltAt(),
		IsDirty(),
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
	)
}
