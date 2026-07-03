package artifact

import (
	"fmt"
	"time"

	"github.com/suchasplus/til-consensus/consensus"
)

func BuildSummary(result *consensus.RunResult) string {
	return consensus.BuildRunSummary(result)
}

func FormatDuration(value time.Duration) string {
	if value < time.Second {
		return fmt.Sprintf("%dms", value.Milliseconds())
	}
	if value < time.Minute {
		return fmt.Sprintf("%.1fs", value.Seconds())
	}
	minutes := int(value / time.Minute)
	seconds := int((value % time.Minute) / time.Second)
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}
