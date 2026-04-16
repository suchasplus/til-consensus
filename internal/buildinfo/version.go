package buildinfo

import "strings"

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
	}
}
