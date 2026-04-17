package app

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func shouldEnableColor(writer io.Writer) bool {
	if force := strings.TrimSpace(os.Getenv("FORCE_COLOR")); force != "" && force != "0" && !strings.EqualFold(force, "false") {
		return true
	}
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	term := strings.TrimSpace(os.Getenv("TERM"))
	return term != "" && !strings.EqualFold(term, "dumb")
}

func ansi(code int, value string) string {
	return fmt.Sprintf("\x1b[%dm%s\x1b[0m", code, value)
}

func colorizeRunOutput(text string) string {
	if text == "" {
		return text
	}
	replacements := []struct {
		old string
		new string
	}{
		{"[til-consensus][debug]", ansi(35, "[til-consensus][debug]")},
		{"[til-consensus]", ansi(36, "[til-consensus]")},
		{"run started", ansi(34, "run started")},
		{"run completed", ansi(32, "run completed")},
		{"phase completed:", ansi(34, "phase completed:")},
		{"phase:", ansi(34, "phase:")},
		{"task dispatched:", ansi(36, "task dispatched:")},
		{"task completed:", ansi(32, "task completed:")},
		{"task retrying:", ansi(33, "task retrying:")},
		{"task failed:", ansi(31, "task failed:")},
		{"claim revised:", ansi(33, "claim revised:")},
		{"claim adjudicated:", ansi(35, "claim adjudicated:")},
		{"observation recorded:", ansi(35, "observation recorded:")},
		{"session_started", ansi(36, "session_started")},
		{"session_finalized", ansi(35, "session_finalized")},
		{"session_failed", ansi(31, "session_failed")},
		{"ledger_appended", ansi(36, "ledger_appended")},
		{"supported", ansi(32, "supported")},
		{"keep_with_caveat", ansi(33, "keep_with_caveat")},
		{"keep", ansi(32, "keep")},
		{"refuted", ansi(31, "refuted")},
		{"reject", ansi(31, "reject")},
		{"error=", ansi(31, "error=")},
		{"reason=", ansi(33, "reason=")},
		{"undetermined", ansi(33, "undetermined")},
		{"insufficient_evidence", ansi(33, "insufficient_evidence")},
		{"inconclusive", ansi(33, "inconclusive")},
		{"contradicted", ansi(31, "contradicted")},
		{"reopen=true", ansi(33, "reopen=true")},
	}
	for _, item := range replacements {
		text = strings.ReplaceAll(text, item.old, item.new)
	}
	return text
}

func colorizeViewText(text string) string {
	if text == "" {
		return text
	}
	replacements := []struct {
		old string
		new string
	}{
		{"运行头部", ansi(36, "运行头部")},
		{"任务摘要", ansi(36, "任务摘要")},
		{"统计", ansi(36, "统计")},
		{"关键 Claims", ansi(34, "关键 Claims")},
		{"挑战明细", ansi(33, "挑战明细")},
		{"验证明细", ansi(35, "验证明细")},
		{"Observations", ansi(35, "Observations")},
		{"Follow-ups", ansi(35, "Follow-ups")},
		{"Debug Events", ansi(35, "Debug Events")},
		{"Rounds", ansi(34, "Rounds")},
		{"Votes", ansi(34, "Votes")},
		{"Statements", ansi(34, "Statements")},
		{"Convergence", ansi(34, "Convergence")},
		{"风险与未决项", ansi(33, "风险与未决项")},
		{"相关文件", ansi(36, "相关文件")},
		{"supported", ansi(32, "supported")},
		{"keep_with_caveat", ansi(33, "keep_with_caveat")},
		{"keep", ansi(32, "keep")},
		{"refuted", ansi(31, "refuted")},
		{"reject", ansi(31, "reject")},
		{"failed", ansi(31, "failed")},
		{"undetermined", ansi(33, "undetermined")},
		{"insufficient evidence", ansi(33, "insufficient evidence")},
		{"insufficient_evidence", ansi(33, "insufficient_evidence")},
		{"inconclusive", ansi(33, "inconclusive")},
		{"unresolved_conflict", ansi(33, "unresolved_conflict")},
		{"requires_human_review", ansi(31, "requires_human_review")},
		{"action_blocked_by_risk", ansi(33, "action_blocked_by_risk")},
		{"rawVerdict:", ansi(35, "rawVerdict:")},
		{"rawTaskVerdict:", ansi(35, "rawTaskVerdict:")},
		{"failure:", ansi(31, "failure:")},
	}
	for _, item := range replacements {
		text = strings.ReplaceAll(text, item.old, item.new)
	}
	return text
}
